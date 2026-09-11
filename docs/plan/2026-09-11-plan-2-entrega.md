# Plan 2 — Interfaz por caso, dataset real, monitoreo completo, documentación y despliegue final

> **Para quien ejecute este plan:** usar `superpowers:executing-plans` tarea por tarea, en orden. Cada paso lleva casilla `- [ ]`. Ninguna tarea se da por terminada sin correr su verificación. **Ningún `git push` sin luz verde explícita de Leno.** Sigue al Plan 1 (`2026-09-10-capa-de-casos.md`), cuyas fases 0-2 están cerradas.

**Objetivo:** que el sistema se maneje completo desde el navegador (enviar casos, verlos, leer el reporte), que el dataset cumpla la consigna (400-600 archivos con pesados e imágenes, organizado en casos, con generación automática), que el monitoreo cubra casos y pools, y que existan los seis entregables con evidencia real.

**Arquitectura (ya construida en el Plan 1):** node-1 (laptop de Leno) corre Postgres+Redis+MinIO en Docker y el coordinador + dashboard + worker-video como procesos; los demás nodos corren un binario que abre él la conexión hacia el coordinador. La unidad de trabajo es el caso; el barrier lo cierra y genera el reporte.

**Stack:** Go 1.27 · React 18 + Vite 5 (CSS Modules, tema oscuro ya existente) · Node 24 / npm 11 · PostgreSQL 16 · Redis 7 · MinIO · ffmpeg 9 · Prometheus + Grafana · Vagrant + VirtualBox · cloudflared.

**Spec:** `../../../ProyectoProgramadoI_PlataformaMultimediaCasos_v2.docx` (resumen en `CLAUDE.md` §1-§13).

## Restricciones globales (copiadas de la consigna)

- Dashboard: *"estado agregado por caso: en progreso, completado, parcialmente completado, fallido · estado por sub-tarea y por worker: pendientes, en proceso, completadas y fallidas · comportamiento de carga"*. Entregable: *"dashboard o consola de monitoreo (con vista por caso y por sub-tarea)"*.
- Monitoreo: *"CPU, memoria, carga de trabajo por worker, estado de nodos, sub-tareas activas o en espera, agrupadas por caso"*.
- Dataset: *"entre (mínimo) 400 y 600 archivos · mezcla de audio y video · diversidad de formatos y tamaños (livianos, medianos y pesados) · al menos un conjunto de casos homogéneos y un conjunto de casos heterogéneos · metadatos asociados (JSON, BD u otra) · documentar composición, criterios de agrupación y volumen total"*.
- Generación automática: *"agrupe archivos en casos a partir de un criterio: misma carpeta local, repositorio compartido, almacenamiento en la nube, agrupación por metadatos (evento, sesión, usuario, lote de ingesta)"*.
- Cliente: *"envío de casos · consulta por caso y por sub-tarea · recuperación de resultados y reportes · generación de casos concurrentes"*.
- Entregables: documento de arquitectura · repositorio · sistema funcional distribuido · dashboard · documentación técnica y manual de uso · informe de pruebas con evidencia de carga, distribución, casos heterogéneos y comportamiento del sistema.
- El dashboard vale 5 %: **no rediseñar**, agregar la pestaña que falta con el estilo que ya existe.
- Los `.ps1` en UTF-8 con BOM y sin tildes en cadenas; los `.sh` con LF; nada de `node_modules/`, `dataset/files/`, `bin/`, `dist/` en git.

---

## Fases y orden

| Fase | Qué entrega | Rubros |
|---|---|---|
| **3 — Pestaña "Casos"** | Todo el flujo de un caso desde el navegador; una sola URL para dashboard, API y `/connect` | Dashboard 5 % · Casos y resultados 10 % |
| **4 — Dataset y generación automática** | 400-600 archivos con pesados e imágenes, organizados en casos, ingesta automática, generador de carga | Casos y concurrencia 20 % · Documentación |
| **5 — Monitoreo completo** | `/metrics` del coordinador (workers remotos), sub-tareas agrupadas por caso, Grafana con casos y pools | Monitoreo y balanceo 15 % |
| **6 — Documentación y evidencia** | README, manual de usuario, arquitectura con diagramas, API, informe de pruebas con números reales | Arquitectura 15 % · Documentación 5 % |
| **7 — Despliegue final** | 3 nodos con Vagrant, túnel Cloudflare, VM Arch, las 3 laptops físicas, checklist de la rúbrica | Implementación distribuida 20 % |

La Fase 3 lleva pasos al detalle. Las fases 4-7 tienen tareas concretas con archivos y verificación; su detalle paso a paso se expande al abrirlas, como se hizo en el Plan 1.

---

## Mapa de archivos (Fase 3)

