# MediaCase Platform

**Plataforma distribuida de procesamiento multimedia por casos y monitoreo cooperativo de recursos.**

IC-6600 · Principios de Sistemas Operativos · Instituto Tecnológico de Costa Rica
Campus San Carlos · II Semestre 2026 · Proyecto Programado I (consigna v2.0)

**Equipo:** Magdaleno Gómez Díaz · Jennifer Yajaira Lopez Miranda · Jonathan Sancho Loaiza

---

## Qué es

Un sistema distribuido que recibe **casos de procesamiento** —conjuntos de uno o varios
archivos multimedia relacionados, enviados como una sola solicitud— los descompone en
sub-tareas, las enruta según el tipo de cada archivo, las ejecuta concurrentemente en
varios nodos worker, y al cerrar el caso produce un **reporte consolidado**.

Un caso puede ser **homogéneo** (todos los archivos del mismo tipo, misma operación) o
**heterogéneo** (mezcla de audio y video que requieren operaciones distintas). El
coordinador inspecciona cada archivo, decide la operación, y aplica un mecanismo de
sincronización **barrier/join** para determinar cuándo el caso terminó.

El foco del proyecto no es la reproducción multimedia ni la interfaz, sino la
**arquitectura distribuida de procesamiento**: administración de procesos, planificación,
colas, concurrencia, sincronización, comunicación entre nodos y monitoreo de recursos.

## Arquitectura

```
Cliente / generador de casos
          │
          ▼
   Job Queue  (Redis Streams · jobs:high · jobs:normal · jobs:low)
          │
          ▼
   Coordinador
     ├─ Registry   → registro de workers, heartbeat, evicción de nodos muertos
     ├─ Scheduler  → dequeue por prioridad, asignación least-loaded, reclaim
     ├─ Router     → inspecciona el archivo y determina la operación
     ├─ Barrier    → cierra el caso cuando todas sus sub-tareas resolvieron
     └─ API HTTP   → REST + WebSocket
          │
    ┌─────┼─────┬─────────┐
    ▼     ▼     ▼         ▼
 Worker-1 Worker-2 Worker-3 ...   (pool de goroutines · FFmpeg)
    └─────┴─────┴─────────┘
          │
          ├──► PostgreSQL   estado de casos, sub-tareas y workers
          └──► MinIO        repositorio de resultados
                    │
                    ▼
          Dashboard (React + WebSocket)  ·  Prometheus + Grafana
```

Documentación detallada en [`docs/architecture.md`](docs/architecture.md).

## Stack

| Componente | Tecnología | Rol |
|---|---|---|
| Coordinador | Go 1.26 | Orquestación, routing, scheduler, API REST + WebSocket |
| Workers | Go 1.26 + FFmpeg | Ejecución de sub-tareas multimedia, pool de goroutines |
| Cola | Redis 7 Streams | 3 niveles de prioridad con consumer groups |
| Estado | PostgreSQL 16 | Casos, sub-tareas, workers |
| Resultados | MinIO | Almacenamiento de archivos procesados |
| Observabilidad | Prometheus + Grafana | Métricas por worker |
| Dashboard | React 18 + Vite | Monitoreo en tiempo real |

## Operaciones soportadas

| Operación | Entrada | Salida |
|---|---|---|
| `convert` | video | MP4 re-codificado (H.264 + AAC) |
| `extract_audio` | video / audio | MP3 192k |
| `convert_audio` | audio | WAV PCM 16-bit 44.1 kHz |
| `thumbnail` | video / audio | JPEG del frame a los 5 s · PNG de forma de onda |

## Puesta en marcha

**Requisitos:** Docker Desktop · Go 1.26+ (solo para el cliente CLI) · FFmpeg (solo para
generar el dataset).

```bash
# 1. Generar el dataset de prueba (400+ archivos sintéticos)
chmod +x dataset/scripts/generate_dataset.sh
./dataset/scripts/generate_dataset.sh

# 2. Levantar el sistema completo
make up
make logs
```

| Servicio | URL | Credenciales |
|---|---|---|
| Dashboard | http://localhost:5173 | — |
| API del coordinador | http://localhost:8080 | — |
| MinIO Console | http://localhost:9001 | `minioadmin` / `minioadmin` |
| Prometheus | http://localhost:9090 | — |
| Grafana | http://localhost:3001 | `admin` / `admin` |

## Pruebas

```bash
./tests/measure_times.sh      # reporte de tiempos de procesamiento
./tests/failure_scenario.sh   # tolerancia a fallos: mata un worker y verifica el reclaim
```

## Estado del proyecto

La capa de **infraestructura distribuida** está implementada y funcionando. La capa de
**casos** (la unidad de trabajo que define la consigna v2.0) está en desarrollo.

**Implementado**
- [x] Cola Redis Streams con 3 prioridades y consumer groups
- [x] Registro de workers con heartbeat y evicción de nodos caídos
- [x] Scheduler con asignación *least-loaded* y re-encolado de trabajos huérfanos
- [x] Pool de goroutines por worker con backpressure (HTTP 429)
- [x] Reintentos con `max_retries` y detección de trabajos colgados
- [x] Operaciones FFmpeg y subida de resultados a MinIO
- [x] Dashboard en tiempo real por WebSocket
- [x] Métricas Prometheus y dashboards de Grafana
- [x] Generador de dataset sintético

**En desarrollo**
- [ ] Entidad `Case` y descomposición de casos en sub-tareas
- [ ] Routing por tipo de archivo decidido en el coordinador
- [ ] Sincronización barrier/join y estado agregado del caso
- [ ] Reporte consolidado por caso
- [ ] Pools de workers especializados por tipo de contenido
- [ ] Generación automática de casos por agrupación de metadatos
- [ ] Despliegue multi-máquina con distribución física real
- [ ] Manual de usuario

## Estructura

```
cmd/
  coordinator/        punto de entrada del coordinador
  worker/             punto de entrada del worker
  client/             cliente CLI y generador de carga
  generate_manifest/  construcción del manifiesto del dataset
internal/
  coordinator/        api · registry · scheduler · websocket hub
  queue/              Redis Streams
  db/                 esquema y acceso a PostgreSQL
  models/             tipos de dominio
  multimedia/         wrappers de FFmpeg
  monitoring/         métricas Prometheus
  storage/            cliente MinIO
dashboard/            interfaz React + Vite
dataset/              generador y manifiesto del dataset de prueba
infra/                configuración de Prometheus y Grafana
docs/                 arquitectura y guía de pruebas
tests/                scripts de carga y de fallo
```

## Créditos

Magdaleno Gómez Díaz · Jennifer Yajaira Lopez Miranda · Jonathan Sancho Loaiza
IC-6600 Principios de Sistemas Operativos · TEC Campus San Carlos · II Semestre 2026

## Licencia

MIT — ver [`LICENSE`](LICENSE).
