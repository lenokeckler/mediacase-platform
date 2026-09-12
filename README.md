<p align="center"><img src="dashboard/src/app/public/logo.svg" alt="MediaCase" width="96" height="96"></p>

# MediaCase

**Plataforma distribuida de procesamiento multimedia por casos y monitoreo cooperativo de recursos.**

IC-6600 Principios de Sistemas Operativos · Instituto Tecnológico de Costa Rica, Campus San Carlos ·
II Semestre 2026 · Proyecto Programado I (consigna v2.0).

**Equipo:** Magdaleno Gómez Díaz · Jennifer Yajaira López Miranda · Jonathan Sancho Loaiza

---

## Qué es

Un sistema distribuido cuya unidad de trabajo es el **caso**: un conjunto de uno o varios archivos
multimedia relacionados que entra como **una sola solicitud**. El coordinador inspecciona cada
archivo y decide la operación por su tipo (video → convertir, audio → convertir, imagen →
miniatura), descompone el caso en sub-tareas, las encola por pool y prioridad, las reparte entre
workers que corren en máquinas distintas, y cuando **todas** resolvieron (barrier/join) cierra el
caso con un estado agregado y un **reporte consolidado**. Un caso puede ser homogéneo (un solo
tipo de archivo) o heterogéneo (video + audio + imágenes, tres operaciones en tres pools).

Lo que importa aquí no es la interfaz sino lo del curso: procesos y estados, planificación por
prioridad y por pool, colas, concurrencia, sincronización con barrier, comunicación por red entre
nodos, y monitoreo con balanceo de carga.

## Arquitectura en diez líneas

- **node-1** (la laptop de Leno): PostgreSQL + Redis + MinIO + Prometheus + Grafana en Docker; el
  **coordinador** (Go) y un worker de video como procesos nativos. Una sola URL,
  `http://<ip>:8080`, sirve el dashboard, la API, el WebSocket, `/metrics` y `/connect`.
- **Workers** (Go + ffmpeg) en cualquier otra máquina: VMs Ubuntu creadas con Vagrant, las
  laptops del equipo, o cualquier PC que baje el ZIP de `/connect`. Cada worker declara un rol
  (`video` · `audio` · `metadata` · `all`) y **abre él** un WebSocket hacia el coordinador: no
  necesita puerto abierto ni IP alcanzable.
- **Cola**: 9 streams de Redis, uno por pool × prioridad. El scheduler solo saca de la cola de un
  pool cuando hay un worker de ese pool con capacidad; dentro del pool elige el menos cargado.
- **Estado** en PostgreSQL (casos, sub-tareas, workers); **archivos** en MinIO (`dataset/`
  entradas, `results/` salidas y reportes).
- **Barrier**: cada sub-tarea que resuelve bloquea la fila del caso y cuenta; el caso cierra una
  sola vez, como `completed`, `partially_completed` o `failed`.

Detalle con diagramas en [`docs/architecture.md`](docs/architecture.md).

## Levantar node-1

Requisitos en node-1: Docker Desktop, ffmpeg en el PATH, Go 1.26+ (o los binarios ya compilados en
`bin/`), Node 20+ solo si se toca el dashboard (el compilado `dist/` está versionado).

**Doble clic en `MediaCase.bat`.** Arranca Docker Desktop si hace falta, levanta Postgres, Redis,
MinIO, Prometheus y Grafana, detecta la IP de la laptop en el WiFi (`MINIO_PUBLIC_ENDPOINT=auto`),
compila y abre el coordinador y el worker local de video en dos ventanas, y abre el dashboard.
`MediaCase-detener.bat` apaga todo (y cierra Docker Desktop para liberar RAM). Una vez, como
administrador: `scripts\firewall-node1.ps1` (abre 8080 y 9000 para los demás nodos).

A mano, son los mismos tres pasos:

```powershell
docker compose -f docker-compose.infra.yml up -d   # 1. infraestructura
scripts\run-coordinator.ps1                         # 2. coordinador (lee infra/env/node1.env)
scripts\run-worker.ps1                              # 3. worker local de video (otra terminal)
```

| Servicio | URL |
|---|---|
| Dashboard + API | http://localhost:8080 |
| Grafana (lectura sin login) | http://localhost:3001 |
| Prometheus | http://localhost:9090 |
| MinIO consola | http://localhost:9001 (`minioadmin` / `minioadmin`) |

## Sumar un worker

- **Cualquier PC de la red**: abrir `http://<ip-de-node-1>:8080/connect`, descargar el ZIP de su
  sistema, descomprimir, doble clic en `start-worker.bat` (o `bash start-worker.sh`). Aparece en
  el dashboard en segundos. Rol y tamaño del pool se cambian en `worker.env`.
- **VMs node-2 y node-3** (Vagrant + VirtualBox, en node-1): `cd infra/vagrant && vagrant up`;
  tras recompilar, `bash redeploy.sh`.
- **Desde otra red**: en el dashboard, Monitor → **Compartir → "Publicar en internet"**: el
  coordinador abre dos túneles de Cloudflare (8080 y 9000) y muestra la URL `https://…/connect` para
  pasar; el ZIP bajado por ahí sale con `https://`/`wss://` y MinIO por TLS. Si la red bloquea el
  túnel (el WiFi del TEC corta el puerto 7844), la tarjeta lo dice y la salida es encender WARP en
  node-1. También por script: `scripts\tunnel.ps1`.