```
cmd/coordinator/main.go                   modificar  servir dashboard/dist en "/" y la API bajo "/api/"
internal/coordinator/api.go               modificar  Router() devuelve el mux de la API; nuevo Handler() compuesto
internal/coordinator/static.go            crear      servidor de la SPA con fallback a index.html
internal/coordinator/upload.go            crear      POST /upload → MinIO dataset/ ; GET /dataset → lista del bucket
internal/storage/minio.go                 modificar  ListObjects(bucket, prefix)
dashboard/src/api.js                      modificar  cases: submit/list/get/report/cancel; dataset: list/upload
dashboard/src/app/app.jsx                 modificar  pestaña "Casos" (primera) y "Conectar" (enlace a /connect)
dashboard/src/components/CasesPanel.jsx   crear      lista de casos + filtro + auto-refresh
dashboard/src/components/CasesPanel.module.css
dashboard/src/components/SubmitCasePanel.jsx   crear  subir archivos o elegir del dataset, nombre, prioridad
dashboard/src/components/SubmitCasePanel.module.css
dashboard/src/components/CaseDetail.jsx   crear      sub-tareas, cancelar, reporte con descargas
dashboard/src/components/CaseDetail.module.css
dashboard/src/components/WorkerCard.jsx   modificar  mostrar rol y pools
dashboard/src/components/QueueDepth.jsx   modificar  profundidad por pool
dashboard/src/hooks/useCases.js           crear      polling cada 2 s de GET /cases (+ detalle del caso abierto)
scripts/build-dashboard.ps1               crear      npm install + npm run build
tests/ui_case_flow.md                     crear      guion manual del hito (capturas)
```

---

# FASE 3 — Pestaña "Casos" en el dashboard  ✅ cerrada 2026-09-11 (hito `tests/ui_case_flow.md`)

**Hito:** desde el navegador, en `http://<ip-de-leno>:8080`: subir 3 archivos (video, audio, imagen), enviar el caso con nombre y prioridad, ver sus sub-tareas avanzar con su worker, ver el caso cerrar y leer el reporte con enlaces de descarga; cancelar otro caso a medio camino. Todo sin abrir una terminal.

### Task 3.0: Entorno del dashboard

- [x] **Paso 1:** `cd dashboard && npm install` (Node 24 y npm 11 ya están). Esperado: `node_modules/` creado, sin errores de peer deps.
- [x] **Paso 2:** con el coordinador nativo corriendo (`scripts/run-coordinator.ps1`), `npm run dev` → abrir `http://localhost:5173`. Esperado: dashboard actual con los workers en vivo (el proxy de Vite manda `/api` y `/ws` al 8080).
- [x] **Paso 3:** `npm run build` → `dashboard/dist/` regenerado. Commit **no** incluye `dist/` si se decide servirlo desde el coordinador compilando en CI; **decisión:** `dist/` sigue versionado (ya lo está) para que quien clone no necesite Node. Se regenera con `scripts/build-dashboard.ps1` antes de cada commit que toque el dashboard.
- [x] **Paso 4:** `scripts/build-dashboard.ps1`:

```powershell
# Compila el dashboard (React/Vite) a dashboard/dist, que sirve el coordinador.
Set-Location (Join-Path $PSScriptRoot '..\dashboard')
if (-not (Test-Path node_modules)) { npm install }
npm run build
if ($LASTEXITCODE -ne 0) { Write-Error 'fallo el build del dashboard'; exit 1 }
```
(UTF-8 con BOM, CRLF.)

---

### Task 3.1: El coordinador sirve el dashboard — una sola URL

Hoy el dashboard compilado se sirve con nginx en Docker y llama a `/api/*`. En la topología nativa no hay nginx. El coordinador pasa a servir `dashboard/dist` en `/` y la API bajo `/api/` (además de en `/`, para no romper al worker, al cliente ni a los scripts).

**Files:** Create `internal/coordinator/static.go`; Modify `internal/coordinator/api.go` (`Handler()`), `cmd/coordinator/main.go`.

**Interfaces:**
- `func (a *API) Handler(staticDir string) http.Handler` — compone: `/api/` → API sin prefijo · `/ws` y `/workers/{id}/stream` → como hoy · rutas de API en `/` (compatibilidad) · todo lo demás → SPA (archivo si existe, si no `index.html`).
- Variable de entorno `DASHBOARD_DIR` (default `dashboard/dist`); si el directorio no existe, `/` devuelve 404 y el resto sigue funcionando.

- [x] **Paso 1: `static.go`**

```go
package coordinator

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler sirve los archivos compilados del dashboard y devuelve index.html para
// cualquier ruta que no sea un archivo (React Router / recarga en una pestaña).
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "dashboard no compilado: correr scripts/build-dashboard.ps1", http.StatusNotFound)
			return
		}
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r) // un asset que no existe no debe devolver index.html
			return
		}
		http.ServeFile(w, r, index)
	})
}
```

