# Plan de implementación — Distribución real, capa de casos y pools especializados

> **Para quien ejecute este plan:** usar la skill `superpowers:executing-plans` tarea por tarea, en orden. Cada paso lleva casilla `- [ ]`. Ninguna tarea se da por terminada sin correr su verificación. **Ningún `git push` sin luz verde explícita de Leno.**

**Objetivo:** convertir la plataforma actual (procesamiento por archivo suelto, todo en una máquina) en lo que pide la consigna v2.0: casos heterogéneos con routing por tipo, barrier/join, reporte consolidado, y workers reales corriendo en máquinas separadas.

**Arquitectura:** un nodo coordinador (Postgres + Redis + MinIO en Docker; coordinador y dashboard como procesos) y N nodos worker (un binario Go estático + ffmpeg, sin Docker) que se conectan por red. El coordinador inspecciona cada archivo del caso, decide la operación, encola cada sub-tarea en la cola de su pool, y cierra el caso con un barrier cuando todas las sub-tareas resolvieron.

**Stack:** Go 1.26 · PostgreSQL 16 · Redis 7 (Streams) · MinIO · ffmpeg · React (solo dashboard) · VirtualBox 7.2 + Vagrant (nodos de prueba) · Docker Desktop (solo infraestructura del nodo 1).

**Spec:** `../../../ProyectoProgramadoI_PlataformaMultimediaCasos_v2.docx` (resumen operativo en `CLAUDE.md` §1–§13).

## Restricciones globales (copiadas de la consigna)

- *"al menos tres nodos worker ejecutándose de forma concurrente, los cuales deberán desplegarse en entidades de ejecución separadas"* — comunicación **por red**, no solo memoria compartida.
- *"no se evaluará (positivamente) un modelo de casos que sea únicamente una etiqueta de agrupación sobre archivos idénticos"* — `POST /batch` que crea N jobs sueltos **no cumple**.
- *"el coordinador deberá determinar el estado agregado únicamente cuando disponga del resultado de todas las sub-tareas del caso (patrón barrier/join)"*.
- *"un caso se considera completed si todas sus sub-tareas finalizaron exitosamente, y partially_completed si finalizó con al menos una sub-tarea fallida"*.
- Estados por sub-tarea: `pendiente, asignado, en ejecución, completado, fallido`. Estados por caso: `queued, processing, completed, partially_completed, failed, retrying, cancelled`.
- Routing por tipo: *"la operación NO puede venir siempre dictada por el cliente"* — el cliente puede sugerirla, el coordinador decide.
- Toda decisión tecnológica debe **justificarse** por escrito (`docs/architecture.md`).
- Convenciones del repo: comentarios en español en el código de `cmd/`, inglés donde ya estaba en inglés; `.sh` con LF (ver `.gitattributes`); no commitear `dataset/files/` ni `node_modules/`.

---

## Alcance de este plan y qué queda para el Plan 2

Este plan cubre **Fase 0, Fase 1 y Fase 2**. Son las que concentran el 55 % de la rúbrica y las que desbloquean todo lo demás. Cada fase termina en un **hito** verificable de punta a punta.

| Fase | Qué entrega | Rubros que toca |
|---|---|---|
| **0 — Distribución real** | Un worker en una VM procesa un archivo que le asignó el coordinador desde el host | Implementación distribuida 20 % |
| **1 — Capa de casos** | `POST /cases`, routing por tipo, barrier/join, reporte consolidado, los 7 estados | Casos y concurrencia 20 % · Casos y resultados 10 % |
| **2 — Pools especializados** | `worker-video` / `worker-audio` / `worker-metadata`, colas por pool, justificación Unidad 1 | Monitoreo y balanceo 15 % · Arquitectura 15 % |

**Compromisos ya acordados para el Plan 2** (2026-09-10, con Leno):
- **Dataset (Fase 3):** el generador actual produce videos de color sólido — el "largo" pesa 0.6 MB y ffmpeg lo convierte en 1 s, así que no hay archivos pesados ni saturación observable. Se rehace con contenido visual real (`testsrc2`, `mandelbrot`, ruido) a 1080p y bitrate alto, **tres niveles por tamaño** (livianos < 5 MB, medianos 20–50 MB, pesados 150–400 MB), unos pocos videos reales de dominio público, **imágenes** (la consigna las menciona en casos heterogéneos), y agrupación en casos homogéneos/heterogéneos con el criterio documentado. Volumen esperado: 8–15 GB.
- **Métricas (Fase 2 o 4):** con la conexión saliente, Prometheus ya no puede *scrapear* a workers remotos. El coordinador expone `/metrics` con los gauges por worker que le llegan por heartbeat, y Prometheus scrapea solo al coordinador.
- **Acceso desde cualquier red (Fase 6):** Cloudflare Tunnel (túnel rápido `trycloudflare.com` para la demo; dominio barato si se quiere URL fija). Probar descargas pesadas por el túnel; los workers en LAN bajan de MinIO por IP local.

**Plan 2** (se escribe al cerrar la Fase 2, con lo aprendido): Fase 2.5 worker descargable con conexión saliente (WebSocket) · Fase 3 dataset en casos + app de ingesta (generación automática) · Fase 4 vista por caso en el dashboard · Fase 5 manual, informe de pruebas con mediciones reales, diagramas · Fase 6 prueba en las 3 laptops físicas.

---

## Mapa de archivos

```
cmd/coordinator/main.go          modificar  conectar barrier, MinIO, compose infra
cmd/worker/main.go               modificar  quitar Postgres · canal saliente WS · descarga de MinIO · WORKER_ROLE
cmd/worker/main_test.go          crear      pruebas de loadConfig y capabilities
cmd/client/main.go               modificar  modo -case
internal/models/job.go           modificar  CaseID, FileType, Pool, StatusCancelled
internal/models/case.go          crear      Case, CaseStatus, FileType
internal/db/db.go                modificar  migración cases + columnas nuevas en jobs
internal/db/cases.go             crear      consultas de casos
internal/cases/router.go         crear      DetectFileType, Route, PoolFor  (puro)
internal/cases/router_test.go    crear
internal/cases/barrier.go        crear      ComputeStatus (puro) + Barrier.OnJobResolved (tx)
internal/cases/barrier_test.go   crear
internal/cases/report.go         crear      BuildReport, Summary  (puro)
internal/cases/report_test.go    crear
internal/coordinator/api.go      modificar  rutas nuevas, jobProgress con started_at, submitJob con routing
internal/coordinator/cases_api.go crear     handlers de /cases
internal/coordinator/worker_hub.go crear    canal WebSocket por worker (Task 0.1)
internal/coordinator/download.go  crear    GET /connect y GET /download/worker (Task 0.8)
internal/coordinator/registry.go modificar  Capabilities, LeastLoadedFor(pool)
internal/coordinator/scheduler.go modificar  dispatch por pool, hooks del barrier, skip cancelados
internal/coordinator/ws.go       modificar  ByPool en QueueDepthSnapshot
internal/queue/queue.go          modificar  streams por pool
internal/storage/minio.go        modificar  UploadObject, Download, bucket dataset
docker-compose.infra.yml         crear      solo postgres + redis + minio (+ prometheus/grafana)
docker-compose.yml               modificar  workers sin DATABASE_URL ni volumen dataset; WORKER_ROLE
scripts/build-workers.ps1        crear      compilación cruzada linux/windows
scripts/run-coordinator.ps1      crear      coordinador nativo en el host
scripts/run-worker.ps1           crear      worker-video nativo en el host
scripts/firewall-node1.ps1       crear      regla de firewall (requiere admin)
infra/env/node1.env.example      crear
infra/env/worker.env.example     crear
infra/vagrant/Vagrantfile        crear      node2 y node3
infra/vagrant/provision_worker.sh crear     ffmpeg + systemd unit
infra/vagrant/redeploy.sh        crear      copiar binario nuevo y reiniciar
tests/distributed_smoke.sh       crear      hito Fase 0
tests/case_scenario.sh           crear      hito Fase 1
tests/pools_scenario.sh          crear      hito Fase 2
docs/architecture.md             modificar  sección "Modelo de asignación (Unidad 1)"
```

## Convenciones de prueba

- **Unitarias (Go):** `go test ./internal/cases/... ./cmd/worker/...` — no necesitan Docker ni red.
- **Multimedia:** `go test ./internal/multimedia/...` — necesita `ffmpeg` en el PATH.
- **Integración local:** `docker compose -f docker-compose.infra.yml up -d` + coordinador nativo + worker nativo, y luego el script `.sh` del hito, corrido desde **Git Bash**.
- **Integración distribuida:** lo mismo, pero el worker corre en `node2`/`node3` (Vagrant).
- Los scripts `.sh` terminan con `echo "HITO OK"` y salida `0` si todo pasó; cualquier `exit 1` es fallo.

---

# FASE 0 — Distribución real

**Hito:** un worker corriendo en **otra máquina física** — la PC con Windows 11 de la novia de Leno, en el mismo WiFi — completa un job que el coordinador (laptop de Leno) le asignó por el canal saliente, con el archivo de entrada bajado de MinIO. En esa PC **no se configura nada**: se descarga el ZIP desde `http://<ip-de-leno>:8080/connect`, se descomprime y se hace doble clic. Las VMs de Vagrant (Task 0.6) son la **segunda** validación y sirven para probar 3 nodos a la vez sin depender de otra PC.

### Task 0.0: Herramientas locales en el host ✅ 2026-09-10 (Go 1.27 y ffmpeg 9 portables en %LOCALAPPDATA%\Programs, sin admin; Vagrant pendiente)

**Files:** ninguno del repo. Instala Go, ffmpeg y Vagrant en Windows.

- [x] **Paso 1: Pedir OK a Leno** — instala software fuera del repo.
- [x] **Paso 2: Instalar**

```powershell
winget install --id GoLang.Go --exact --accept-package-agreements --accept-source-agreements
winget install --id Gyan.FFmpeg --exact --accept-package-agreements --accept-source-agreements
winget install --id Hashicorp.Vagrant --exact --accept-package-agreements --accept-source-agreements
```

- [x] **Paso 3: Abrir una terminal nueva (para que tome el PATH) y verificar**

```powershell
go version        # esperado: go version go1.26.x windows/amd64
ffmpeg -version   # esperado: ffmpeg version 7.x
vagrant --version # esperado: Vagrant 2.4.9
```

- [x] **Paso 4: Correr las pruebas que ya existen, para tener línea base**

```powershell
go test ./...
```
Esperado: `ok` en `internal/multimedia` (usa ffmpeg). Si falla algo aquí, es un problema del entorno, no del plan — resolverlo antes de seguir.

---

### Task 0.1: Conexión saliente — el worker abre el canal, el coordinador le manda las tareas por ahí ✅ 2026-09-10

Hoy el coordinador hace `POST http://<worker>:8090/tasks` (`scheduler.go:120`): necesita **entrar** a la máquina del worker, lo que exige puerto abierto y regla de firewall en cada nodo. Se invierte: el worker abre un WebSocket **hacia** el coordinador (`GET /workers/{id}/stream`) y lo mantiene vivo; el coordinador envía las asignaciones por ese canal. El worker no escucha ningún puerto para recibir trabajo. Heartbeat y progreso siguen por HTTP (también salientes).

**Files:**
- Create: `internal/coordinator/worker_hub.go` — canal por worker
- Create: `internal/coordinator/worker_hub_test.go`
- Modify: `internal/coordinator/api.go` — ruta `GET /workers/{id}/stream`
- Modify: `internal/coordinator/scheduler.go` — `sendToWorker` usa el hub
- Modify: `cmd/worker/main.go` — `streamLoop` con reconexión; se elimina el handler `/tasks`
- Modify: `cmd/coordinator/main.go` — cablear el hub

**Interfaces (Produces):**

```go
// Mensajes coordinador → worker y worker → coordinador (JSON sobre WebSocket)
type wsMsg struct {
	Type   string      `json:"type"`             // "assign" | "accept" | "reject" | "ping" | "pong"
	Job    *models.Job `json:"job,omitempty"`    // en "assign"
	JobID  string      `json:"job_id,omitempty"` // en "accept"/"reject"
	Reason string      `json:"reason,omitempty"` // en "reject"
}

type WorkerHub struct{ ... }
func NewWorkerHub() *WorkerHub
func (h *WorkerHub) ServeStream(w http.ResponseWriter, r *http.Request)  // GET /workers/{id}/stream
func (h *WorkerHub) IsConnected(workerID string) bool
// Assign envía la sub-tarea y espera accept/reject (timeout 5 s).
// ErrWorkerBusy si rechaza (equivale al 429 de hoy); ErrNotConnected si no hay canal.
func (h *WorkerHub) Assign(ctx context.Context, workerID string, job *models.Job) error
```

- [x] **Paso 1: `worker_hub.go`**