Guía paso a paso, avisos de Windows 11 y diagnóstico en
[`docs/manual-usuario.md`](docs/manual-usuario.md).

## Enviar un caso

- **Dashboard**: pestaña Casos → **+ Nuevo caso** → subir archivos o elegirlos del dataset →
  enviar → ver sus sub-tareas avanzar → leer el reporte con enlaces de descarga.
- **CLI**: `go run ./cmd/client -case -name demo -files "a.mp4,b.flac,c.webp" -watch`
- **Generación automática** desde el dataset:
  `bin/ingest cases --group-by session --limit 10` (casos homogéneos y heterogéneos por
  construcción), y **generador de carga**:
  `bin/ingest load --cases 20 --concurrency 5 --group-by session --wait`.

## Dataset

492 archivos sintéticos (250 video, 172 audio, 70 imágenes; 13 formatos; 310 livianos < 5 MB,
140 medianos 20-50 MB, 42 pesados 150-400 MB; 14.3 GB) con metadatos de agrupación (evento,
sesión, lote, usuario). Se genera con `bash dataset/scripts/generate_dataset.sh` (~1 h,
reproducible) y se sube con `bin/ingest upload`. Composición y criterios en
[`docs/dataset.md`](docs/dataset.md).

## Pruebas

Cada fase del desarrollo cerró con un script de hito que termina en `HITO OK`:

| Script | Qué demuestra |
|---|---|
| `tests/distributed_smoke.sh` | un worker en otra máquina procesa una sub-tarea |
| `tests/case_scenario.sh` | caso heterogéneo con un archivo corrupto → `partially_completed` y reporte |
| `tests/pools_scenario.sh` | cada sub-tarea corre en un worker de su pool |
| `tests/failure_scenario.sh` | caída de un worker → sus sub-tareas se re-encolan y las toma otro |
| `tests/dataset_scenario.sh` | el dataset real subido y convertido automáticamente en casos que cierran |
| `tests/monitoring_scenario.sh` | bajo 20 casos concurrentes: `/metrics`, Prometheus, Grafana, sub-tareas por caso, saturación por pool |
| `tests/measure_times.sh` | informe de tiempos por sub-tarea, por caso, por pool y por tamaño (PostgreSQL) |

Resultados con números reales y capturas en [`docs/informe-pruebas.md`](docs/informe-pruebas.md).
Tests unitarios: `go test ./...` (routing, barrier, reporte, agrupación, registry, hub).

## Estructura del repositorio

```
cmd/coordinator         proceso coordinador (API, scheduler, barrier, reporte, dashboard estático)
cmd/worker              proceso worker (canal saliente, pool de goroutines, ffmpeg, MinIO)
cmd/client              cliente CLI: casos, seguimiento, sub-tareas sueltas
cmd/ingest              ingesta del dataset, generación automática de casos, generador de carga
internal/cases          routing por tipo, barrier/join, reporte consolidado (puro, con tests)
internal/coordinator    API HTTP, registry de workers, scheduler, hubs WebSocket, /metrics
internal/queue          Redis Streams: 9 colas pool × prioridad
internal/db             esquema y consultas PostgreSQL
internal/ingest         agrupación del manifest en casos (puro, con tests)
internal/multimedia     operaciones ffmpeg
internal/storage        cliente MinIO
internal/monitoring     métricas del worker
dashboard/              React + Vite (fuente); dist/ es el compilado que sirve el coordinador
dataset/                generador, manifest.json y validador; files/ no se versiona
infra/                  prometheus.yml, Grafana provisionado, Vagrantfile, plantillas .env
scripts/                start-node1/stop-node1 (los .bat), run-coordinator/run-worker, build-dashboard/build-workers, firewall, túnel
tests/                  scripts de hito e informe de tiempos
docs/                   arquitectura, API, manual de usuario, dataset, informe de pruebas, plan
```

## Documentación

| Documento | Contenido |
|---|---|
| [`docs/architecture.md`](docs/architecture.md) | componentes, nodos, flujo de caso y sub-tarea, colas, pools (Unidad 1), comunicación, despliegue, decisiones |
| [`docs/manual-usuario.md`](docs/manual-usuario.md) | dashboard, enviar/seguir/cancelar casos, reporte, conectar una PC, diagnóstico |
| [`docs/api.md`](docs/api.md) | todos los endpoints con ejemplos reales |
| [`docs/dataset.md`](docs/dataset.md) | composición, criterios de agrupación, volumen, uso |
| [`docs/informe-pruebas.md`](docs/informe-pruebas.md) | carga, distribución, casos heterogéneos, fallos, saturación y redistribución, hardware real |
| [`docs/plan/`](docs/plan/) | los planes de implementación con cada tarea y su verificación |

## Estado del proyecto

Fases 0-5 del plan cerradas con hito verificado (distribución real, capa de casos, pools
especializados, dashboard por caso, dataset e ingesta, monitoreo completo). En curso: documentación
final y despliegue en las tres laptops del equipo. Ver `docs/plan/2026-09-11-plan-2-entrega.md`.

## Licencia

MIT — ver [`LICENSE`](LICENSE).
