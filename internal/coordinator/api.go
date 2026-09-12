package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
)

// API groups all HTTP handlers of the coordinator.
type API struct {
	queue     *queue.Queue
	registry  *Registry
	hub       *Hub       // WebSocket del dashboard
	workerHub *WorkerHub // WebSocket de los workers (canal saliente)
	db        *sql.DB
	barrier   *cases.Barrier       // cierra el caso cuando todas sus sub-tareas resolvieron
	minio     *storage.MinIOClient // entradas (dataset/) y resultados; nil si no está disponible

	// onWorkerRestart se invoca cuando un worker se registra con un ID conocido pero otra
	// instancia (proceso nuevo): sus jobs en vuelo deben volver a la cola. Lo conecta el scheduler.
	onWorkerRestart func(ctx context.Context, workerID string)
	// onCaseClosed genera el reporte consolidado (también al cancelar). Lo conecta main.
	onCaseClosed func(caseID string)
	// tunnel publica node-1 en internet desde el dashboard; nil = sin túnel (tests).
	tunnel *Tunnel
}

// SetTunnel conecta el gestor de túnel (botón "Publicar en internet" del dashboard).
func (a *API) SetTunnel(t *Tunnel) { a.tunnel = t }

// SetOnCaseClosed conecta la generación del reporte al cierre/cancelación de un caso.
func (a *API) SetOnCaseClosed(fn func(caseID string)) { a.onCaseClosed = fn }

// SetOnWorkerRestart conecta el reclaim del scheduler al registro de workers.
func (a *API) SetOnWorkerRestart(fn func(ctx context.Context, workerID string)) {
	a.onWorkerRestart = fn
}

func NewAPI(q *queue.Queue, reg *Registry, hub *Hub, workerHub *WorkerHub, database *sql.DB,
	barrier *cases.Barrier, minio *storage.MinIOClient) *API {
	return &API{queue: q, registry: reg, hub: hub, workerHub: workerHub, db: database, barrier: barrier, minio: minio}
}

// Router builds and returns the HTTP mux with all the routes.
func (a *API) Router() http.Handler {
	mux := http.NewServeMux()

	// Casos (la unidad de trabajo de la consigna v2.0)
	mux.HandleFunc("POST /cases", a.submitCase)
	mux.HandleFunc("GET /cases", a.listCases)
	mux.HandleFunc("GET /cases/{id}", a.getCase)
	mux.HandleFunc("GET /cases/{id}/report", a.getCaseReport)
	mux.HandleFunc("POST /cases/{id}/cancel", a.cancelCase)

	// Jobs sueltos (pruebas y compatibilidad)
	mux.HandleFunc("POST /jobs", a.submitJob)
	mux.HandleFunc("GET /jobs", a.listJobs)
	mux.HandleFunc("GET /jobs/{id}", a.getJob)

	// Workers
	mux.HandleFunc("POST /workers/register", a.registerWorker)
	mux.HandleFunc("POST /workers/{id}/heartbeat", a.workerHeartbeat)
	mux.HandleFunc("GET /workers/{id}/stream", a.workerHub.ServeStream) // canal saliente del worker
	mux.HandleFunc("POST /workers/{id}/unregister", a.unregisterWorker) // despedida: re-encolar lo suyo ya
	mux.HandleFunc("GET /workers", a.listWorkers)

	// Stats + WebSocket + Prometheus
	mux.HandleFunc("GET /stats", a.getStats)
	mux.HandleFunc("GET /ws", a.hub.ServeWS)
	mux.Handle("GET /metrics", MetricsHandler(a.registry, a.queue, a.db))

	// Entradas: subir al bucket dataset/ y listarlo (lo usa el dashboard para armar casos)
	mux.HandleFunc("POST /upload", a.uploadFiles)
	mux.HandleFunc("GET /dataset", a.listDataset)

	// Conectar otra máquina como worker: página + ZIP con el .env ya escrito
	mux.HandleFunc("GET /connect", a.connectPage)
	mux.HandleFunc("GET /download/worker", a.downloadWorker)

	// Compartir node-1: URLs de la LAN y túnel hacia internet manejado desde el dashboard
	mux.HandleFunc("GET /share", a.getShare)
	mux.HandleFunc("POST /tunnel", a.startTunnel)
	mux.HandleFunc("DELETE /tunnel", a.stopTunnel)

	mux.HandleFunc("POST /jobs/{id}/progress", a.jobProgress)
	mux.HandleFunc("POST /jobs/{id}/complete", a.jobComplete)
	mux.HandleFunc("POST /jobs/{id}/fail", a.jobFail)

	return mux
}