- [x] **Paso 2: `Handler()` en `api.go`** — `Router()` sigue devolviendo el mux de la API. Nuevo:

```go
// Handler compone la API (en / y bajo /api/), el WebSocket y el dashboard estático.
func (a *API) Handler(staticDir string) http.Handler {
	api := a.Router()
	root := http.NewServeMux()
	root.Handle("/api/", http.StripPrefix("/api", api))
	// Rutas de la API que deben seguir existiendo sin prefijo (worker, cliente, scripts, /connect):
	for _, p := range []string{"/jobs", "/jobs/", "/cases", "/cases/", "/workers", "/workers/",
		"/stats", "/ws", "/upload", "/files", "/dataset", "/connect", "/download/", "/batch"} {
		root.Handle(p, api)
	}
	root.Handle("/", spaHandler(staticDir))
	return root
}
```
En `main.go`: `Handler: api.Handler(envOr("DASHBOARD_DIR", "dashboard/dist"))`.

- [x] **Paso 3: Verificar** — `go build ./... && go vet ./...`; con el coordinador nativo: `curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/` → 200 (HTML); `curl -s localhost:8080/api/workers` y `curl -s localhost:8080/workers` → mismo JSON; `curl -s localhost:8080/connect | head -c 60` → HTML de conectar; abrir `http://172.24.83.164:8080` desde otra máquina → dashboard con workers en vivo (el WS en `/ws` sigue funcionando porque el dashboard lo abre contra `window.location.host`).
- [x] **Paso 4: Commit** — `feat(coordinator): sirve el dashboard compilado y la API bajo /api (una sola URL)`

---

### Task 3.2: Subida al bucket de entradas y listado del dataset

Hoy `POST /upload` guarda en una carpeta local. Los workers ya solo leen de MinIO: la subida va al bucket `dataset/`.

**Files:** Create `internal/coordinator/upload.go`; Modify `internal/storage/minio.go` (`ListObjects`), `api.go` (rutas; quitar `uploadFile` y `listFiles` viejos), `cmd/coordinator/main.go` (pasar el cliente MinIO al API).

**Interfaces:**
- `func (m *MinIOClient) ListObjects(ctx, bucket, prefix string) ([]ObjectInfo, error)` con `ObjectInfo{Key string; Size int64; LastModified time.Time}`.
- `POST /upload` (multipart, campo `file`, puede repetirse) → `201 {"keys":["boda.mp4","discurso.mp3"]}`. Clave = nombre base saneado; si ya existe, se sobrescribe (misma clave = mismo contenido para el dataset de prueba).
- `GET /dataset?prefix=casos/` → `[{"key":..,"size":..,"type":"video|audio|image|other"}]`.
- `NewAPI(..., minio *storage.MinIOClient)`; si es `nil`, `/upload` responde 503.

- [x] **Paso 1: `ListObjects`** en storage (con `m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})`).
- [x] **Paso 2: `upload.go`** — `ParseMultipartForm(64 << 20)`; por cada `FileHeader` en `r.MultipartForm.File["file"]`: guardar a temporal, `UploadObject(ctx, storage.DatasetBucket, safeName, tmp)`, borrar temporal. Rechazar nombres sin extensión soportada usando `cases.DetectFileType` (400 con el nombre).
- [x] **Paso 3: `GET /dataset`** — lista con `cases.DetectFileType` para el campo `type`.
- [x] **Paso 4: Verificar** — `curl -F file=@C:/tmp/mediacase/prueba.mp4 -F file=@C:/tmp/mediacase/prueba.wav localhost:8080/upload` → `{"keys":["prueba.mp4","prueba.wav"]}`; `curl localhost:8080/dataset` los lista; en MinIO (`localhost:9001`) están en `dataset/`. `curl -F file=@go.mod localhost:8080/upload` → 400 `formato no soportado`.
- [x] **Paso 5: Commit** — `feat(coordinator): POST /upload sube al bucket dataset; GET /dataset lista las entradas`

---

### Task 3.3: `api.js` y hook `useCases`

**Files:** Modify `dashboard/src/api.js`; Create `dashboard/src/hooks/useCases.js`.

- [x] **Paso 1: `api.js`** — agregar:

```js
    // ── Casos ──
    submitCase: (name, priority, files) =>
        request('POST', '/cases', { name, priority: Number(priority), files }),
    listCases: (status = '') =>
        request('GET', `/cases${status ? `?status=${status}` : ''}`),
    getCase: (id) => request('GET', `/cases/${id}`),
    getCaseReport: (id) => request('GET', `/cases/${id}/report`),
    cancelCase: (id) => request('POST', `/cases/${id}/cancel`),

    // ── Dataset (bucket de entradas) ──
    listDataset: (prefix = '') => request('GET', `/dataset?prefix=${encodeURIComponent(prefix)}`),
    uploadFiles: async (fileList) => {
        const form = new FormData()
        for (const f of fileList) form.append('file', f)
        const r = await fetch(`${BASE}/upload`, { method: 'POST', body: form })
        if (!r.ok) throw new Error(await r.text() || `HTTP ${r.status}`)
        return r.json()   // { keys: [...] }
    },
```
y quitar `uploadFile` viejo y `listFiles` (ya no hay carpeta local). `BatchPanel.jsx` y `SubmitJobPanel.jsx` que los usaban: `SubmitJobPanel` pasa a usar `uploadFiles` + `submitJob` con la clave; `BatchPanel` se elimina (lo reemplaza el caso).