```go
package coordinator

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"golang.org/x/net/websocket"
)

var (
	ErrNotConnected = errors.New("worker sin canal abierto")
	ErrWorkerBusy   = errors.New("worker rechazó la sub-tarea (pool lleno)")
)

type wsMsg struct {
	Type   string      `json:"type"`
	Job    *models.Job `json:"job,omitempty"`
	JobID  string      `json:"job_id,omitempty"`
	Reason string      `json:"reason,omitempty"`
}

type workerConn struct {
	conn    *websocket.Conn
	sendMu  sync.Mutex            // un escritor a la vez
	pending map[string]chan wsMsg // job_id → respuesta accept/reject
	pendMu  sync.Mutex
}

type WorkerHub struct {
	mu    sync.RWMutex
	conns map[string]*workerConn
}

func NewWorkerHub() *WorkerHub { return &WorkerHub{conns: make(map[string]*workerConn)} }

// ServeStream: el worker se conecta aquí y se queda. Cada mensaje que manda se despacha
// a quien esté esperando la respuesta de esa sub-tarea.
func (h *WorkerHub) ServeStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	websocket.Handler(func(c *websocket.Conn) {
		wc := &workerConn{conn: c, pending: make(map[string]chan wsMsg)}
		h.mu.Lock()
		if old, ok := h.conns[id]; ok {
			old.conn.Close() // reconexión: cerrar el canal viejo
		}
		h.conns[id] = wc
		h.mu.Unlock()
		log.Printf("[whub] worker %s conectó desde %s", id, r.RemoteAddr)

		for {
			var m wsMsg
			if err := websocket.JSON.Receive(c, &m); err != nil {
				break
			}
			switch m.Type {
			case "accept", "reject":
				wc.pendMu.Lock()
				ch, ok := wc.pending[m.JobID]
				wc.pendMu.Unlock()
				if ok {
					ch <- m
				}
			case "ping":
				wc.send(wsMsg{Type: "pong"})
			}
		}

		h.mu.Lock()
		if h.conns[id] == wc {
			delete(h.conns, id)
		}
		h.mu.Unlock()
		log.Printf("[whub] worker %s desconectó", id)
	}).ServeHTTP(w, r)
}

func (wc *workerConn) send(m wsMsg) error {
	wc.sendMu.Lock()
	defer wc.sendMu.Unlock()
	wc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return websocket.JSON.Send(wc.conn, m)
}

func (h *WorkerHub) IsConnected(workerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[workerID]
	return ok
}

func (h *WorkerHub) Assign(ctx context.Context, workerID string, job *models.Job) error {
	h.mu.RLock()
	wc, ok := h.conns[workerID]
	h.mu.RUnlock()
	if !ok {
		return ErrNotConnected
	}
	reply := make(chan wsMsg, 1)
	wc.pendMu.Lock()
	wc.pending[job.ID] = reply
	wc.pendMu.Unlock()
	defer func() {
		wc.pendMu.Lock()
		delete(wc.pending, job.ID)
		wc.pendMu.Unlock()
	}()

	if err := wc.send(wsMsg{Type: "assign", Job: job}); err != nil {
		return err
	}
	select {
	case m := <-reply:
		if m.Type == "reject" {
			return ErrWorkerBusy
		}
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("worker no respondió a la asignación")
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

- [x] **Paso 2: Ruta y scheduler** — en `Router()`: `mux.HandleFunc("GET /workers/{id}/stream", a.workerHub.ServeStream)`. `API` y `Scheduler` reciben `*WorkerHub`. En `scheduler.go`, `sendToWorker` pasa a:

```go
func (s *Scheduler) sendToWorker(ctx context.Context, worker *models.WorkerInfo, job *models.Job) error {
	return s.workerHub.Assign(ctx, worker.ID, job)
}
```
y en `dispatch`, la rama del `429` se convierte en `if errors.Is(err, ErrWorkerBusy)` (mismo tratamiento: re-encolar sin contar reintento). Antes de `Dequeue`, saltar workers sin canal: `if worker == nil || !s.workerHub.IsConnected(worker.ID) { return fmt.Errorf("no workers available") }`.

- [x] **Paso 3: Worker — `streamLoop`** en `cmd/worker/main.go`. Reemplaza al handler `/tasks` (borrar `handleAssign` y su `mux.HandleFunc("/tasks", ...)`; el servidor HTTP del worker queda solo para `/health` y `/metrics`, que son opcionales):

```go
// streamLoop mantiene el canal con el coordinador. Si se cae, reintenta con espera creciente.
func (w *worker) streamLoop(ctx context.Context) {
	wsURL := strings.Replace(strings.Replace(w.cfg.coordinatorURL, "https://", "wss://", 1), "http://", "ws://", 1)
	wsURL += "/workers/" + w.cfg.workerID + "/stream"
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		conn, err := websocket.Dial(wsURL, "", w.cfg.coordinatorURL)
		if err != nil {
			log.Printf("[stream] no se pudo conectar (%v); reintento en %s", err, backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		log.Printf("[stream] canal abierto con %s", w.cfg.coordinatorURL)
		w.serveStream(ctx, conn)
		conn.Close()
		log.Printf("[stream] canal cerrado; reconectando")
	}
}

func (w *worker) serveStream(ctx context.Context, conn *websocket.Conn) {
	var sendMu sync.Mutex
	send := func(m wsMsg) {
		sendMu.Lock()
		defer sendMu.Unlock()
		websocket.JSON.Send(conn, m)
	}
	for {
		var m wsMsg
		if err := websocket.JSON.Receive(conn, &m); err != nil {
			return
		}
		if m.Type != "assign" || m.Job == nil {
			continue
		}
		job := jobAssignment{JobID: m.Job.ID, FilePath: m.Job.FilePath,
			Operation: string(m.Job.Operation), Priority: m.Job.Priority}
		select {
		case w.jobCh <- job:
			log.Printf("[assign] job %s aceptado (op=%s)", job.JobID, job.Operation)
			send(wsMsg{Type: "accept", JobID: job.JobID})
		default:
			log.Printf("[assign] pool lleno, rechazando job %s", job.JobID)
			send(wsMsg{Type: "reject", JobID: job.JobID, Reason: "pool lleno"})
		}
	}
}
```
(`wsMsg` se define también en el worker con la misma forma.) En `main()`: después de `register()`, `go w.streamLoop(ctx)`. El `register()` ya no manda `hostname` con puerto: manda `"hostname": hostnameOf()` (`os.Hostname()`), solo informativo para el dashboard. **`WORKER_ADVERTISE_ADDR` no existe** — no se agrega a config, `.env` ni `provision_worker.sh`.

- [x] **Paso 4: Prueba unitaria del hub** — `internal/coordinator/worker_hub_test.go`:

```go
package coordinator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"golang.org/x/net/websocket"
)

func TestWorkerHub_AssignAcceptReject(t *testing.T) {
	hub := NewWorkerHub()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /workers/{id}/stream", hub.ServeStream)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j0"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("sin canal: %v", err)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/workers/w1/stream"
	c, err := websocket.Dial(wsURL, "", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	go func() { // worker falso: acepta j1, rechaza j2
		for {
			var m wsMsg
			if websocket.JSON.Receive(c, &m) != nil {
				return
			}
			reply := "accept"
			if m.Job != nil && m.Job.ID == "j2" {
				reply = "reject"
			}
			websocket.JSON.Send(c, wsMsg{Type: reply, JobID: m.Job.ID})
		}
	}()
	time.Sleep(100 * time.Millisecond)
	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j1"}); err != nil {
		t.Fatalf("j1: %v", err)
	}
	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j2"}); !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("j2: %v", err)
	}
}
```

- [x] **Paso 5: Verificar** — `go test ./internal/coordinator/ -run TestWorkerHub -v` → PASS. Luego integración local (Task 0.4 modo nativo): el log del coordinador muestra `[whub] worker node1 conectó`, y un `POST /jobs` se completa. Apagar el coordinador 10 s y volver a arrancarlo: el worker reconecta solo (`canal abierto`).
- [x] **Paso 6: Commit** (pedir OK) — `git commit -am "workers: canal saliente por WebSocket; el coordinador ya no necesita alcanzar al worker"`

---

### Task 0.2: El worker deja de hablar con PostgreSQL

El worker hoy escribe en `jobs` por SQL (`updateDBStatus`) **y además** reporta lo mismo por HTTP (`reportProgress`). Para correr en otra máquina no debe necesitar la base de datos: solo la URL del coordinador.

**Files:**
- Modify: `cmd/worker/main.go` — quitar `database/sql`, `lib/pq`, `dbURL`, campo `db`, `updateDBStatus` y sus 6 llamadas; `newWorker` deja de recibir `db`.
- Modify: `internal/coordinator/api.go:196-215` (`jobProgress`) — ahora es la **única** fuente de verdad, así que debe fijar `started_at`.

**Interfaces:**
- Consumes: `POST /jobs/{id}/progress` con `{progress, status, result_url, error}` (ya existe).
- Produces: `jobProgress` fija `started_at = COALESCE(started_at, NOW())` cuando `status = running`.

- [ ] **Paso 1: Cambiar `jobProgress` en el coordinador**

```go
func (a *API) jobProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		Progress  int    `json:"progress"`
		Status    string `json:"status"`
		ResultURL string `json:"result_url"`
		ErrorMsg  string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	switch payload.Status {
	case string(models.StatusRunning):
		a.db.Exec(`UPDATE jobs SET status='running', progress=$1,
		           started_at=COALESCE(started_at, NOW()) WHERE id=$2`, payload.Progress, id)
	case string(models.StatusCompleted):
		a.db.Exec(`UPDATE jobs SET status='completed', progress=100, result_url=$1,
		           completed_at=NOW() WHERE id=$2`, payload.ResultURL, id)
	case string(models.StatusFailed):
		a.db.Exec(`UPDATE jobs SET status='failed', progress=$1, error_msg=$2,
		           completed_at=NOW() WHERE id=$3`, payload.Progress, payload.ErrorMsg, id)
	case "":
		a.db.Exec(`UPDATE jobs SET progress=$1 WHERE id=$2`, payload.Progress, id)
	default:
		a.db.Exec(`UPDATE jobs SET status=$1, progress=$2 WHERE id=$3`, payload.Status, payload.Progress, id)
	}
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Paso 2: Quitar SQL del worker**

En `cmd/worker/main.go`:
1. Borrar los imports `"database/sql"` y `_ "github.com/lib/pq"`.
2. Borrar `dbURL` de `workerConfig` y de `loadConfig`.
3. Borrar el campo `db *sql.DB` de `worker`; `newWorker(cfg workerConfig, s *storage.MinIOClient) *worker`.
4. Borrar la función `updateDBStatus` completa y sus 6 llamadas dentro de `processJob` (quedan solo las `reportProgress`).
5. En `main()`: borrar el bloque `sql.Open` / `db.Ping` con reintentos; `w := newWorker(cfg, minioClient)`.

- [ ] **Paso 3: Verificar que compila y no queda rastro**

```powershell
go build ./cmd/worker && go vet ./cmd/worker
Select-String -Path cmd/worker/main.go -Pattern "database/sql|updateDBStatus|dbURL|lib/pq"
```
Esperado: compila; la búsqueda devuelve **cero** líneas.

- [ ] **Paso 4: Prueba de integración local** (usa el compose actual, que todavía trae los 3 workers)

```bash
docker compose up --build -d
sleep 15
JOB=$(curl -s -X POST localhost:8080/jobs -H 'Content-Type: application/json' \
  -d '{"file_path":"/app/dataset/files/audio_short_1_mp3.mp3","operation":"convert_audio","priority":5}' | python -c "import sys,json;print(json.load(sys.stdin)['id'])")
sleep 20
docker compose exec postgres psql -U media -d mediacase -tA \
  -c "SELECT status, worker_id, started_at IS NOT NULL, completed_at IS NOT NULL FROM jobs WHERE id='$JOB'"
```
Esperado: `completed|worker-N|t|t` — el job cerró y **tiene `started_at`** aunque el worker ya no toca la base. (Si `dataset/files` está vacío, correr antes `bash dataset/scripts/generate_dataset.sh`.)

- [ ] **Paso 5: Commit** (pedir OK) — `git commit -am "worker: reportar solo por HTTP al coordinador; jobProgress fija started_at"`

---

### Task 0.3: Entradas desde MinIO

El worker deja de leer del disco local. `file_path` pasa a ser la **clave del objeto** en el bucket `dataset` (p. ej. `audio_short_1_mp3.mp3`). El worker la baja a un directorio temporal, procesa, sube el resultado y borra los temporales.

**Files:**
- Modify: `internal/storage/minio.go`
- Modify: `cmd/worker/main.go` (`processJob`)
- Modify: `docker-compose.yml` — quitar `- ./dataset:/app/dataset:ro` de los tres workers.

**Interfaces:**
- Produces:
  - `const DatasetBucket = "dataset"`
  - `func (m *MinIOClient) EnsureBucket(ctx, bucket string) error`
  - `func (m *MinIOClient) UploadObject(ctx, bucket, objectKey, localPath string) error`
  - `func (m *MinIOClient) Download(ctx, bucket, objectKey, destDir string) (localPath string, err error)`
  - `Upload` (resultados) queda igual, implementado sobre `UploadObject`.

- [ ] **Paso 1: Implementar en `internal/storage/minio.go`**

Cambiar `ensureBucket` por una versión pública que reciba el bucket (mantener la política de lectura pública que ya aplica), y agregar:

```go
// DatasetBucket guarda los archivos de entrada de los casos.
const DatasetBucket = "dataset"

// UploadObject sube localPath a bucket/objectKey. Crea el bucket si hace falta.
func (m *MinIOClient) UploadObject(ctx context.Context, bucket, objectKey, localPath string) error {
	if err := m.EnsureBucket(ctx, bucket); err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = m.client.PutObject(ctx, bucket, objectKey, f, fi.Size(),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object %s/%s: %w", bucket, objectKey, err)
	}
	return nil
}

// Download baja bucket/objectKey a destDir y retorna la ruta local.
// El nombre local conserva la extensión original para que ffmpeg la reconozca.
func (m *MinIOClient) Download(ctx context.Context, bucket, objectKey, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", destDir, err)
	}
	local := filepath.Join(destDir, filepath.Base(objectKey))
	if err := m.client.FGetObject(ctx, bucket, objectKey, local, minio.GetObjectOptions{}); err != nil {
		return "", fmt.Errorf("get object %s/%s: %w", bucket, objectKey, err)
	}
	return local, nil
}
```

Y reescribir `Upload` para reutilizarlo:

```go
func (m *MinIOClient) Upload(ctx context.Context, jobID, localPath string) (string, error) {
	objectName := fmt.Sprintf("jobs/%s/%s", jobID, filepath.Base(localPath))
	if err := m.UploadObject(ctx, m.bucket, objectName, localPath); err != nil {
		return "", err
	}
	pubEndpoint := getEnv("MINIO_PUBLIC_ENDPOINT", getEnv("MINIO_ENDPOINT", "minio:9000"))
	return fmt.Sprintf("http://%s/%s/%s", pubEndpoint, m.bucket, objectName), nil
}
```

- [ ] **Paso 2: El worker baja la entrada** — al inicio de `processJob`, antes del `switch`:

```go
	// Bajar la entrada de MinIO a un directorio temporal propio del job.
	inDir := filepath.Join(os.TempDir(), "mediacase-in", job.JobID)
	localInput, dlErr := w.storage.Download(ctx, storage.DatasetBucket, job.FilePath, inDir)
	if dlErr != nil {
		log.Printf("[job %s] descarga FALLÓ: %v", job.JobID, dlErr)
		monitoring.JobsFailed.WithLabelValues(w.cfg.workerID, job.Operation).Inc()
		w.reportProgress(job.JobID, 0, string(models.StatusFailed), "", "descarga de entrada: "+dlErr.Error())
		return
	}
	defer os.RemoveAll(inDir)
```

y en el `switch` usar `localInput` en lugar de `job.FilePath` en las 4 llamadas a `multimedia.*`. Agregar `"path/filepath"` a los imports.

- [ ] **Paso 3: Compose sin volumen de dataset** — en `docker-compose.yml`, en `worker-1`, `worker-2`, `worker-3`, borrar el bloque `volumes:` con `./dataset`. También borrar `DATABASE_URL` de los tres (ya no se usa).

- [ ] **Paso 4: Verificar de punta a punta**

```bash
docker compose up --build -d && sleep 15
# subir una entrada al bucket dataset con el cliente mc que trae la imagen de MinIO
docker compose cp dataset/files/audio_short_1_mp3.mp3 minio:/tmp/in.mp3
docker compose exec minio mc alias set local http://localhost:9000 minioadmin minioadmin
docker compose exec minio mc mb --ignore-existing local/dataset
docker compose exec minio mc cp /tmp/in.mp3 local/dataset/audio_short_1_mp3.mp3
# encolar usando la CLAVE, no una ruta
JOB=$(curl -s -X POST localhost:8080/jobs -H 'Content-Type: application/json' \
  -d '{"file_path":"audio_short_1_mp3.mp3","operation":"convert_audio","priority":5}' | python -c "import sys,json;print(json.load(sys.stdin)['id'])")
sleep 20
curl -s localhost:8080/jobs/$JOB | python -m json.tool | grep -E '"status"|"result_url"'
```
Esperado: `"status": "completed"` y una `result_url` bajo `http://localhost:9000/results/jobs/...`. Abrir esa URL en el navegador y que descargue el archivo.

- [ ] **Paso 5: Commit** (pedir OK) — `git commit -am "storage: bucket dataset, Download/UploadObject; el worker baja la entrada de MinIO"`

---

### Task 0.4: Configuración por entorno para el nodo 1 y los nodos worker

**Files:**
- Create: `docker-compose.infra.yml`, `infra/env/node1.env.example`, `infra/env/worker.env.example`, `scripts/run-coordinator.ps1`, `scripts/run-worker.ps1`, `scripts/firewall-node1.ps1`

**Por qué el coordinador corre nativo y no en Docker:** en Windows, Docker Desktop vive dentro de WSL2 y su salida hacia la red *host-only* de VirtualBox (`192.168.56.x`) no está garantizada. Un proceso nativo está directamente en la red del host. Además es la topología final (ver `CLAUDE.md` §15).

- [ ] **Paso 1: `docker-compose.infra.yml`** — copiar de `docker-compose.yml` **solo** los servicios `redis`, `postgres`, `minio`, `prometheus`, `grafana` y la sección `volumes`. Sin `coordinator`, sin `worker-*`, sin `dashboard`.

- [ ] **Paso 2: `infra/env/node1.env.example`**

```dotenv
# Nodo coordinador (host Windows). Copiar a infra/env/node1.env
DATABASE_URL=postgres://media:media@localhost:5432/mediacase?sslmode=disable
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
PORT=8080
# MinIO visto desde el propio host y desde los otros nodos
MINIO_ENDPOINT=localhost:9000
MINIO_PUBLIC_ENDPOINT=192.168.56.1:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=results
```

- [ ] **Paso 3: `infra/env/worker.env.example`**

```dotenv
# Nodo worker. Copiar a /etc/mediacase/worker.env en la VM (lo hace provision_worker.sh)
WORKER_ID=node2
WORKER_ROLE=all
WORKER_POOL_SIZE=2
COORDINATOR_URL=http://192.168.56.1:8080
MINIO_ENDPOINT=192.168.56.1:9000
MINIO_PUBLIC_ENDPOINT=192.168.56.1:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=results
```

- [ ] **Paso 4: `scripts/run-coordinator.ps1`** — carga el `.env` y corre el binario:

```powershell
# Corre el coordinador como proceso nativo del host, con infra/env/node1.env
$envFile = Join-Path $PSScriptRoot "..\infra\env\node1.env"
if (-not (Test-Path $envFile)) { Write-Error "Falta $envFile (copiar de node1.env.example)"; exit 1 }
Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=' } | ForEach-Object {
    $k, $v = $_ -split '=', 2
    [Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim(), 'Process')
}
Set-Location (Join-Path $PSScriptRoot "..")
go run ./cmd/coordinator
```