// ── Job handlers ─────────────────────────────────────────────────────────────

func (a *API) submitJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FilePath  string           `json:"file_path"`
		Operation models.Operation `json:"operation"`
		Priority  int              `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// Routing por tipo también aquí: el coordinador valida/decide la operación y el pool.
	d, err := cases.Route(req.FilePath, req.Operation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job := &models.Job{
		ID:         uuid.New().String(),
		FileID:     req.FilePath,
		FilePath:   req.FilePath,
		FileType:   d.FileType,
		Pool:       d.Pool,
		Operation:  d.Operation,
		Priority:   req.Priority,
		Status:     models.StatusPending,
		MaxRetries: 3,
		CreatedAt:  time.Now(),
	}
	if job.Priority == 0 {
		job.Priority = 5 // default: normal
	}

	if err := db.InsertJob(a.db, job); err != nil {
		log.Printf("[api] insert job: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if err := a.queue.Enqueue(r.Context(), job); err != nil {
		log.Printf("[api] enqueue: %v", err)
		http.Error(w, "queue error", http.StatusInternalServerError)
		return
	}

	log.Printf("[api] job submitted: %s op=%s priority=%d", job.ID, job.Operation, job.Priority)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(job)
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	jobs, err := db.ListJobs(a.db, status)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := db.GetJob(a.db, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// ── Worker handlers ───────────────────────────────────────────────────────────

func (a *API) registerWorker(w http.ResponseWriter, r *http.Request) {
	var info models.WorkerInfo
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	restarted := a.registry.Register(&info)
	log.Printf("[api] worker registered: %s (%s) instance=%s", info.ID, info.Hostname, shortID(info.Instance))
	if restarted && a.onWorkerRestart != nil {
		// Proceso nuevo con el mismo ID: lo que el proceso anterior tenía en vuelo se perdió.
		log.Printf("[api] worker %s es un proceso nuevo: reclamando sus sub-tareas huérfanas", info.ID)
		a.onWorkerRestart(r.Context(), info.ID)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// unregisterWorker: el worker se apaga de forma ordenada. Se da de baja y sus sub-tareas
// asignadas o en ejecución vuelven a la cola de inmediato (sin esperar los 15 s del heartbeat).
func (a *API) unregisterWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		Instance string `json:"instance"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	if !a.registry.Remove(id, payload.Instance) {
		w.WriteHeader(http.StatusNoContent) // ya no estaba (o era una instancia vieja)
		return
	}
	log.Printf("[api] worker %s se despidió; reclamando sus sub-tareas", id)
	if a.onWorkerRestart != nil {
		a.onWorkerRestart(r.Context(), id)
	}
	w.WriteHeader(http.StatusOK)
}