- [x] **Paso 2: `useCases.js`** — polling cada 2 s de `listCases()`; `openCase(id)` que hace polling de `getCase(id)` cada 1 s mientras no sea terminal y al cerrar pide `getCaseReport(id)` una vez. Devuelve `{ cases, loading, error, selected, report, openCase, closeCase, refresh }`. Detener el polling al desmontar.

- [x] **Paso 3: Verificar** — en la consola del navegador: `import('/src/api.js')` no aplica en Vite; verificar con la pestaña de la Task 3.4.
- [x] **Paso 4: Commit** con la Task 3.4.

---

### Task 3.4: `CasesPanel` — la lista de casos

**Files:** Create `CasesPanel.jsx` + `.module.css`; Modify `app.jsx` (pestaña "Casos" como primera; `TABS = ['Casos', 'Monitor', 'Enviar', 'Historial']`).

- [x] **Paso 1:** Tabla con columnas **Nombre · Estado · Sub-tareas (resueltas/total) · Prioridad · Creado · Duración**, filtro por estado (`todos | queued | processing | retrying | completed | partially_completed | failed | cancelled`), badge de color por estado (reutilizar los colores de `JobTable`: `completed` verde, `partially_completed` ámbar `#78350f/#fcd34d`, `failed` rojo, `retrying` violeta `#3b0764/#d8b4fe`, `cancelled` gris). Clic en una fila → `openCase(id)` y se muestra `CaseDetail` debajo (Task 3.6). Arriba a la derecha el botón **"+ Nuevo caso"** que abre `SubmitCasePanel` (Task 3.5).
- [x] **Paso 2:** Textos en **español** (la pestaña nueva; las existentes se traducen en la Task 3.8).
- [x] **Paso 3: Verificar** — `npm run dev`; mandar un caso con `client -case` y verlo aparecer en ≤2 s; cambiar el filtro; el conteo de sub-tareas sube en vivo.
- [x] **Paso 4: Commit** — `feat(dashboard): pestaña Casos con lista, filtro y auto-refresh`

---

### Task 3.5: `SubmitCasePanel` — enviar un caso

**Files:** Create `SubmitCasePanel.jsx` + `.module.css`.

- [x] **Paso 1:** Formulario: **Nombre** · **Prioridad** (1-10, default 5) · **Archivos**, con dos formas de agregarlos: (a) *Subir desde esta PC* (`<input type=file multiple>`, muestra tipo detectado por extensión y tamaño), (b) *Elegir del dataset* (lista de `listDataset()` con buscador y checkboxes). Cada archivo elegido muestra la **operación que va a decidir el coordinador** (misma tabla que `internal/cases/router.go`: video→convert, audio→convert_audio, image→thumbnail) y permite cambiarla solo entre las válidas para su tipo. Botón **Enviar caso** → si hay archivos locales, `uploadFiles` primero → `submitCase(name, priority, files)` con `{key, operation?}` → al éxito, cierra el panel y abre el caso nuevo en `CaseDetail`. Errores del servidor (400 con el archivo que no sirve) se muestran tal cual.
- [x] **Paso 2: Verificar** — subir video+audio+imagen desde el navegador, enviar, ver el caso en la lista con 3 sub-tareas; intentar enviar un `.txt` → mensaje `formato no soportado`.
- [x] **Paso 3: Commit** — `feat(dashboard): enviar casos desde el navegador (subida o dataset)`

---

### Task 3.6: `CaseDetail` — sub-tareas, cancelar, reporte

**Files:** Create `CaseDetail.jsx` + `.module.css`.

- [x] **Paso 1:** Cabecera con nombre, estado (badge), prioridad, creado/iniciado/cerrado, duración, y botón **Cancelar** (visible solo si el estado no es terminal; pide confirmación con un segundo clic, no con `confirm()`). Tabla de sub-tareas: **Archivo · Tipo · Operación · Pool · Estado · Progreso · Worker · Duración · Error**. Cuando el caso es terminal: sección **Reporte consolidado** con el `summary` grande, la tabla `by_type_and_operation`, y en cada sub-tarea completada un enlace **Descargar** a `result_url`; enlace **Descargar reporte (JSON)** a `/api/cases/{id}/report`.
- [x] **Paso 2: Verificar** — abrir un caso en curso: el progreso y el worker cambian en vivo; cancelar uno con 6 archivos pesados → estado `cancelled`, las pendientes `cancelled`, la que corría termina; abrir uno cerrado → reporte con enlaces que descargan de MinIO (la `result_url` lleva la IP pública de MinIO, funciona desde otra PC).
- [x] **Paso 3: Commit** — `feat(dashboard): detalle del caso con sub-tareas, cancelación y reporte consolidado`