`scripts/run-worker.ps1` es idéntico pero lee `infra/env/worker-host.env` (WORKER_ID=node1, COORDINATOR_URL=http://localhost:8080, MINIO_ENDPOINT=localhost:9000) y ejecuta `go run ./cmd/worker`.

- [ ] **Paso 5: `scripts/firewall-node1.ps1`** (correr **una vez**, como administrador; pedir OK a Leno porque toca el sistema)

```powershell
# Permite que otros nodos lleguen al coordinador (8080) y a MinIO (9000) en node-1.
# Solo node-1 necesita esto: los workers NO abren puertos (conexión saliente, Task 0.1).
# Cubre la red host-only de VirtualBox y las redes privadas de casa (192.168.0.0/16, 10.0.0.0/8).
New-NetFirewallRule -DisplayName "MediaCase node-1 (coordinator+minio)" -Direction Inbound `
  -Protocol TCP -LocalPort 8080,9000 -RemoteAddress 192.168.0.0/16,10.0.0.0/8 -Action Allow -Profile Any
```

- [ ] **Paso 6: Verificar el modo "todo nativo en el host"**

```powershell
docker compose -f docker-compose.infra.yml up -d
Copy-Item infra/env/node1.env.example infra/env/node1.env
# terminal 1
.\scripts\run-coordinator.ps1
# terminal 2  (worker-host.env con WORKER_ID=node1)
.\scripts\run-worker.ps1
```
Y en Git Bash repetir el `curl` de la Task 0.3 paso 4 (con la clave ya subida). Esperado: `completed` con `worker_id = node1`.

- [ ] **Paso 7: Commit** (pedir OK) — `git add docker-compose.infra.yml infra/env scripts && git commit -m "infra: compose solo-infraestructura, env de ejemplo y scripts nativos para node-1"`

Agregar a `.gitignore`: `infra/env/*.env` (los `.example` sí se commitean).

---

### Task 0.5: Compilación cruzada

**Files:** Create `scripts/build-workers.ps1`; Modify `Makefile` (targets `build-linux`, `build-windows`); agregar `bin/` a `.gitignore`.

- [ ] **Paso 1: `scripts/build-workers.ps1`**

```powershell
# Compila el worker para Linux (VMs / laptops de los compañeros) y Windows (host).
Set-Location (Join-Path $PSScriptRoot "..")
New-Item -ItemType Directory -Force bin | Out-Null
$env:CGO_ENABLED = "0"
$env:GOOS = "linux";   $env:GOARCH = "amd64"; go build -ldflags="-s -w" -o bin/worker-linux-amd64 ./cmd/worker
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -ldflags="-s -w" -o bin/worker-windows-amd64.exe ./cmd/worker
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Get-ChildItem bin | Select-Object Name, @{n='MB';e={[math]::Round($_.Length/1MB,1)}}
```

- [ ] **Paso 2: Verificar** — `.\scripts\build-workers.ps1` → dos binarios de ~10–15 MB. `file bin/worker-linux-amd64` (Git Bash) → `ELF 64-bit ... statically linked`.
- [ ] **Paso 3: Commit** (pedir OK) — `git add scripts/build-workers.ps1 Makefile .gitignore && git commit -m "build: compilación cruzada del worker (linux/windows, estático)"`

---

### Task 0.6: Vagrant — node2 y node3

**Files:** Create `infra/vagrant/Vagrantfile`, `infra/vagrant/provision_worker.sh`, `infra/vagrant/redeploy.sh`.

- [ ] **Paso 1: `Vagrantfile`**

```ruby
# infra/vagrant/Vagrantfile — dos nodos worker en la red host-only 192.168.56.0/24
Vagrant.configure("2") do |config|
  config.vm.box = "bento/ubuntu-24.04"
  # El repo completo queda montado en /vagrant (para copiar el binario compilado)
  config.vm.synced_folder "../..", "/vagrant"

  nodes = {
    "node2" => { ip: "192.168.56.101", role: "audio",    mem: 1536, cpus: 2 },
    "node3" => { ip: "192.168.56.102", role: "metadata", mem: 1024, cpus: 1 },
  }

  nodes.each do |name, n|
    config.vm.define name do |node|
      node.vm.hostname = name
      node.vm.network "private_network", ip: n[:ip]      # adaptador 2: host-only
      node.vm.provider "virtualbox" do |vb|
        vb.name   = "mediacase-#{name}"
        vb.memory = n[:mem]
        vb.cpus   = n[:cpus]
      end
      node.vm.provision "shell", path: "provision_worker.sh",
        env: { "WORKER_ID" => name, "WORKER_ROLE" => n[:role], "WORKER_IP" => n[:ip] }
    end
  end
end
```

(Vagrant agrega el adaptador 1 NAT automáticamente. En la Fase 0 el rol no se usa todavía — el worker acepta todo; cobra sentido en la Fase 2.)

- [ ] **Paso 2: `provision_worker.sh`**

```bash
#!/usr/bin/env bash
# Aprovisiona un nodo worker: ffmpeg + binario + systemd. Idempotente.
set -euo pipefail

apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ffmpeg > /dev/null

install -d /etc/mediacase /var/lib/mediacase
install -m 0755 /vagrant/bin/worker-linux-amd64 /usr/local/bin/mediacase-worker

cat > /etc/mediacase/worker.env <<EOF
WORKER_ID=${WORKER_ID}
WORKER_ROLE=${WORKER_ROLE}
WORKER_POOL_SIZE=2
COORDINATOR_URL=http://192.168.56.1:8080
MINIO_ENDPOINT=192.168.56.1:9000
MINIO_PUBLIC_ENDPOINT=192.168.56.1:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=results
EOF

cat > /etc/systemd/system/mediacase-worker.service <<'EOF'
[Unit]
Description=MediaCase worker
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/mediacase/worker.env
ExecStart=/usr/local/bin/mediacase-worker
Restart=always
RestartSec=3
WorkingDirectory=/var/lib/mediacase

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now mediacase-worker
sleep 2
systemctl --no-pager --lines=5 status mediacase-worker || true
echo "provision OK: ${WORKER_ID} (${WORKER_ROLE}) en ${WORKER_IP}"
```

`Restart=always` es la reconexión automática: si el worker muere o no encuentra al coordinador, systemd lo relanza cada 3 s.

- [ ] **Paso 3: `redeploy.sh`** (para no re-aprovisionar en cada cambio de código)

```bash
#!/usr/bin/env bash
# Copia el binario recién compilado a los nodos y reinicia el servicio.
# Uso (desde infra/vagrant, en Git Bash):  bash redeploy.sh [node2|node3|all]
set -euo pipefail
target="${1:-all}"
nodes=("node2" "node3"); [[ "$target" != "all" ]] && nodes=("$target")
for n in "${nodes[@]}"; do
  vagrant ssh "$n" -c "sudo install -m 0755 /vagrant/bin/worker-linux-amd64 /usr/local/bin/mediacase-worker && sudo systemctl restart mediacase-worker && sleep 1 && systemctl is-active mediacase-worker"
done
```

- [ ] **Paso 4: Levantar**

```powershell
.\scripts\build-workers.ps1          # el provision copia bin/worker-linux-amd64
cd infra\vagrant
vagrant up                            # primera vez: baja la box (~600 MB), 5-10 min
vagrant ssh node2 -c "systemctl is-active mediacase-worker && ping -c1 192.168.56.1"
```
Esperado: `active` y el ping responde. **Si `vagrant up` rechaza VirtualBox 7.2**, Vagrant 2.4.9 lo soporta; si aparece "The provider 'virtualbox' ... version", correr `vagrant plugin repair` y reintentar; en último caso instalar VirtualBox 7.1 en paralelo.

- [ ] **Paso 5: Commit** (pedir OK) — `git add infra/vagrant && git commit -m "infra: Vagrantfile con node2/node3 y aprovisionamiento del worker como servicio systemd"`

---

### Task 0.7: Script de hito — un worker remoto procesa un archivo del coordinador del host

**Files:** Create `tests/distributed_smoke.sh`.

- [ ] **Paso 1: El script**

```bash
#!/usr/bin/env bash
# Hito Fase 0: el coordinador (host) asigna un job y lo completa un worker en otra máquina.
# Requiere: docker-compose.infra.yml arriba, coordinador nativo corriendo, worker del host APAGADO,
#           al menos un worker REMOTO conectado (otra PC o una VM), y una entrada subida como dataset/<KEY>.
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
KEY="${1:-audio_short_1_mp3.mp3}"

echo "→ workers registrados:"
curl -s "$COORD/workers" | python -m json.tool
echo "→ encolando job sobre $KEY"
JOB=$(curl -s -X POST "$COORD/jobs" -H 'Content-Type: application/json' \
  -d "{\"file_path\":\"$KEY\",\"operation\":\"convert_audio\",\"priority\":5}" \
  | python -c "import sys,json;print(json.load(sys.stdin)['id'])")

for i in $(seq 1 60); do
  J=$(curl -s "$COORD/jobs/$JOB")
  ST=$(echo "$J" | python -c "import sys,json;print(json.load(sys.stdin)['status'])")
  WK=$(echo "$J" | python -c "import sys,json;print(json.load(sys.stdin).get('worker_id',''))")
  echo "  [$i] status=$ST worker=$WK"
  [[ "$ST" == "completed" || "$ST" == "failed" ]] && break
  sleep 2
done

[[ "$ST" == "completed" ]] || { echo "FALLÓ: status=$ST"; exit 1; }
[[ -n "$WK" && "$WK" != "node1" ]] || { echo "FALLÓ: lo procesó '$WK', no un nodo remoto"; exit 1; }
echo "HITO OK — job $JOB completado por $WK"
```

- [ ] **Paso 2: Correrlo** (Git Bash) — `bash tests/distributed_smoke.sh`. Esperado: `HITO OK — ... completado por node2`.

- [ ] **Paso 3: Prueba de caída** — `vagrant ssh node2 -c "sudo systemctl stop mediacase-worker"`, encolar otro job, esperar 15 s: en el log del coordinador debe aparecer `evicted stale worker: node2` y `reclaimed job ... re-enqueuing`. Levantar `node3` (`vagrant up node3`) y el job debe completarse ahí. Volver a arrancar node2.

- [ ] **Paso 4: Registrar en `CLAUDE.md` §15** que la Fase 0 cerró, con fecha.
- [ ] **Paso 5: Commit** (pedir OK) — `git add tests/distributed_smoke.sh && git commit -m "tests: hito de distribución real (worker remoto)"`

---

### Task 0.8: Página "Conectar esta PC" y ZIP del worker con el `.env` ya escrito

El coordinador sirve un ZIP por sistema operativo con: el binario, `worker.env` **pre-llenado con la URL con la que el navegador llegó** (cabecera `Host`), un lanzador, y `ffmpeg` si está empaquetado. Quien lo baja no escribe IPs ni instala nada.

**Files:** Create `internal/coordinator/download.go`; Modify `api.go` (rutas `GET /connect`, `GET /download/worker`); agregar `dist/` a `.gitignore`.

**Interfaces:**
- `GET /connect` → HTML mínimo con dos botones (Windows / Linux) e instrucciones de 3 líneas.
- `GET /download/worker?os=windows|linux` → `application/zip`, nombre `mediacase-worker-<os>.zip`.
- Layout del ZIP (Windows): `worker.exe`, `worker.env`, `start-worker.ps1`, `start-worker.bat`, `ffmpeg.exe` (si existe `dist/ffmpeg/windows/ffmpeg.exe`). Linux: `worker`, `worker.env`, `start-worker.sh` (ffmpeg se instala con `apt`; el script avisa si falta).
- `worker.env` generado: `COORDINATOR_URL=http://<Host>`; `MINIO_ENDPOINT` y `MINIO_PUBLIC_ENDPOINT` desde `MINIO_PUBLIC_ENDPOINT` del coordinador; credenciales desde el env; `WORKER_ROLE=all`; `WORKER_POOL_SIZE=2`. `WORKER_ID` lo pone el lanzador con el nombre de la máquina.

- [ ] **Paso 1: `download.go`** (stdlib `archive/zip`, sin dependencias):

```go
package coordinator

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const connectHTML = `<!doctype html><meta charset="utf-8"><title>MediaCase — conectar esta PC</title>
<style>body{font-family:system-ui;max-width:640px;margin:3rem auto;padding:0 1rem;line-height:1.5}
a.b{display:inline-block;padding:.7rem 1.2rem;background:#1DB954;color:#fff;border-radius:6px;text-decoration:none;margin-right:.6rem}</style>
<h1>Conectar esta PC como worker</h1>
<p>Descargá el worker, descomprimilo y ejecutá <code>start-worker</code>. En unos segundos esta máquina aparece en el dashboard.</p>
<p><a class="b" href="/download/worker?os=windows">Windows</a><a class="b" href="/download/worker?os=linux">Linux</a></p>
<ol><li>Descomprimir el ZIP en cualquier carpeta.</li><li>Windows: doble clic en <code>start-worker.bat</code>. Linux: <code>bash start-worker.sh</code>.</li><li>Dejar la ventana abierta mientras quieras que esta PC procese.</li></ol>
<p>No hay que instalar nada ni abrir puertos: el worker se conecta hacia el coordinador.</p>`

func (a *API) connectPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, connectHTML)
}

func (a *API) downloadWorker(w http.ResponseWriter, r *http.Request) {
	osName := r.URL.Query().Get("os")
	if osName != "windows" && osName != "linux" {
		http.Error(w, "os debe ser windows o linux", http.StatusBadRequest)
		return
	}
	coordURL := "http://" + r.Host // la URL con la que el navegador llegó hasta aquí
	minioPub := envOr("MINIO_PUBLIC_ENDPOINT", "localhost:9000")
	env := fmt.Sprintf("COORDINATOR_URL=%s\nMINIO_ENDPOINT=%s\nMINIO_PUBLIC_ENDPOINT=%s\nMINIO_ACCESS_KEY=%s\nMINIO_SECRET_KEY=%s\nMINIO_BUCKET=%s\nWORKER_ROLE=all\nWORKER_POOL_SIZE=2\n",
		coordURL, minioPub, minioPub,
		envOr("MINIO_ACCESS_KEY", "minioadmin"), envOr("MINIO_SECRET_KEY", "minioadmin"), envOr("MINIO_BUCKET", "results"))

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="mediacase-worker-%s.zip"`, osName))
	zw := zip.NewWriter(w)
	defer zw.Close()

	addFile := func(name, src string, mode os.FileMode) error {
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f.Close()
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(mode)
		zf, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(zf, f)
		return err
	}
	addText := func(name, body string, mode os.FileMode) {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(mode)
		zf, _ := zw.CreateHeader(hdr)
		io.WriteString(zf, body)
	}

	addText("worker.env", env, 0o644)
	if osName == "windows" {
		if err := addFile("worker.exe", filepath.Join("bin", "worker-windows-amd64.exe"), 0o755); err != nil {
			http.Error(w, "binario de Windows no compilado (scripts/build-workers.ps1)", http.StatusInternalServerError)
			return
		}
		addFile("ffmpeg.exe", filepath.Join("dist", "ffmpeg", "windows", "ffmpeg.exe"), 0o755) // opcional
		addText("start-worker.ps1", "$env:PATH = \"$PSScriptRoot;$env:PATH\"\r\n"+
			"Get-Content \"$PSScriptRoot\\worker.env\" | Where-Object { $_ -match '^\\s*[^#].*=' } | ForEach-Object { $k,$v = $_ -split '=',2; [Environment]::SetEnvironmentVariable($k.Trim(),$v.Trim(),'Process') }\r\n"+
			"if (-not $env:WORKER_ID) { $env:WORKER_ID = $env:COMPUTERNAME.ToLower() }\r\n"+
			"& \"$PSScriptRoot\\worker.exe\"\r\n", 0o644)
		addText("start-worker.bat", "@echo off\r\npowershell -NoProfile -ExecutionPolicy Bypass -File \"%~dp0start-worker.ps1\"\r\npause\r\n", 0o644)
	} else {
		if err := addFile("worker", filepath.Join("bin", "worker-linux-amd64"), 0o755); err != nil {
			http.Error(w, "binario de Linux no compilado", http.StatusInternalServerError)
			return
		}
		addText("start-worker.sh", "#!/usr/bin/env bash\ncd \"$(dirname \"$0\")\"\n"+
			"command -v ffmpeg >/dev/null || { echo \"Falta ffmpeg: sudo apt install ffmpeg\"; exit 1; }\n"+
			"set -a; source ./worker.env; set +a\nexport WORKER_ID=\"${WORKER_ID:-$(hostname)}\"\nexec ./worker\n", 0o755)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

Rutas en `Router()`: `mux.HandleFunc("GET /connect", a.connectPage)` y `mux.HandleFunc("GET /download/worker", a.downloadWorker)`.

- [ ] **Paso 2: Empaquetar ffmpeg para Windows** — copiar el `ffmpeg.exe` que instaló winget a `dist/ffmpeg/windows/ffmpeg.exe` (`(Get-Command ffmpeg).Source`). `dist/` está en `.gitignore` (~90 MB).
- [ ] **Paso 3: Verificar en la propia laptop** — con el coordinador nativo corriendo, abrir `http://localhost:8080/connect`, bajar el ZIP de Windows, descomprimirlo en `C:\tmp\w\`, doble clic en `start-worker.bat`: en el log del coordinador aparece `[whub] worker <nombre-de-la-laptop> conectó` y `GET /workers` lo lista. Cerrar la ventana → a los 15 s el coordinador lo expulsa.
- [ ] **Paso 4: Commit** (pedir OK) — `git commit -am "coordinator: página /connect y descarga del worker con .env pre-llenado"`

---

### Task 0.9: HITO REAL — la PC de la novia de Leno (Windows 11, mismo WiFi)

Requiere: `docker-compose.infra.yml` arriba, coordinador nativo corriendo en la laptop de Leno, `scripts/firewall-node1.ps1` aplicado (una vez, admin), y una entrada subida a `dataset/`.

- [ ] **Paso 1:** En la laptop de Leno, `ipconfig` → anotar la IPv4 del adaptador WiFi (p. ej. `192.168.1.10`). `MINIO_PUBLIC_ENDPOINT` en `infra/env/node1.env` debe ser esa IP con `:9000`.
- [ ] **Paso 2:** En la otra PC, abrir `http://192.168.1.10:8080/connect` en el navegador. Si no carga: revisar que las dos estén en el mismo WiFi y que la regla de firewall exista (`Get-NetFirewallRule -DisplayName "MediaCase*"`).
- [ ] **Paso 3:** Descargar Windows → descomprimir → doble clic `start-worker.bat`. Debe imprimir `canal abierto con http://192.168.1.10:8080`.
- [ ] **Paso 4:** En la laptop de Leno, con el worker del host **apagado**: `bash tests/distributed_smoke.sh` → `HITO OK — job ... completado por <nombre-de-esa-pc>`.
- [ ] **Paso 5:** Prueba de caída: cerrar la ventana del worker en la otra PC a mitad de un job → en ≤15 s el coordinador lo expulsa y re-encola; volver a abrir `start-worker.bat` → el job se completa. Capturas de pantalla de ambas máquinas para el informe.
- [ ] **Paso 6:** Registrar en `CLAUDE.md` §15 que la Fase 0 cerró en hardware real, con fecha.

---

# FASE 1 — Capa de casos

**Hito:** `POST /cases` con 3 archivos (2 válidos, 1 corrupto) → los 3 se procesan en paralelo → el caso cierra solo como `partially_completed` → `GET /cases/{id}/report` devuelve el reporte consolidado con el resumen *"de 3 archivos — 1 video convertido, 1 audio convertido, 1 fallido"*.

### Task 1.1: Modelo `Case` y migración

**Files:**
- Create: `internal/models/case.go`
- Modify: `internal/models/job.go` (campos nuevos + `StatusCancelled`)
- Modify: `internal/db/db.go` (`Migrate`, `InsertJob`, `GetJob`, `ListJobs`, `scanJob`)

**Interfaces (Produces):**

```go
// internal/models/case.go
package models

import "time"

type CaseStatus string

const (
	CaseQueued             CaseStatus = "queued"
	CaseProcessing         CaseStatus = "processing"
	CaseCompleted          CaseStatus = "completed"
	CasePartiallyCompleted CaseStatus = "partially_completed"
	CaseFailed             CaseStatus = "failed"
	CaseRetrying           CaseStatus = "retrying"
	CaseCancelled          CaseStatus = "cancelled"
)

// IsTerminal indica si el caso ya no cambia de estado.
func (s CaseStatus) IsTerminal() bool {
	switch s {
	case CaseCompleted, CasePartiallyCompleted, CaseFailed, CaseCancelled:
		return true
	}
	return false
}

type FileType string

const (
	FileVideo FileType = "video"
	FileAudio FileType = "audio"
	FileImage FileType = "image"
)

type Case struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      CaseStatus `json:"status"`
	Priority    int        `json:"priority"`
	TotalJobs   int        `json:"total_jobs"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Jobs        []*Job     `json:"jobs,omitempty"`
}
```

En `job.go` agregar a `Job`:

```go
	CaseID   string   `json:"case_id,omitempty"`
	FileType FileType `json:"file_type"`
	Pool     string   `json:"pool"`
```

y la constante `StatusCancelled JobStatus = "cancelled"`.

- [ ] **Paso 1: Migración** — agregar al final del `Exec` de `Migrate`:

```sql
	CREATE TABLE IF NOT EXISTS cases (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL DEFAULT '',
		status       TEXT NOT NULL DEFAULT 'queued',
		priority     INT  NOT NULL DEFAULT 5,
		total_jobs   INT  NOT NULL DEFAULT 0,
		report       JSONB,
		created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		started_at   TIMESTAMPTZ,
		completed_at TIMESTAMPTZ
	);
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS case_id   TEXT REFERENCES cases(id);
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS file_type TEXT NOT NULL DEFAULT '';
	ALTER TABLE jobs ADD COLUMN IF NOT EXISTS pool      TEXT NOT NULL DEFAULT '';
	CREATE INDEX IF NOT EXISTS idx_jobs_case ON jobs(case_id);
	CREATE INDEX IF NOT EXISTS idx_cases_status ON cases(status);
```

- [ ] **Paso 2: Consultas de jobs con los campos nuevos** — en `db.go`:

```go
const jobColumns = `id, file_path, operation, status, priority,
	worker_id, progress, error_msg, result_url, retries, max_retries,
	created_at, started_at, completed_at, case_id, file_type, pool`

func InsertJob(db *sql.DB, job *models.Job) error {
	_, err := db.Exec(`
		INSERT INTO jobs (id, file_id, file_path, operation, status, priority, max_retries,
		                  created_at, case_id, file_type, pool)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9,''), $10, $11)`,
		job.ID, job.FileID, job.FilePath, job.Operation, job.Status, job.Priority,
		job.MaxRetries, job.CreatedAt, job.CaseID, job.FileType, job.Pool)
	return err
}
```

`GetJob` y `ListJobs` usan `SELECT ` + `jobColumns` + ` FROM jobs ...`. En `scanJob` agregar `var caseID sql.NullString` al final del `Scan` (`&caseID, &j.FileType, &j.Pool`) y `j.CaseID = caseID.String`.

- [ ] **Paso 3: Verificar** — `go build ./... && go vet ./...`; luego `docker compose -f docker-compose.infra.yml up -d`, correr el coordinador y:

```bash
docker compose -f docker-compose.infra.yml exec postgres psql -U media -d mediacase -c "\d cases" -c "\d jobs" | grep -E "case_id|file_type|pool|total_jobs"
```
Esperado: las 4 columnas listadas.

- [ ] **Paso 4: Commit** (pedir OK) — `git commit -am "models/db: entidad Case, case_id/file_type/pool en jobs, migración"`

---

### Task 1.2: Router por tipo (función pura, TDD)

**Files:** Create `internal/cases/router.go`, `internal/cases/router_test.go`.

**Interfaces (Produces):**

```go
package cases

type RouteDecision struct {
	FileType  models.FileType
	Operation models.Operation
	Pool      string
}

// DetectFileType clasifica por extensión. Error si la extensión no está soportada.
func DetectFileType(filename string) (models.FileType, error)
// DefaultOperation es la operación que el coordinador elige si el cliente no pide una.
func DefaultOperation(ft models.FileType) models.Operation
// PoolFor decide qué pool de workers ejecuta un tipo de contenido.
func PoolFor(ft models.FileType) string
// Route valida la operación pedida (o elige la default) y devuelve la decisión completa.
func Route(filename string, requested models.Operation) (RouteDecision, error)
```

- [ ] **Paso 1: Pruebas**

```go
package cases

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestDetectFileType(t *testing.T) {
	cases := map[string]models.FileType{
		"boda.mp4": models.FileVideo, "x.MKV": models.FileVideo, "a.webm": models.FileVideo,
		"discurso.mp3": models.FileAudio, "b.flac": models.FileAudio, "c.wav": models.FileAudio,
		"foto.jpg": models.FileImage, "d.PNG": models.FileImage,
	}
	for name, want := range cases {
		got, err := DetectFileType(name)
		if err != nil || got != want {
			t.Errorf("%s: got %v/%v, want %v", name, got, err, want)
		}
	}
	if _, err := DetectFileType("archivo.exe"); err == nil {
		t.Error("exe debería ser no soportado")
	}
}

func TestRoute_DefaultsPorTipo(t *testing.T) {
	tests := []struct {
		file string
		op   models.Operation
		pool string
	}{
		{"v.mp4", models.OpConvert, "video"},
		{"a.mp3", models.OpConvertAudio, "audio"},
		{"i.jpg", models.OpThumbnail, "metadata"},
	}
	for _, tc := range tests {
		d, err := Route(tc.file, "")
		if err != nil || d.Operation != tc.op || d.Pool != tc.pool {
			t.Errorf("%s: %+v err=%v", tc.file, d, err)
		}
	}
}

func TestRoute_RespetaOperacionValida(t *testing.T) {
	d, err := Route("v.mp4", models.OpExtractAudio)
	if err != nil || d.Operation != models.OpExtractAudio || d.Pool != "video" {
		t.Fatalf("%+v err=%v", d, err)
	}
}

func TestRoute_RechazaOperacionIncompatible(t *testing.T) {
	if _, err := Route("a.mp3", models.OpExtractAudio); err == nil {
		t.Error("extract_audio sobre audio debe rechazarse")
	}
	if _, err := Route("v.mp4", models.OpConvertAudio); err == nil {
		t.Error("convert_audio sobre video debe rechazarse")
	}
}
```

- [ ] **Paso 2: Verificar que falla** — `go test ./internal/cases/ -v` → no compila (`Route` no definida).

- [ ] **Paso 3: Implementar**

```go
// Package cases contiene la lógica de casos: routing por tipo, barrier/join y reporte.
package cases

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

var extToType = map[string]models.FileType{
	".mp4": models.FileVideo, ".mkv": models.FileVideo, ".avi": models.FileVideo,
	".mov": models.FileVideo, ".webm": models.FileVideo,
	".mp3": models.FileAudio, ".wav": models.FileAudio, ".flac": models.FileAudio,
	".aac": models.FileAudio, ".ogg": models.FileAudio, ".m4a": models.FileAudio,
	".jpg": models.FileImage, ".jpeg": models.FileImage, ".png": models.FileImage,
	".gif": models.FileImage, ".webp": models.FileImage, ".bmp": models.FileImage,
}

// Operaciones válidas por tipo de contenido. La primera es la default.
var opsByType = map[models.FileType][]models.Operation{
	models.FileVideo: {models.OpConvert, models.OpExtractAudio, models.OpThumbnail},
	models.FileAudio: {models.OpConvertAudio, models.OpThumbnail},
	models.FileImage: {models.OpThumbnail},
}

var poolByType = map[models.FileType]string{
	models.FileVideo: "video",
	models.FileAudio: "audio",
	models.FileImage: "metadata",
}

type RouteDecision struct {
	FileType  models.FileType
	Operation models.Operation
	Pool      string
}

func DetectFileType(filename string) (models.FileType, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	ft, ok := extToType[ext]
	if !ok {
		return "", fmt.Errorf("formato no soportado: %q", ext)
	}
	return ft, nil
}

func DefaultOperation(ft models.FileType) models.Operation { return opsByType[ft][0] }

func PoolFor(ft models.FileType) string { return poolByType[ft] }

func Route(filename string, requested models.Operation) (RouteDecision, error) {
	ft, err := DetectFileType(filename)
	if err != nil {
		return RouteDecision{}, err
	}
	op := requested
	if op == "" {
		op = DefaultOperation(ft)
	} else {
		valid := false
		for _, o := range opsByType[ft] {
			if o == op {
				valid = true
				break
			}
		}
		if !valid {
			return RouteDecision{}, fmt.Errorf("operación %q no aplica a %s (%s)", op, ft, filename)
		}
	}
	return RouteDecision{FileType: ft, Operation: op, Pool: PoolFor(ft)}, nil
}
```

- [ ] **Paso 4: Verificar que pasa** — `go test ./internal/cases/ -v` → 4 × `PASS`.
- [ ] **Paso 5: Commit** (pedir OK) — `git add internal/cases && git commit -m "cases: router por tipo de contenido (decisión del coordinador)"`

---

### Task 1.3: Consultas de casos

**Files:** Create `internal/db/cases.go`.

**Interfaces (Produces):**

```go
package db

func InsertCase(db *sql.DB, c *models.Case) error
func GetCase(db *sql.DB, id string) (*models.Case, error)          // sin jobs
func ListCases(db *sql.DB, status string, limit int) ([]*models.Case, error)
func ListJobsByCase(db *sql.DB, caseID string) ([]*models.Job, error)
func SetCaseStatus(db *sql.DB, id string, st models.CaseStatus) error
func SaveCaseReport(db *sql.DB, id string, report []byte) error
func GetCaseReport(db *sql.DB, id string) ([]byte, error)          // nil si aún no hay

type CaseCounts struct{ Total, Completed, Failed, Running, Pending int }
func CountJobsByCase(tx *sql.Tx, caseID string) (CaseCounts, error) // recibe Tx: lo usa el barrier
```

- [ ] **Paso 1: Implementar** — SQL directo, mismo estilo que `db.go`:

```go
package db

import (
	"database/sql"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

const caseColumns = `id, name, status, priority, total_jobs, created_at, started_at, completed_at`

func InsertCase(db *sql.DB, c *models.Case) error {
	_, err := db.Exec(`INSERT INTO cases (id, name, status, priority, total_jobs, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, c.ID, c.Name, c.Status, c.Priority, c.TotalJobs, c.CreatedAt)
	return err
}

func scanCase(row interface{ Scan(...any) error }) (*models.Case, error) {
	c := &models.Case{}
	if err := row.Scan(&c.ID, &c.Name, &c.Status, &c.Priority, &c.TotalJobs,
		&c.CreatedAt, &c.StartedAt, &c.CompletedAt); err != nil {
		return nil, err
	}
	return c, nil
}

func GetCase(db *sql.DB, id string) (*models.Case, error) {
	return scanCase(db.QueryRow(`SELECT `+caseColumns+` FROM cases WHERE id=$1`, id))
}

func ListCases(db *sql.DB, status string, limit int) ([]*models.Case, error) {
	q := `SELECT ` + caseColumns + ` FROM cases`
	args := []any{}
	if status != "" {
		q += ` WHERE status=$1`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT ` + itoa(limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.Case, 0)
	for rows.Next() {
		if c, err := scanCase(rows); err == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func ListJobsByCase(db *sql.DB, caseID string) ([]*models.Job, error) {
	rows, err := db.Query(`SELECT `+jobColumns+` FROM jobs WHERE case_id=$1 ORDER BY created_at`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*models.Job, 0)
	for rows.Next() {
		if j, err := scanJob(rows); err == nil {
			out = append(out, j)
		}
	}
	return out, nil
}

func SetCaseStatus(db *sql.DB, id string, st models.CaseStatus) error {
	_, err := db.Exec(`UPDATE cases SET status=$1 WHERE id=$2`, st, id)
	return err
}

func SaveCaseReport(db *sql.DB, id string, report []byte) error {
	_, err := db.Exec(`UPDATE cases SET report=$1 WHERE id=$2`, report, id)
	return err
}

func GetCaseReport(db *sql.DB, id string) ([]byte, error) {
	var raw []byte
	err := db.QueryRow(`SELECT report FROM cases WHERE id=$1`, id).Scan(&raw)
	return raw, err
}

type CaseCounts struct{ Total, Completed, Failed, Running, Pending int }

func CountJobsByCase(tx *sql.Tx, caseID string) (CaseCounts, error) {
	var c CaseCounts
	err := tx.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status='completed'),
		       COUNT(*) FILTER (WHERE status='failed'),
		       COUNT(*) FILTER (WHERE status='running'),
		       COUNT(*) FILTER (WHERE status IN ('pending','assigned'))
		FROM jobs WHERE case_id=$1`, caseID).
		Scan(&c.Total, &c.Completed, &c.Failed, &c.Running, &c.Pending)
	return c, err
}

func itoa(n int) string { return strconv.Itoa(n) } // import "strconv"
```

- [ ] **Paso 2: Verificar** — `go build ./... && go vet ./internal/db/`.
- [ ] **Paso 3: Commit** (pedir OK) — `git add internal/db/cases.go && git commit -m "db: consultas de casos y conteo de sub-tareas por caso"`

---

### Task 1.4: `POST /cases` — recibir, enrutar, descomponer, encolar

**Files:**
- Create: `internal/coordinator/cases_api.go`
- Modify: `internal/coordinator/api.go` (`Router`: agregar rutas; `submitJob`: usar `cases.Route` para llenar `FileType`/`Pool`)

**Interfaces:**
- Consumes: `cases.Route`, `db.InsertCase`, `db.InsertJob`, `queue.Enqueue`.
- Produces: `POST /cases` — request/response:

```json
// request
{ "name": "boda-garcia", "priority": 8,
  "files": [ { "key": "boda.mp4" },
             { "key": "discurso.mp3", "operation": "convert_audio" },
             { "key": "foto.jpg" } ] }
// response 201: models.Case con "jobs" poblado
```
Errores: `400` si `files` vacío o alguna ruta no válida (se rechaza el caso completo, no se inserta nada).

- [ ] **Paso 1: Handler**

```go
package coordinator

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/models"
)

type caseFileReq struct {
	Key       string           `json:"key"`
	Operation models.Operation `json:"operation,omitempty"`
}

type submitCaseReq struct {
	Name     string        `json:"name"`
	Priority int           `json:"priority"`
	Files    []caseFileReq `json:"files"`
}

// submitCase: registra el caso, lo descompone en sub-tareas (routing por tipo) y las encola.
func (a *API) submitCase(w http.ResponseWriter, r *http.Request) {
	var req submitCaseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Files) == 0 {
		http.Error(w, "body inválido: se requiere files[] no vacío", http.StatusBadRequest)
		return
	}
	if req.Priority == 0 {
		req.Priority = 5
	}

	// 1. Validar y enrutar TODO antes de tocar la base: el caso se acepta entero o se rechaza entero.
	decisions := make([]cases.RouteDecision, len(req.Files))
	for i, f := range req.Files {
		d, err := cases.Route(f.Key, f.Operation)
		if err != nil {
			http.Error(w, "archivo "+f.Key+": "+err.Error(), http.StatusBadRequest)
			return
		}
		decisions[i] = d
	}

	// 2. Registrar el caso.
	c := &models.Case{
		ID: uuid.New().String(), Name: req.Name, Status: models.CaseQueued,
		Priority: req.Priority, TotalJobs: len(req.Files), CreatedAt: time.Now(),
	}
	if err := db.InsertCase(a.db, c); err != nil {
		log.Printf("[cases] insert case: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// 3. Descomponer en sub-tareas y encolar cada una en su pool.
	for i, f := range req.Files {
		job := &models.Job{
			ID: uuid.New().String(), CaseID: c.ID, FileID: f.Key, FilePath: f.Key,
			Operation: decisions[i].Operation, FileType: decisions[i].FileType, Pool: decisions[i].Pool,
			Priority: req.Priority, Status: models.StatusPending, MaxRetries: 3, CreatedAt: time.Now(),
		}
		if err := db.InsertJob(a.db, job); err != nil {
			log.Printf("[cases] insert job %s: %v", job.ID, err)
			continue
		}
		if err := a.queue.Enqueue(r.Context(), job); err != nil {
			log.Printf("[cases] enqueue job %s: %v", job.ID, err)
		}
		c.Jobs = append(c.Jobs, job)
	}

	log.Printf("[cases] caso %s (%s): %d sub-tareas encoladas", c.ID, c.Name, len(c.Jobs))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}
```

- [ ] **Paso 2: Rutas** — en `Router()` agregar `mux.HandleFunc("POST /cases", a.submitCase)`. En `submitJob` (el endpoint suelto que sigue existiendo para pruebas), después de decodificar:

```go
	d, err := cases.Route(req.FilePath, req.Operation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job.Operation, job.FileType, job.Pool = d.Operation, d.FileType, d.Pool
```

- [ ] **Paso 3: Verificar** — con infra + coordinador + worker host corriendo, y dos claves subidas a `dataset/`:

```bash
curl -s -X POST localhost:8080/cases -H 'Content-Type: application/json' -d '{
  "name":"prueba-1","priority":7,
  "files":[{"key":"video_short_1_mp4.mp4"},{"key":"audio_short_1_mp3.mp3"}]}' | python -m json.tool
```
Esperado: `201`, `"status": "queued"`, `"total_jobs": 2`, y en `jobs[]` el video con `"operation": "convert", "pool": "video"` y el audio con `"convert_audio", "audio"` — **sin que el cliente los haya pedido**. Un `{"key":"x.exe"}` debe devolver `400 archivo x.exe: formato no soportado`.

- [ ] **Paso 4: Commit** (pedir OK) — `git add internal/coordinator && git commit -m "coordinator: POST /cases con routing por tipo y descomposición en sub-tareas"`

---

### Task 1.5: Barrier/join

**Files:** Create `internal/cases/barrier.go`, `internal/cases/barrier_test.go`.

**Interfaces (Produces):**

```go
// ComputeStatus es la regla pura del barrier. closed=false mientras falte alguna sub-tarea.
func ComputeStatus(total, completed, failed int) (status models.CaseStatus, closed bool)

type Barrier struct { db *sql.DB; onClose func(caseID string) }
func NewBarrier(db *sql.DB, onClose func(caseID string)) *Barrier
// OnJobResolved se llama cada vez que una sub-tarea llega a completed o failed.
// Cierra el caso exactamente una vez (SELECT ... FOR UPDATE) cuando todas resolvieron.
func (b *Barrier) OnJobResolved(ctx context.Context, caseID string) error
```

Regla: `closed ⇔ completed+failed == total`. Con `closed`: `failed==0 → completed`; `completed==0 → failed`; si no → `partially_completed`.

- [ ] **Paso 1: Prueba de la regla pura**

```go
package cases

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func TestComputeStatus(t *testing.T) {
	tests := []struct {
		total, done, failed int
		want                models.CaseStatus
		closed              bool
	}{
		{3, 1, 0, "", false},                              // faltan 2
		{3, 2, 0, "", false},                              // falta 1: NO cierra aunque 2 estén listas
		{3, 3, 0, models.CaseCompleted, true},
		{3, 2, 1, models.CasePartiallyCompleted, true},
		{3, 0, 3, models.CaseFailed, true},
		{1, 0, 1, models.CaseFailed, true},
		{0, 0, 0, models.CaseCompleted, true},              // caso vacío: se cierra completo
	}
	for _, tc := range tests {
		got, closed := ComputeStatus(tc.total, tc.done, tc.failed)
		if closed != tc.closed || (closed && got != tc.want) {
			t.Errorf("(%d,%d,%d): got %v/%v want %v/%v", tc.total, tc.done, tc.failed, got, closed, tc.want, tc.closed)
		}
	}
}
```

- [ ] **Paso 2: Verificar que falla** — `go test ./internal/cases/ -run TestComputeStatus`.

- [ ] **Paso 3: Implementar**

```go
package cases

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/models"
)

func ComputeStatus(total, completed, failed int) (models.CaseStatus, bool) {
	if completed+failed < total {
		return "", false
	}
	switch {
	case failed == 0:
		return models.CaseCompleted, true
	case completed == 0:
		return models.CaseFailed, true
	default:
		return models.CasePartiallyCompleted, true
	}
}

type Barrier struct {
	db      *sql.DB
	onClose func(caseID string)
}

func NewBarrier(database *sql.DB, onClose func(caseID string)) *Barrier {
	return &Barrier{db: database, onClose: onClose}
}

func (b *Barrier) OnJobResolved(ctx context.Context, caseID string) error {
	if caseID == "" {
		return nil // job suelto (POST /jobs), no pertenece a un caso
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Bloquea la fila del caso: dos sub-tareas que terminan a la vez se serializan aquí.
	var status models.CaseStatus
	var total int
	if err := tx.QueryRow(`SELECT status, total_jobs FROM cases WHERE id=$1 FOR UPDATE`, caseID).
		Scan(&status, &total); err != nil {
		return fmt.Errorf("lock case %s: %w", caseID, err)
	}
	if status.IsTerminal() {
		return nil // ya cerrado (o cancelado): nada que hacer
	}

	counts, err := db.CountJobsByCase(tx, caseID)
	if err != nil {
		return err
	}
	final, closed := ComputeStatus(total, counts.Completed, counts.Failed)
	if !closed {
		return tx.Commit() // barrier sigue esperando
	}

	if _, err := tx.Exec(`UPDATE cases SET status=$1, completed_at=NOW() WHERE id=$2`, final, caseID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("[barrier] caso %s cerrado: %s (%d ok, %d fallidas de %d)",
		caseID, final, counts.Completed, counts.Failed, total)
	if b.onClose != nil {
		b.onClose(caseID) // genera el reporte (Task 1.7)
	}
	return nil
}
```

- [ ] **Paso 4: Verificar que pasa** — `go test ./internal/cases/ -v` → `TestComputeStatus PASS`; `go vet ./internal/cases/`.
- [ ] **Paso 5: Commit** (pedir OK) — `git add internal/cases/barrier*.go && git commit -m "cases: barrier/join con cierre exactamente-una-vez"`

---

### Task 1.6: Enganchar el barrier donde una sub-tarea resuelve

Una sub-tarea llega a `completed`/`failed` en **cuatro** lugares. Los cuatro deben avisar al barrier.

**Files:**
- Modify: `internal/coordinator/api.go` (`API` recibe `*cases.Barrier`; `jobProgress`)
- Modify: `internal/coordinator/scheduler.go` (`Scheduler` recibe `*cases.Barrier`; `requeueJob`, `reclaimStuckJobs`)
- Modify: `cmd/coordinator/main.go` (construir el barrier y pasarlo)

- [ ] **Paso 1: Helper compartido** en `api.go`:

```go
// caseOf devuelve el case_id de un job ("" si es un job suelto).
func caseOf(database *sql.DB, jobID string) string {
	var caseID sql.NullString
	database.QueryRow(`SELECT case_id FROM jobs WHERE id=$1`, jobID).Scan(&caseID)
	return caseID.String
}
```

- [ ] **Paso 2: `jobProgress`** — en los `case` de `completed` y `failed`, después del `Exec`:

```go
		if err := a.barrier.OnJobResolved(r.Context(), caseOf(a.db, id)); err != nil {
			log.Printf("[barrier] job %s: %v", id, err)
		}
```
Y en el `case running`, además del `UPDATE jobs`, mover el caso a `processing`:
```go
		a.db.Exec(`UPDATE cases SET status='processing', started_at=COALESCE(started_at, NOW())
		           WHERE id=(SELECT case_id FROM jobs WHERE id=$1) AND status IN ('queued','retrying')`, id)
```

- [ ] **Paso 3: Scheduler** — en `requeueJob`, cuando `job.Retries >= job.MaxRetries` y se marca `failed`: `s.barrier.OnJobResolved(ctx, job.CaseID)`. En `reclaimStuckJobs`, cambiar el `UPDATE` por `UPDATE ... RETURNING case_id` e invocar el barrier por cada `case_id` distinto no nulo.

- [ ] **Paso 4: Cableado en `main.go`**

```go
	barrier := cases.NewBarrier(database, nil) // onClose se conecta en la Task 1.7
	scheduler := coordinator.NewScheduler(q, registry, database, barrier)
	api := coordinator.NewAPI(q, registry, hub, database, barrier)
```

- [ ] **Paso 5: Verificar** — repetir el `POST /cases` de la Task 1.4 y esperar ~30 s:

```bash
docker compose -f docker-compose.infra.yml exec postgres psql -U media -d mediacase -tA \
  -c "SELECT id, status, started_at IS NOT NULL, completed_at IS NOT NULL FROM cases ORDER BY created_at DESC LIMIT 1"
```
Esperado: `...|completed|t|t`. Repetir con un caso que incluya una clave que **no existe** en MinIO (`{"key":"no-existe.mp4"}`): esa sub-tarea falla en la descarga y el caso debe quedar `partially_completed`.

- [ ] **Paso 6: Commit** (pedir OK) — `git commit -am "coordinator: el barrier se dispara en los 4 puntos donde una sub-tarea resuelve"`

---

### Task 1.7: Reporte consolidado

**Files:** Create `internal/cases/report.go`, `internal/cases/report_test.go`; Modify `cases_api.go` (`GET /cases/{id}/report`), `cmd/coordinator/main.go` (onClose + MinIO).

**Interfaces (Produces):**

```go
type SubTaskResult struct {
	JobID, File string
	FileType  models.FileType
	Operation models.Operation
	Status    models.JobStatus
	WorkerID  string
	StartedAt, CompletedAt *time.Time
	DurationSeconds float64
	ResultURL, Error string
}
type GroupCount struct {
	FileType  models.FileType
	Operation models.Operation
	Completed, Failed int
}
type Report struct {
	CaseID, Name string
	Status       models.CaseStatus
	CreatedAt    time.Time
	StartedAt, CompletedAt *time.Time
	DurationSeconds float64
	Totals   struct{ Total, Completed, Failed int }
	ByTypeAndOperation []GroupCount
	SubTasks []SubTaskResult
	Summary  string
}
func BuildReport(c *models.Case, jobs []*models.Job) *Report   // puro
func Summary(r *Report) string                                  // puro, en español
```
Todos los campos con etiquetas `json:"snake_case"`.

- [ ] **Paso 1: Prueba del resumen**

```go
func TestSummary(t *testing.T) {
	now := time.Now()
	c := &models.Case{ID: "C1", Status: models.CasePartiallyCompleted, TotalJobs: 4, CreatedAt: now}
	jobs := []*models.Job{
		{FilePath: "a.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusCompleted},
		{FilePath: "b.mp4", FileType: models.FileVideo, Operation: models.OpConvert, Status: models.StatusCompleted},
		{FilePath: "c.mp3", FileType: models.FileAudio, Operation: models.OpConvertAudio, Status: models.StatusCompleted},
		{FilePath: "d.jpg", FileType: models.FileImage, Operation: models.OpThumbnail, Status: models.StatusFailed, ErrorMsg: "formato no soportado"},
	}
	r := BuildReport(c, jobs)
	want := "de 4 archivos — 2 videos convertidos, 1 audio convertido, 1 fallido (formato no soportado)"
	if r.Summary != want {
		t.Fatalf("\n got: %s\nwant: %s", r.Summary, want)
	}
	if r.Totals.Completed != 3 || r.Totals.Failed != 1 || len(r.ByTypeAndOperation) != 3 {
		t.Fatalf("totales/grupos: %+v", r.Totals)
	}
}
```

- [ ] **Paso 2: Verificar que falla**, luego **Paso 3: Implementar**

```go
package cases

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// (structs de la sección Interfaces, con tags json)

var opLabels = map[models.Operation][2]string{ // singular, plural
	models.OpConvert:      {"video convertido", "videos convertidos"},
	models.OpConvertAudio: {"audio convertido", "audios convertidos"},
	models.OpExtractAudio: {"audio extraído", "audios extraídos"},
	models.OpThumbnail:    {"miniatura generada", "miniaturas generadas"},
}

func BuildReport(c *models.Case, jobs []*models.Job) *Report {
	r := &Report{CaseID: c.ID, Name: c.Name, Status: c.Status, CreatedAt: c.CreatedAt,
		StartedAt: c.StartedAt, CompletedAt: c.CompletedAt}
	if c.StartedAt != nil && c.CompletedAt != nil {
		r.DurationSeconds = c.CompletedAt.Sub(*c.StartedAt).Seconds()
	}
	groups := map[string]*GroupCount{}
	for _, j := range jobs {
		st := SubTaskResult{JobID: j.ID, File: j.FilePath, FileType: j.FileType, Operation: j.Operation,
			Status: j.Status, WorkerID: j.WorkerID, StartedAt: j.StartedAt, CompletedAt: j.CompletedAt,
			ResultURL: j.ResultURL, Error: j.ErrorMsg}
		if j.StartedAt != nil && j.CompletedAt != nil {
			st.DurationSeconds = j.CompletedAt.Sub(*j.StartedAt).Seconds()
		}
		r.SubTasks = append(r.SubTasks, st)
		r.Totals.Total++
		key := string(j.FileType) + "/" + string(j.Operation)
		g, ok := groups[key]
		if !ok {
			g = &GroupCount{FileType: j.FileType, Operation: j.Operation}
			groups[key] = g
		}
		switch j.Status {
		case models.StatusCompleted:
			r.Totals.Completed++
			g.Completed++
		case models.StatusFailed:
			r.Totals.Failed++
			g.Failed++
		}
	}
	for _, g := range groups {
		r.ByTypeAndOperation = append(r.ByTypeAndOperation, *g)
	}
	sort.Slice(r.ByTypeAndOperation, func(i, k int) bool {
		a, b := r.ByTypeAndOperation[i], r.ByTypeAndOperation[k]
		return a.FileType+"/"+models.FileType(a.Operation) < b.FileType+"/"+models.FileType(b.Operation)
	})
	r.Summary = Summary(r)
	return r
}

func Summary(r *Report) string {
	parts := []string{}
	for _, g := range r.ByTypeAndOperation {
		if g.Completed == 0 {
			continue
		}
		lbl := opLabels[g.Operation]
		if g.Completed == 1 {
			parts = append(parts, fmt.Sprintf("1 %s", lbl[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", g.Completed, lbl[1]))
		}
	}
	if r.Totals.Failed > 0 {
		reason := ""
		for _, s := range r.SubTasks {
			if s.Status == models.StatusFailed && s.Error != "" {
				reason = " (" + firstLine(s.Error) + ")"
				break
			}
		}
		word := "fallidos"
		if r.Totals.Failed == 1 {
			word = "fallido"
		}
		parts = append(parts, fmt.Sprintf("%d %s%s", r.Totals.Failed, word, reason))
	}
	return fmt.Sprintf("de %d archivos — %s", r.Totals.Total, strings.Join(parts, ", "))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
```

- [ ] **Paso 4: Generar y guardar el reporte al cerrar** — en `cmd/coordinator/main.go`, construir MinIO (`storage.NewMinIOClient()`; si falla, solo log) y:

```go
	barrier := cases.NewBarrier(database, func(caseID string) {
		c, err := db.GetCase(database, caseID)
		if err != nil { log.Printf("[report] get case %s: %v", caseID, err); return }
		jobs, _ := db.ListJobsByCase(database, caseID)
		rep := cases.BuildReport(c, jobs)
		raw, _ := json.Marshal(rep)
		if err := db.SaveCaseReport(database, caseID, raw); err != nil {
			log.Printf("[report] save %s: %v", caseID, err)
		}
		if minioClient != nil {
			tmp := filepath.Join(os.TempDir(), "report-"+caseID+".json")
			if os.WriteFile(tmp, raw, 0o644) == nil {
				minioClient.UploadObject(ctx, "results", "cases/"+caseID+"/report.json", tmp)
				os.Remove(tmp)
			}
		}
		log.Printf("[report] caso %s: %s", caseID, rep.Summary)
	})
```

- [ ] **Paso 5: Endpoint** en `cases_api.go` + ruta `GET /cases/{id}/report`:

```go
func (a *API) getCaseReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	raw, err := db.GetCaseReport(a.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if raw == nil {
		c, _ := db.GetCase(a.db, id)
		http.Error(w, "el caso aún no ha terminado (estado: "+string(c.Status)+")", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}
```

- [ ] **Paso 6: Verificar** — `go test ./internal/cases/ -v` (TestSummary PASS). Luego un caso real y, tras cerrar: `curl -s localhost:8080/cases/$ID/report | python -m json.tool` → `summary`, `by_type_and_operation`, `sub_tasks[]` con `worker_id`, `started_at`, `completed_at`. En MinIO (`localhost:9001`) debe existir `results/cases/<id>/report.json`.
- [ ] **Paso 7: Commit** (pedir OK) — `git commit -am "cases: reporte consolidado por caso + GET /cases/{id}/report"`

---

### Task 1.8: `GET /cases`, `GET /cases/{id}`, `POST /cases/{id}/cancel`

**Files:** Modify `cases_api.go`, `api.go` (rutas), `scheduler.go` (saltar jobs cancelados).

- [ ] **Paso 1: Handlers**

```go
func (a *API) listCases(w http.ResponseWriter, r *http.Request) {
	list, err := db.ListCases(a.db, r.URL.Query().Get("status"), 500)
	if err != nil { http.Error(w, "db error", 500); return }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (a *API) getCase(w http.ResponseWriter, r *http.Request) {
	c, err := db.GetCase(a.db, r.PathValue("id"))
	if err != nil { http.Error(w, "not found", 404); return }
	c.Jobs, _ = db.ListJobsByCase(a.db, c.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

// cancelCase: marca el caso cancelado y sus sub-tareas aún no iniciadas. Las que ya corren terminan,
// pero el barrier ignora casos terminales, así que el caso no vuelve a cambiar de estado.
func (a *API) cancelCase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := a.db.Exec(`UPDATE cases SET status='cancelled', completed_at=NOW()
		WHERE id=$1 AND status IN ('queued','processing','retrying')`, id)
	if n, _ := res.RowsAffected(); err != nil || n == 0 {
		http.Error(w, "el caso no existe o ya es terminal", http.StatusConflict)
		return
	}
	a.db.Exec(`UPDATE jobs SET status='cancelled', completed_at=NOW()
		WHERE case_id=$1 AND status IN ('pending','assigned')`, id)
	w.WriteHeader(http.StatusOK)
}
```
Rutas: `GET /cases`, `GET /cases/{id}`, `POST /cases/{id}/cancel`.

- [ ] **Paso 2: Scheduler salta cancelados** — en `dispatch`, después de `Dequeue`:

```go
	var st string
	s.db.QueryRow(`SELECT status FROM jobs WHERE id=$1`, job.ID).Scan(&st)
	if st == string(models.StatusCancelled) {
		s.queue.Ack(ctx, queue.StreamFor(job.Pool, job.Priority), msgID) // ver Task 2.2; hoy StreamForPriority
		return nil
	}
```

- [ ] **Paso 3: Verificar** — `POST /cases` con 6 archivos, inmediatamente `POST /cases/$ID/cancel`; `GET /cases/$ID` debe mostrar `cancelled` y sub-tareas `cancelled`/`completed` pero ninguna `pending`. `GET /cases?status=cancelled` la lista.
- [ ] **Paso 4: Commit** (pedir OK) — `git commit -am "coordinator: listar, consultar y cancelar casos"`

---

### Task 1.9: Estado `retrying`

Cuando un worker muere, sus sub-tareas se re-encolan (`reclaimWorkerJobs`). El caso debe reflejarlo.

**Files:** Modify `scheduler.go` (`reclaimWorkerJobs`).

- [ ] **Paso 1:** al final de `reclaimWorkerJobs`, con los `case_id` de los jobs reclamados (agregar `case_id` al `RETURNING` y recolectarlos en un `map[string]bool`):

```go
	for caseID := range affected {
		s.db.ExecContext(ctx, `UPDATE cases SET status='retrying' WHERE id=$1 AND status='processing'`, caseID)
	}
```
La vuelta a `processing` ya está cubierta por la Task 1.6 (`status IN ('queued','retrying')` cuando una sub-tarea reporta `running`).

- [ ] **Paso 2: Verificar** — caso de 4 archivos hacia node2; a mitad, `vagrant ssh node2 -c "sudo systemctl stop mediacase-worker"`. En ≤15 s `GET /cases/$ID` → `retrying`. Arrancar node3: pasa a `processing` y termina `completed`.
- [ ] **Paso 3: Commit** (pedir OK) — `git commit -am "coordinator: estado retrying al reclamar sub-tareas de un worker caído"`

---

### Task 1.10: Cliente — modo `-case`

**Files:** Modify `cmd/client/main.go`.

- [ ] **Paso 1: Flags y función**

```go
	caseMode  := flag.Bool("case", false, "Enviar un caso (usa -files, -name, -priority)")
	files     := flag.String("files", "", "Claves en MinIO separadas por coma: a.mp4,b.mp3")
	caseName  := flag.String("name", "", "Nombre del caso")
```

```go
func runCase(coordinatorURL, name, files string, priority int, watch bool) {
	var req struct {
		Name     string `json:"name"`
		Priority int    `json:"priority"`
		Files    []struct{ Key string `json:"key"` } `json:"files"`
	}
	req.Name, req.Priority = name, priority
	for _, k := range strings.Split(files, ",") {
		if k = strings.TrimSpace(k); k != "" {
			req.Files = append(req.Files, struct{ Key string `json:"key"` }{k})
		}
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(coordinatorURL+"/cases", "application/json", bytes.NewReader(body))
	if err != nil { log.Fatalf("POST /cases: %v", err) }
	defer resp.Body.Close()
	var c struct{ ID string `json:"id"`; Status string `json:"status"`; TotalJobs int `json:"total_jobs"` }
	json.NewDecoder(resp.Body).Decode(&c)
	fmt.Printf("caso %s creado: %d sub-tareas, estado %s\n", c.ID, c.TotalJobs, c.Status)
	if !watch { return }
	for {
		r, _ := http.Get(coordinatorURL + "/cases/" + c.ID)
		var cur struct{ Status string `json:"status"`; Jobs []struct{ Status string `json:"status"`; WorkerID string `json:"worker_id"` } `json:"jobs"` }
		json.NewDecoder(r.Body).Decode(&cur); r.Body.Close()
		done := 0
		for _, j := range cur.Jobs { if j.Status == "completed" || j.Status == "failed" { done++ } }
		fmt.Printf("\r  %s — %d/%d resueltas   ", cur.Status, done, len(cur.Jobs))
		if cur.Status == "completed" || cur.Status == "partially_completed" || cur.Status == "failed" || cur.Status == "cancelled" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Println()
	r, _ := http.Get(coordinatorURL + "/cases/" + c.ID + "/report")
	io.Copy(os.Stdout, r.Body); r.Body.Close()
	fmt.Println()
}
```
En `main()`: `case *caseMode: runCase(*coordinatorURL, *caseName, *files, *priority, *watch)`.

- [ ] **Paso 2: Verificar** — `go run ./cmd/client -coordinator http://localhost:8080 -case -name demo -files video_short_1_mp4.mp4,audio_short_1_mp3.mp3 -priority 8 -watch` → imprime progreso y al final el JSON del reporte.
- [ ] **Paso 3: Commit** (pedir OK) — `git commit -am "client: modo -case con seguimiento y reporte"`

---

### Task 1.11: HITO — caso heterogéneo con un fallo

**Files:** Create `tests/case_scenario.sh`.

- [ ] **Paso 1: Script**

```bash
#!/usr/bin/env bash
# Hito Fase 1: caso heterogéneo (video + audio + archivo corrupto) → partially_completed + reporte.
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
INFRA="docker compose -f docker-compose.infra.yml"

# Entradas: dos válidas del dataset y una "corrupta" (texto con extensión .mp4)
echo "esto no es un video" > /tmp/corrupto.mp4
$INFRA cp dataset/files/video_short_1_mp4.mp4 minio:/tmp/v.mp4
$INFRA cp dataset/files/audio_short_1_mp3.mp3 minio:/tmp/a.mp3
$INFRA cp /tmp/corrupto.mp4 minio:/tmp/corrupto.mp4
$INFRA exec minio sh -c "mc alias set local http://localhost:9000 minioadmin minioadmin >/dev/null && mc mb --ignore-existing local/dataset >/dev/null && mc cp /tmp/v.mp4 local/dataset/hito_video.mp4 && mc cp /tmp/a.mp3 local/dataset/hito_audio.mp3 && mc cp /tmp/corrupto.mp4 local/dataset/hito_corrupto.mp4" >/dev/null

ID=$(curl -s -X POST "$COORD/cases" -H 'Content-Type: application/json' -d '{
  "name":"hito-fase-1","priority":8,
  "files":[{"key":"hito_video.mp4"},{"key":"hito_audio.mp3"},{"key":"hito_corrupto.mp4"}]}' \
  | python -c "import sys,json;print(json.load(sys.stdin)['id'])")
echo "caso $ID"

for i in $(seq 1 90); do
  ST=$(curl -s "$COORD/cases/$ID" | python -c "import sys,json;print(json.load(sys.stdin)['status'])")
  echo "  [$i] $ST"
  case "$ST" in completed|partially_completed|failed|cancelled) break;; esac
  sleep 2
done

[[ "$ST" == "partially_completed" ]] || { echo "FALLÓ: esperaba partially_completed, fue $ST"; exit 1; }
REP=$(curl -s "$COORD/cases/$ID/report")
echo "$REP" | python -m json.tool
echo "$REP" | grep -q '"summary": "de 3 archivos' || { echo "FALLÓ: resumen"; exit 1; }
echo "$REP" | grep -q '1 fallido'                  || { echo "FALLÓ: no cuenta el fallido"; exit 1; }
echo "$REP" | python -c "import sys,json; r=json.load(sys.stdin); assert all(s['worker_id'] for s in r['sub_tasks'] if s['status']=='completed'), 'sub-tarea sin worker'"
echo "HITO OK — $(echo "$REP" | python -c "import sys,json;print(json.load(sys.stdin)['summary'])")"
```

- [ ] **Paso 2: Correr en modo local** (worker del host) y **en modo distribuido** (node2 + node3 arriba, worker host apagado). Ambos deben dar `HITO OK`.
- [ ] **Paso 3: Registrar en `CLAUDE.md` §15** que la Fase 1 cerró.
- [ ] **Paso 4: Commit** (pedir OK) — `git add tests/case_scenario.sh && git commit -m "tests: hito de caso heterogéneo con fallo parcial"`

---

# FASE 2 — Pools especializados

**Hito:** un caso heterogéneo (video + audio + imagen) se procesa con cada sub-tarea en el nodo de su pool: video en `node1` (host), audio en `node2`, imagen en `node3`. Cambiar el rol de un nodo y ver cómo cambia la asignación.

### Task 2.1: Capabilities en el registro

**Files:** Modify `internal/models/job.go` (`WorkerInfo.Capabilities`), `cmd/worker/main.go` (`WORKER_ROLE`), `cmd/worker/main_test.go`, `internal/coordinator/registry.go`, `internal/db/db.go` (columna).

**Interfaces (Produces):**
- `WorkerInfo.Capabilities []string json:"capabilities"` — subconjunto de `{"video","audio","metadata"}`.
- `func RoleCapabilities(role string) []string` en `cmd/worker`: `video→[video]`, `audio→[audio]`, `metadata→[metadata]`, `all` o vacío → los tres.
- `func (r *Registry) LeastLoadedFor(pool string) *models.WorkerInfo` — igual que `LeastLoaded` pero solo entre workers con esa capability. `LeastLoaded()` pasa a ser `LeastLoadedFor("")` (sin filtro).

- [ ] **Paso 1: Prueba**

```go
func TestRoleCapabilities(t *testing.T) {
	if got := RoleCapabilities("audio"); len(got) != 1 || got[0] != "audio" { t.Fatal(got) }
	if got := RoleCapabilities("all"); len(got) != 3 { t.Fatal(got) }
	if got := RoleCapabilities(""); len(got) != 3 { t.Fatal(got) }
	if got := RoleCapabilities("bogus"); len(got) != 3 { t.Fatal("rol desconocido → genérico:", got) }
}
```

- [ ] **Paso 2: Implementar** — en el worker, `cfg.role = getEnv("WORKER_ROLE", "all")` y en `register()` agregar `"capabilities": RoleCapabilities(w.cfg.role)`. En `registry.go`:

```go
func (r *Registry) LeastLoadedFor(pool string) *models.WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *models.WorkerInfo
	for _, w := range r.workers {
		if !r.isAlive(w) || (pool != "" && !hasCapability(w, pool)) {
			continue
		}
		if best == nil || w.ActiveJobs < best.ActiveJobs ||
			(w.ActiveJobs == best.ActiveJobs && w.CPUPercent < best.CPUPercent) {
			best = w
		}
	}
	return best
}

func hasCapability(w *models.WorkerInfo, pool string) bool {
	for _, c := range w.Capabilities {
		if c == pool {
			return true
		}
	}
	return false
}
```
`Register` persiste `capabilities` como texto separado por comas (`ALTER TABLE worker_registry ADD COLUMN IF NOT EXISTS capabilities TEXT NOT NULL DEFAULT ''` en `Migrate`; `loadFromDB` lo vuelve a partir).

- [ ] **Paso 3: Verificar** — `go test ./cmd/worker/ -v`; arrancar un worker con `WORKER_ROLE=audio` y `curl localhost:8080/workers` debe mostrar `"capabilities": ["audio"]`.
- [ ] **Paso 4: Commit** (pedir OK) — `git commit -am "workers: capabilities por rol (video/audio/metadata/all)"`

---

### Task 2.2: Colas por pool

Cada sub-tarea va a la cola de su pool: `jobs:<pool>:<prioridad>` (9 streams). Es literalmente *"encolar cada sub-tarea en el mecanismo de distribución adecuado"*.

**Files:** Modify `internal/queue/queue.go`, `internal/coordinator/ws.go`, `cmd/coordinator/main.go` (snapshot).

**Interfaces (Produces):**

```go
var Pools = []string{"video", "audio", "metadata"}
func StreamFor(pool string, priority int) string        // "jobs:video:high"
func (q *Queue) Enqueue(ctx, job *models.Job) error       // usa job.Pool; error si vacío
func (q *Queue) Dequeue(ctx, consumerID, pool string) (*models.Job, string, error)
func (q *Queue) EnsureGroups(ctx)                         // los 9 streams
func (q *Queue) DepthByPool(ctx) (map[string]int64, error) // suma de las 3 prioridades por pool
```
`StreamHigh/Normal/Low` y `StreamForPriority` se eliminan; `Dequeue` usa `Block: 500 * time.Millisecond`.

- [ ] **Paso 1: Implementar** (reemplaza las constantes y las 3 funciones; la lógica de XReadGroup queda igual, con `streams := []string{StreamFor(pool,8), StreamFor(pool,5), StreamFor(pool,1), ">", ">", ">"}`).
- [ ] **Paso 2: Snapshot** — `QueueDepthSnapshot` gana `ByPool map[string]int `json:"by_pool"``; `High/Normal/Low` se calculan sumando por prioridad sobre los 9 streams para no romper el dashboard.
- [ ] **Paso 3: Verificar** — `go build ./...`; `docker compose -f docker-compose.infra.yml exec redis redis-cli KEYS 'jobs:*'` tras un `POST /cases` heterogéneo → aparecen `jobs:video:high`, `jobs:audio:high`, `jobs:metadata:high`.
- [ ] **Paso 4: Commit** (pedir OK) — `git commit -am "queue: un stream por pool y prioridad"`

---

### Task 2.3: Scheduler por pool

**Files:** Modify `internal/coordinator/scheduler.go` (`dispatch`).

- [ ] **Paso 1: Implementar** — `dispatch` recorre los pools; para cada uno, si hay un worker capaz, saca **una** sub-tarea de ese pool:

```go
func (s *Scheduler) dispatch(ctx context.Context) error {
	dispatched := false
	for _, pool := range queue.Pools {
		worker := s.registry.LeastLoadedFor(pool)
		if worker == nil {
			continue // sin worker para este pool: sus sub-tareas esperan en cola (visible en el dashboard)
		}
		job, msgID, err := s.queue.Dequeue(ctx, "coordinator", pool)
		if err != nil || job == nil {
			continue
		}
		dispatched = true
		s.assign(ctx, worker, job, msgID) // el cuerpo actual de dispatch desde "log.Printf assigning" en adelante
	}
	if !dispatched {
		return queue.ErrNoMessages
	}
	return nil
}
```
`assign` es el cuerpo actual (assigned → sendToWorker → 429/requeue → Ack) extraído a una función; `Ack` usa `queue.StreamFor(job.Pool, job.Priority)`.

- [ ] **Paso 2: Verificar** — con solo un worker `WORKER_ROLE=audio` conectado, un caso video+audio: la sub-tarea de audio se completa y la de video queda `pending` con `by_pool.video = 1` en `/ws`. Conectar un worker `video` → se completa.
- [ ] **Paso 3: Commit** (pedir OK) — `git commit -am "scheduler: asignación por pool (least-loaded dentro del pool)"`

---

### Task 2.4: Roles en Vagrant, compose y host

- [ ] **Paso 1:** `infra/vagrant/Vagrantfile` ya pasa `WORKER_ROLE` (`node2=audio`, `node3=metadata`). `infra/env/worker-host.env`: `WORKER_ROLE=video`. En `docker-compose.yml` (modo todo-local): `worker-1: WORKER_ROLE=video`, `worker-2: audio`, `worker-3: metadata`.
- [ ] **Paso 2:** `bash infra/vagrant/redeploy.sh` tras compilar; `vagrant provision` si cambió el env.
- [ ] **Paso 3: Commit** (pedir OK) — `git commit -am "infra: roles por nodo (video en host, audio en node2, metadata en node3)"`

---

### Task 2.5: Justificación escrita (Unidad 1)

**Files:** Modify `docs/architecture.md` — nueva sección. Texto a incluir:

> ## Modelo de asignación: pools especializados por tipo de contenido
>
> El sistema usa tres pools de workers — `video`, `audio` y `metadata` — y el coordinador enruta cada sub-tarea al pool que corresponde a su tipo de contenido. La decisión sigue la lógica de heterogeneidad de cómputo de la Unidad 1: así como una GPU es más eficiente para procesamiento paralelo masivo y una NPU para inferencia, en esta plataforma la transcodificación de video es la operación de mayor costo de CPU y se asigna al nodo con más capacidad (node-1: Ryzen 7, 6 núcleos), la conversión de audio a un nodo intermedio (node-2, 2 vCPU) y la generación de miniaturas y asociación de metadatos — operaciones livianas — al nodo de menor capacidad (node-3, 1 vCPU).
>
> **Efecto sobre el balanceo:** dentro de cada pool el scheduler aplica *least-loaded* (menos sub-tareas activas, luego menor CPU). Entre pools no hay robo de trabajo: si el pool `video` está saturado, sus sub-tareas esperan aunque `metadata` esté ocioso. Esto se acepta porque evita que un nodo débil reciba trabajo pesado y degrade el tiempo del caso completo (el barrier espera a la sub-tarea más lenta). El rol `all` permite añadir workers genéricos que cubren los tres pools, lo que da la flexibilidad de un modelo híbrido sin cambiar el scheduler.
>
> **Alternativa descartada:** workers genéricos únicamente. Es más simple y nunca deja pools ociosos, pero pierde la asignación consciente del hardware que la consigna pide justificar, y hace que una sola sub-tarea de video en un nodo lento retrase todo el caso.

- [ ] **Paso 1:** Añadir la sección. Actualizar también la sección *Job Lifecycle* para incluir el flujo caso → routing → sub-tareas → barrier → reporte.
- [ ] **Paso 2: Commit** (pedir OK) — `git commit -am "docs: justificación del modelo de pools especializados (Unidad 1)"`

---

### Task 2.6: HITO — cada sub-tarea corre en su pool

**Files:** Create `tests/pools_scenario.sh`.

- [ ] **Paso 1:** Igual que `case_scenario.sh` pero con `hito_video.mp4`, `hito_audio.mp3` y una imagen (`ffmpeg -f lavfi -i color=c=blue:s=320x240 -frames:v 1 /tmp/img.png`), y al final:

```bash
echo "$REP" | python -c "
import sys, json
r = json.load(sys.stdin)
esperado = {'video': 'node1', 'audio': 'node2', 'image': 'node3'}
for s in r['sub_tasks']:
    assert s['worker_id'] == esperado[s['file_type']], f\"{s['file']} corrió en {s['worker_id']}, esperaba {esperado[s['file_type']]}\"
print('cada sub-tarea corrió en el nodo de su pool')
"
echo "HITO OK"
```

- [ ] **Paso 2:** Correr con node1 (host, video) + node2 + node3 arriba → `HITO OK`.
- [ ] **Paso 3:** Registrar en `CLAUDE.md` §15 que la Fase 2 cerró; **escribir el Plan 2**.
- [ ] **Paso 4: Commit** (pedir OK).

---

## Auto-revisión contra la consigna

| Requisito de la consigna | Tarea(s) |
|---|---|
| Cola con consumo concurrente y prioridades | existente + 2.2 (por pool) |
| Coordinador: recibir casos | 1.4 |
| Coordinador: inspeccionar y determinar operación (routing por tipo) | 1.2, 1.4 |
| Coordinador: registrar caso y descomponer en sub-tareas | 1.1, 1.4 |
| Coordinador: registro de sub-tareas por caso y estado agregado | 1.3, 1.6, 1.9 |
| Coordinador: barrier/join | 1.5, 1.6 |
| Coordinador: asignar, monitorear, redistribuir | existente + 2.3 |
| Workers: ejecutar, reportar, comunicarse por red | 0.1, 0.2, 0.3 |
| Genéricos vs. especializados, **justificado** | 2.1–2.5 |
| ≥3 workers en entidades separadas, comunicación por red | 0.4–0.7, 2.4 |
| Estados por sub-tarea (5) | existente + `cancelled` en 1.8 |
| Estados por caso (7) | 1.1 (`queued/processing/completed/partially_completed/failed`), 1.8 (`cancelled`), 1.9 (`retrying`) |
| `completed` ⇔ todas OK; `partially_completed` ⇔ ≥1 fallida | 1.5 |
| Reporte consolidado con los 6 contenidos mínimos | 1.7 |
| Repositorio de resultados asociado a caso y sub-tarea, con descarga | 0.3, 1.7 (`results/cases/<id>/`), URL en cada sub-tarea |
| Cliente: enviar casos, consultar por caso y sub-tarea, recuperar reporte | 1.8, 1.10 |
| Cliente: generación de casos concurrentes | Plan 2 (Fase 3) — hoy `-batch` genera jobs concurrentes |
| Generación automática de casos (carpeta/metadatos) | Plan 2 (Fase 3) |
| Dataset organizado en casos | Plan 2 (Fase 3) |
| Dashboard con vista por caso y sub-tarea | Plan 2 (Fase 4) — la API que necesita queda lista en 1.8 |
| Monitoreo CPU/mem/carga por worker, sub-tareas por caso | existente (gopsutil, Prometheus) + `by_pool` 2.2; agrupación por caso en Plan 2 |
| Manual de usuario, informe de pruebas, diagramas | Plan 2 (Fase 5); base: 2.5 y los scripts de hito |
| Prueba en 3 laptops físicas | Plan 2 (Fase 6) — solo cambia `infra/env/*.env` |

**Consistencia de nombres verificada:** `cases.Route` / `RouteDecision{FileType, Operation, Pool}` (1.2) se usa en 1.4 y en `submitJob`; `db.CountJobsByCase(tx, id)` (1.3) lo consume `Barrier.OnJobResolved` (1.5); `queue.StreamFor(pool, prio)` (2.2) lo usan 1.8 y 2.3; `Registry.LeastLoadedFor(pool)` (2.1) lo usa 2.3; `storage.DatasetBucket`, `Download`, `UploadObject` (0.3) los usan el worker y 1.7.