func (a *API) workerHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		CPU        float64 `json:"cpu_percent"`
		Mem        float64 `json:"mem_percent"`
		ActiveJobs int     `json:"active_jobs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if !a.registry.Heartbeat(id, payload.CPU, payload.Mem, payload.ActiveJobs) {
		// Worker no estaba registrado — que se registre primero
		http.Error(w, "worker not registered", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *API) listWorkers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a.registry.All())
}

// getStats: conteo de sub-tareas por estado y, además, los casos abiertos con sus sub-tareas
// agrupadas por estado (consigna: "sub-tareas activas o en espera, agrupadas por caso").
func (a *API) getStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.StatsSnapshot())
}

// StatsSnapshot arma el objeto de /stats; lo comparte el snapshot del WebSocket.
func (a *API) StatsSnapshot() map[string]any {
	stats, _ := db.GetStats(a.db)
	out := make(map[string]any, len(stats)+1)
	for k, v := range stats {
		out[k] = v
	}
	byCase, err := db.ListActiveCases(a.db)
	if err != nil {
		log.Printf("[stats] casos activos: %v", err)
	}
	out["by_case"] = byCase
	return out
}

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

	// El worker ya no escribe en la base: este handler es la única fuente de verdad
	// del estado de una sub-tarea, incluidos started_at y completed_at.
	var err error
	switch payload.Status {
	case string(models.StatusRunning):
		// Los avances de progreso viajan en conexiones distintas a la del cierre: uno rezagado
		// puede llegar DESPUÉS del completed/failed. Nunca devolver una sub-tarea terminada a
		// running (dejaba el caso en processing para siempre y a los 15 min la marcaba vencida).
		_, err = a.db.Exec(`UPDATE jobs SET status='running', progress=$1,
			started_at=COALESCE(started_at, NOW())
			WHERE id=$2 AND status IN ('pending','assigned','running')`, payload.Progress, id)
		// La primera sub-tarea que arranca mueve el caso a processing (o lo saca de retrying).
		a.db.Exec(`UPDATE cases SET status='processing', started_at=COALESCE(started_at, NOW())
			WHERE id=(SELECT case_id FROM jobs WHERE id=$1) AND status IN ('queued','retrying')`, id)
	case string(models.StatusCompleted):
		_, err = a.db.Exec(`UPDATE jobs SET status='completed', progress=100, result_url=$1,
			completed_at=NOW() WHERE id=$2 AND status <> 'cancelled'`, payload.ResultURL, id)
		JobsResolved.WithLabelValues("completed", a.poolOf(id)).Inc()
		a.resolveCase(r.Context(), id)
	case string(models.StatusFailed):
		_, err = a.db.Exec(`UPDATE jobs SET status='failed', progress=$1, error_msg=$2,
			completed_at=NOW() WHERE id=$3 AND status <> 'cancelled'`, payload.Progress, payload.ErrorMsg, id)
		JobsResolved.WithLabelValues("failed", a.poolOf(id)).Inc()
		a.resolveCase(r.Context(), id)
	case "":
		_, err = a.db.Exec(`UPDATE jobs SET progress=$1 WHERE id=$2`, payload.Progress, id)
	default:
		_, err = a.db.Exec(`UPDATE jobs SET status=$1, progress=$2 WHERE id=$3`, payload.Status, payload.Progress, id)
	}
	if err != nil {
		log.Printf("[api] progress %s: %v", id, err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// poolOf devuelve el pool de una sub-tarea (etiqueta de las métricas de throughput).
func (a *API) poolOf(jobID string) string {
	var pool sql.NullString
	if err := a.db.QueryRow(`SELECT pool FROM jobs WHERE id=$1`, jobID).Scan(&pool); err != nil || !pool.Valid || pool.String == "" {
		return "none"
	}
	return pool.String
}

func (a *API) jobComplete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		ResultURL string `json:"result_url"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	now := time.Now()
	a.db.Exec(
		`UPDATE jobs SET status='completed', progress=100, result_url=$1, completed_at=$2 WHERE id=$3`,
		payload.ResultURL, now, id,
	)
	log.Printf("[api] job %s completed, result: %s", id, payload.ResultURL)
	w.WriteHeader(http.StatusOK)
}

func (a *API) jobFail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		ErrorMsg string `json:"error_msg"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	a.db.Exec(
		`UPDATE jobs SET status='failed', error_msg=$1 WHERE id=$2`,
		payload.ErrorMsg, id,
	)
	log.Printf("[api] job %s failed: %s", id, payload.ErrorMsg)
	w.WriteHeader(http.StatusOK)
}

// resolveCase avisa al barrier que una sub-tarea del caso llegó a un estado final.
func (a *API) resolveCase(ctx context.Context, jobID string) {
	if a.barrier == nil {
		return
	}
	if err := a.barrier.OnJobResolved(ctx, caseOf(a.db, jobID)); err != nil {
		log.Printf("[barrier] job %s: %v", jobID, err)
	}
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