---

### Task 3.7: Workers con rol y colas por pool

**Files:** Modify `WorkerCard.jsx`, `QueueDepth.jsx`, `useSystemState.js` (pasar `queue_depth.by_pool`).

- [x] **Paso 1:** `WorkerCard` muestra `rol` y chips de pools (`video` `audio` `metadata`); `QueueDepth` agrega tres barras "por pool" debajo de las de prioridad, con el número en espera de cada pool — es la **saturación** que la consigna pide observar.
- [x] **Paso 2: Verificar** — con solo un worker de audio conectado, mandar un caso con video: la barra `video` sube y se queda; conectar uno de video: baja.
- [x] **Paso 3: Commit** — `feat(dashboard): rol y pools por worker; profundidad de cola por pool`

---

### Task 3.8: Español, pestaña "Conectar", limpieza

- [x] **Paso 1:** Traducir las pestañas existentes al español (`Monitor`, `Enviar`, `Historial`), títulos y textos visibles. Agregar en la cabecera un enlace **"Conectar esta PC"** → `/connect` (abre en pestaña nueva).
- [x] **Paso 2:** Eliminar `BatchPanel.jsx` (+ css) y las referencias a `/batch` y `/files`; eliminar `POST /batch` y `GET /files` del coordinador (la consigna descarta explícitamente el modelo de "N jobs sueltos").
- [x] **Paso 3:** `scripts/build-dashboard.ps1` → `dist/` regenerado; `Dockerfile.dashboard` y `nginx.conf` quedan para el modo todo-Docker; en `docker-compose.yml` el servicio `dashboard` pasa a ser opcional (comentado) porque el coordinador ya lo sirve.
- [x] **Paso 4: Commit** — `feat(dashboard): interfaz en español, enlace a /connect, fuera el modo batch`

---

### Task 3.9: HITO — todo desde el navegador

**Files:** Create `tests/ui_case_flow.md` (guion manual con casillas y espacio para capturas; es evidencia para el informe).

- [x] **Paso 1:** Guion: (1) abrir `http://<ip>:8080` desde otra PC · (2) *Conectar esta PC* → el worker aparece en Monitor con su rol · (3) *Casos → Nuevo caso* → subir video, audio e imagen desde esa PC → enviar · (4) ver las 3 sub-tareas con su pool y su worker · (5) el caso cierra `completed`, leer el reporte, descargar un resultado · (6) enviar un caso con un archivo corrupto → `partially_completed` con el error visible · (7) enviar 6 pesados, cancelar → `cancelled` · (8) capturas de cada paso en `docs/img/`.
- [x] **Paso 2:** Ejecutarlo con Leno en la PC de lila (o en la propia laptop). Todas las casillas marcadas.
- [x] **Paso 3:** Registrar en `CLAUDE.md`. Commit.

---

# FASE 4 — Dataset real y generación automática de casos  ✅ cerrada 2026-09-11 (hito `tests/dataset_scenario.sh`)

> **Resultado:** 492 archivos / 14.33 GB (250 video · 172 audio · 70 imágenes; 310 livianos · 140 medianos · 42 pesados), `check_manifest.py` OK, `ingest cases --group-by session --limit 10` → 10 casos (6 homogéneos, 4 heterogéneos, 197 sub-tareas) todos `completed`, `HITO OK`. Documentado en `docs/dataset.md`.
>
> **Desvíos respecto a lo planeado:** (1) el generador v2 no era reproducible (bash ≥ 5.1 re-siembra `$RANDOM` en cada subshell) ni respetaba los tamaños (libvpx dobla el bitrate con `-minrate/-maxrate`; x264 sin `nal-hrd=cbr` se queda corto en contenido simple) → reescrito como v3 con PRNG propio, tamaño objetivo + verificación con `stat` y un reintento; se regeneró todo desde cero. (2) 4.2 (clips públicos) **no se hizo**: era opcional, obligaba a descargar cientos de MB por máquina y la consigna no lo pide; `docs/dataset.md` lo explica. (3) Dos bugs del sistema encontrados con la carga real y arreglados: `POST /cases` con decenas de archivos superaba el `WriteTimeout` de 10 s (cliente recibía EOF con el caso ya creado) → plazo propio de 5 min en `submitCase`; y un avance de progreso rezagado devolvía a `running` una sub-tarea ya `completed` (el caso quedaba en `processing` y a los 15 min se marcaba vencida) → `jobProgress` ya no regresa estados terminales; y `by_pool` del dashboard mostraba `-1` bajo carga porque Redis pierde el `lag` del consumer group tras `XDEL` (go-redis lo entrega como -1) → `queue.waiting` cuenta a mano con `XRANGE` desde el último id entregado. Verificado con `ingest load --cases 20 --concurrency 5 --wait`: 20 casos cerrados `completed` en 26 min, y con `--cases 10` `by_pool.video` = 93 sostenido con los 3 workers al 100 % de CPU. (4) `--group-by` acepta también `type` y `tier`; `--homogeneous-only/--heterogeneous-only` se llaman `--only homogeneous|heterogeneous`.

**Hito:** `dataset/` contiene 400-600 archivos con audio, video **e imágenes**, en tres tamaños reales (livianos < 5 MB · medianos 20-50 MB · pesados 150-400 MB), con metadatos por archivo (`evento`, `sesion`, `lote`, `usuario`); `cmd/ingest` los sube y crea **automáticamente** al menos un lote de casos homogéneos y uno de heterogéneos; el generador de carga lanza N casos concurrentes y en el dashboard se ve la saturación por pool. `docs/dataset.md` documenta composición, criterios y volumen.

### Task 4.1: Generador v2 (`dataset/scripts/generate_dataset.sh`)  ✅ (v3, ver desvíos)
- Contenido visual real: `testsrc2`, `mandelbrot`, `life`, `cellauto` de ffmpeg (no color plano) a 1080p, bitrate 6-12 Mb/s; audio con `anoisesrc`/`sine` mezclados.
- **Tres niveles por tamaño real**, no por duración: liviano (5-20 s, 480p), mediano (60-120 s, 720p), pesado (5-10 min, 1080p). Verificar con `stat` que caen en los rangos y abortar si no.
- Imágenes: `mandelbrot`/`testsrc2` a 1 frame en jpg/png/webp, 60-80 imágenes.
- Distribución objetivo (≈520 archivos, 8-15 GB): 260 video (mp4/mkv/avi/mov/webm) · 190 audio (mp3/wav/flac/aac/ogg) · 70 imágenes.
- Manifest v2: por archivo `{filename, key, type, format, size_bytes, duration_s, tier, event, session, batch, user}` — los metadatos de agrupación se asignan de forma determinista (p. ej. `event = boda|concierto|clase|entrevista`, `session = e{n}-s{k}`, `batch = lote-{fecha}-{n}`).
- Semilla fija (`--seed 42`) para que el dataset sea reproducible en cualquier máquina.
- **Verificación:** `bash dataset/scripts/generate_dataset.sh --seed 42` termina; `python dataset/scripts/check_manifest.py` imprime la composición y valida rangos, cantidades mínimas (≥400) y que haya ≥ 3 valores por cada criterio de agrupación.

### Task 4.2: Videos reales de dominio público (opcional pero recomendado)  ⏭ no se hizo (ver desvíos)
- `dataset/scripts/fetch_public.sh`: descarga 3-5 clips (Big Buck Bunny, Sintel, Tears of Steel — CC BY) y los registra en el manifest con `event=publico`. Documentar la licencia en `docs/dataset.md`.

### Task 4.3: `cmd/ingest` — ingesta y generación automática de casos  ✅
- `ingest upload --dir dataset/files --manifest dataset/manifest.json` → sube a MinIO `dataset/` con clave = `filename` (reanudable: salta lo que ya existe con el mismo tamaño).
- `ingest cases --group-by event|session|batch|folder --priority 5 [--dry-run] [--limit N]` → lee el manifest, agrupa, y por cada grupo hace `POST /cases` con nombre `<criterio>=<valor>`; imprime tabla grupo → nº archivos → tipos → id de caso. `--homogeneous-only` (grupos de un solo tipo) y `--heterogeneous-only`.
- `ingest load --concurrency N --cases M --group-by session` → **generador de carga**: M casos enviados con N en paralelo, imprime tiempos de creación y, al final, `GET /cases` con conteo por estado.
- Tests unitarios de la agrupación (función pura sobre el manifest).
- **Verificación:** `ingest cases --group-by event --dry-run` muestra ≥ 4 casos; `--group-by session` produce casos homogéneos y heterogéneos (el manifest lo garantiza por construcción); `ingest load --cases 20 --concurrency 5` y en el dashboard `by_pool.video` > 0 sostenido, workers al 100 % de CPU.

### Task 4.4: `docs/dataset.md`  ✅
- Composición (tabla por tipo/formato/tier con conteos y GB), criterios de agrupación en casos, volumen total, cómo regenerarlo, licencias de los clips públicos, y cómo se usa para carga por lotes y análisis de tiempos.

### Task 4.5: HITO  ✅ `HITO OK` 2026-09-11 02:52
- `tests/dataset_scenario.sh`: sube el dataset (o verifica que está), crea casos por `session` con `--limit 10`, espera a que cierren, y comprueba: ≥ 1 homogéneo `completed`, ≥ 1 heterogéneo `completed` o `partially_completed`, todas las sub-tareas con `worker_id`. `HITO OK`.

---

# FASE 5 — Monitoreo completo

**Hito:** Grafana muestra, durante una carga de 20 casos: CPU/memoria por worker (incluidos los remotos), sub-tareas activas por pool, casos por estado, y profundidad de cola por pool; el dashboard agrupa las sub-tareas activas/en espera por caso.

### Task 5.1: `/metrics` en el coordinador
- Los workers remotos ya no exponen puerto: Prometheus no puede *scrapear*los. El coordinador expone `GET /metrics` con: `mediacase_worker_cpu_percent{worker,role}`, `mediacase_worker_mem_percent{worker,role}`, `mediacase_worker_active_jobs{worker,role}` (desde el heartbeat), `mediacase_queue_depth{pool,priority}`, `mediacase_cases{status}`, `mediacase_jobs{status,pool}`, `mediacase_case_duration_seconds` (histograma al cerrar).
- `infra/prometheus.yml`: un solo target, `host.docker.internal:8080` (Prometheus corre en Docker, el coordinador nativo).
- **Verificación:** `curl localhost:8080/metrics | grep mediacase_` ≥ 6 familias; en `localhost:9090` la query `mediacase_worker_cpu_percent` devuelve lila.

### Task 5.2: Grafana
- `infra/grafana/dashboards/mediacase.json` con paneles: workers (CPU/mem/activos), colas por pool, casos por estado, duración de casos p50/p95, throughput (sub-tareas/min).
- **Verificación:** capturas durante `ingest load`.

### Task 5.3: Sub-tareas agrupadas por caso en el monitoreo
- `GET /stats` devuelve además `by_case: [{case_id, name, status, running, pending, completed, failed}]` para los casos no terminales; el snapshot WS lo incluye; `Monitor` del dashboard muestra una tarjeta "Casos activos" con esa agrupación.
- **Verificación:** con 3 casos en curso, la tarjeta muestra los 3 con sus contadores cambiando.

### Task 5.4: HITO
- `ingest load --cases 20 --concurrency 5` con 3 workers (host + lila + VM o segundo worker local): capturas de Grafana y del dashboard con saturación (`by_pool` > 0 durante > 30 s) y redistribución (matar un worker: sus sub-tareas pasan a otro; se ve en la gráfica de activos por worker).

---

# FASE 6 — Documentación y evidencia

**Hito:** los seis entregables existen en el repo y alguien que no participó levanta el sistema siguiendo el manual.

### Task 6.1: `README.md` en español, completo
- Qué es (con el concepto de caso), arquitectura en 10 líneas, cómo levantar node-1 (3 comandos), cómo sumar un worker (`/connect` o ZIP), cómo enviar un caso (dashboard / `client -case` / `ingest`), cómo correr las pruebas, estructura del repo, estado del proyecto, equipo.

### Task 6.2: `docs/manual-usuario.md`
- Con capturas (`docs/img/`): dashboard y sus pestañas, enviar un caso, leer un reporte, cancelar, conectar una PC como worker (incluye **Smart App Control** y **Desbloquear** en Windows 11, y `apt install ffmpeg` en Linux), qué hacer si el worker no conecta, cómo apagar todo.

### Task 6.3: `docs/architecture.md` reescrito en español
- Diagramas Mermaid: componentes y nodos; flujo caso → routing → colas → workers → barrier → reporte; secuencia de una sub-tarea; ciclo de vida del caso (7 estados) y de la sub-tarea (6); topología de despliegue (node-1 + nodos remotos + túnel). Decisiones justificadas: Go, Redis Streams por pool, PostgreSQL como verdad, MinIO centralizado (y por qué node-1 es SPOF aceptado), canal saliente, pools especializados (Unidad 1), reclaim por instancia. Tabla de puertos y variables de entorno.

### Task 6.4: `docs/api.md`
- Todos los endpoints con request/response de ejemplo (casos, jobs, workers, upload, dataset, connect, metrics, ws).

### Task 6.5: `docs/informe-pruebas.md`
- `tests/measure_times.sh` adaptado a casos (tiempos por sub-tarea y por caso, p50/p90/p99 por operación y por pool). Secciones con **números reales** y capturas: (1) carga por lotes (`ingest load`), (2) distribución (3 nodos, quién procesó qué), (3) casos heterogéneos (los 3 hitos), (4) comportamiento ante fallos (caída de worker con reinicio rápido y con expulsión; reinicio del coordinador; archivo corrupto; cancelación), (5) saturación y redistribución (Fase 5), (6) prueba en hardware real (lila) y en las 3 laptops (Fase 7). Reemplaza `docs/test_and_demo.md`.

### Task 6.6: HITO
- Un compañero (Jennifer o Jonathan) clona el repo en su laptop y, **solo con el manual**, levanta un worker que aparece en el dashboard de Leno. Anotar cuánto tardó y qué tuvo que preguntar → corregir el manual.

---

# FASE 7 — Despliegue final

**Hito:** el sistema corre con ≥ 3 nodos worker en máquinas distintas, es accesible desde otra red por el túnel, y las 3 laptops del equipo lo ejecutaron con evidencia.

### Task 7.1: Vagrant (Task 0.6 pendiente del Plan 1)
- Leno instala Vagrant (`! winget install --id Hashicorp.Vagrant ...`); `vagrant up` en `infra/vagrant`; `bash redeploy.sh` tras compilar; `tests/pools_scenario.sh` con node1 (host, video) + node2 (audio) + node3 (metadata) → `HITO OK`. Esto da el "mínimo 3 nodos" sin depender de nadie.

### Task 7.2: Cloudflare quick tunnel
- `winget install Cloudflare.cloudflared`; `cloudflared tunnel --url http://localhost:8080` → URL `https://xxx.trycloudflare.com`; desde otra red (datos del celular): abrir el dashboard, `/connect`, bajar el ZIP, conectar un worker. El `worker.env` generado usa `https://…` → el worker abre `wss://`. Probar descarga pesada (`pesado.mp4`) desde MinIO vía túnel; si el límite de Cloudflare la corta, documentar que los workers remotos deben tener MinIO accesible por IP (Tailscale) o usar presigned URLs por el túnel — decidir según el resultado.
- `scripts/tunnel.ps1` que arranca el túnel e imprime la URL.

### Task 7.3: Binario en Arch
- En la VM "Arch Linux" existente: copiar `bin/worker-linux-amd64` + `worker.env`, `pacman -S ffmpeg`, ejecutar → aparece en el dashboard. Evidencia de que los compañeros con Arch no tendrán sorpresas.

### Task 7.4: Las 3 laptops físicas
- Sesión con Jennifer y Jonathan (mismo WiFi o túnel): cada uno baja el ZIP de Linux, `bash start-worker.sh`; correr `pools_scenario.sh` con roles asignados (`WORKER_ROLE` en el `.env` de cada uno: uno audio, uno metadata; Leno video); capturas del dashboard con 3 hostnames/IPs distintas; `measure_times.sh`; todo al informe.

### Task 7.5: Firma del ejecutable (decisión)
- Documentar en el manual que el `.exe` no está firmado y qué implica (SmartScreen, Smart App Control). Si el equipo quiere evitarlo: certificado de firma (~$70-300/año) — fuera del alcance; decisión registrada.

### Task 7.6: Cierre
- Checklist de la rúbrica (los 8 rubros, qué evidencia hay de cada uno, dónde está). `CLAUDE.md` §15 actualizado. Decidir con Leno el `git push` (primer push de todo el trabajo) y agregar a Jennifer y Jonathan como colaboradores.

---

## Auto-revisión contra la consigna (qué queda cubierto al terminar el Plan 2)

| Requisito | Fase / tarea |
|---|---|
| Dashboard con vista por caso y por sub-tarea; enviar casos | 3.4, 3.5, 3.6 |
| Dashboard: comportamiento de carga | 3.7, 5.2 |
| Monitoreo: CPU/mem/carga por worker (incluidos remotos) | 5.1 |
| Monitoreo: sub-tareas activas/en espera agrupadas por caso | 5.3 |
| Dataset 400-600, audio+video (+imágenes), formatos, **tamaños reales**, metadatos | 4.1, 4.2 |
| Dataset organizado en casos homogéneos y heterogéneos | 4.1 (manifest), 4.3 |
| Generación automática de casos (carpeta / metadatos) | 4.3 |
| Cliente: generación de casos concurrentes (carga por lotes) | 4.3 (`ingest load`) |
| Análisis de tiempos por sub-tarea y por caso | 6.5 |
| Observar saturación y redistribución | 5.4 |
| Documentar composición del dataset | 4.4 |
| Documento de arquitectura con diagramas | 6.3 |
| Manual de usuario | 6.2 |
| Documentación técnica (despliegue, API) | 6.1, 6.4 |
| Informe de pruebas (carga, distribución, heterogéneos, comportamiento) | 6.5 |
| ≥ 3 nodos worker en entidades separadas | 7.1, 7.4 |
| Justificar tecnología y repositorio de resultados | 6.3 |

**Lo que ya cubre el Plan 1** (no se repite aquí): casos, routing, barrier, reporte, los 7 estados, pools especializados y su justificación, canal saliente, worker descargable, hitos 0.7/0.9/1.11/2.6.
