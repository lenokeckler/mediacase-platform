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

type API struct {
	queue           *queue.Queue
	registry        *Registry
	hub             *Hub
	workerHub       *WorkerHub
	db              *sql.DB
	barrier         *cases.Barrier
	minio           *storage.MinIOClient
	datasetManifest *manifestCache

	onWorkerRestart func(ctx context.Context, workerID string)

	onCaseClosed func(caseID string)

	tunnel *Tunnel
}

func (a *API) SetTunnel(t *Tunnel) { a.tunnel = t }

func (a *API) SetOnCaseClosed(fn func(caseID string)) { a.onCaseClosed = fn }

func (a *API) SetOnWorkerRestart(fn func(ctx context.Context, workerID string)) {
	a.onWorkerRestart = fn
}

func NewAPI(q *queue.Queue, reg *Registry, hub *Hub, workerHub *WorkerHub, database *sql.DB,
	barrier *cases.Barrier, minio *storage.MinIOClient) *API {
	return &API{queue: q, registry: reg, hub: hub, workerHub: workerHub, db: database, barrier: barrier, minio: minio,
		datasetManifest: &manifestCache{}}
}

func (a *API) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /cases", a.submitCase)
	mux.HandleFunc("GET /cases", a.listCases)
	mux.HandleFunc("GET /cases/{id}", a.getCase)
	mux.HandleFunc("GET /cases/{id}/report", a.getCaseReport)
	mux.HandleFunc("POST /cases/{id}/cancel", a.cancelCase)

	mux.HandleFunc("POST /jobs", a.submitJob)
	mux.HandleFunc("GET /jobs", a.listJobs)
	mux.HandleFunc("GET /jobs/{id}", a.getJob)

	mux.HandleFunc("POST /workers/register", a.registerWorker)
	mux.HandleFunc("POST /workers/{id}/heartbeat", a.workerHeartbeat)
	mux.HandleFunc("GET /workers/{id}/stream", a.workerHub.ServeStream)
	mux.HandleFunc("POST /workers/{id}/unregister", a.unregisterWorker)
	mux.HandleFunc("GET /workers", a.listWorkers)

	mux.HandleFunc("GET /stats", a.getStats)
	mux.HandleFunc("GET /ws", a.hub.ServeWS)
	mux.Handle("GET /metrics", MetricsHandler(a.registry, a.queue, a.db))

	mux.HandleFunc("POST /upload", a.uploadFiles)
	mux.HandleFunc("GET /dataset", a.listDataset)
	mux.HandleFunc("GET /dataset/test-cases", a.datasetTestCases)

	mux.HandleFunc("GET /connect", a.connectPage)
	mux.HandleFunc("GET /download/worker", a.downloadWorker)

	mux.HandleFunc("GET /catalog", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, cases.GetCatalog()) })
	mux.HandleFunc("GET /share", a.getShare)
	mux.HandleFunc("POST /tunnel", a.startTunnel)
	mux.HandleFunc("DELETE /tunnel", a.stopTunnel)

	mux.HandleFunc("POST /jobs/{id}/progress", a.jobProgress)
	mux.HandleFunc("POST /jobs/{id}/complete", a.jobComplete)
	mux.HandleFunc("POST /jobs/{id}/fail", a.jobFail)

	return requireUTF8JSON(mux)
}

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
		job.Priority = 5
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

func (a *API) registerWorker(w http.ResponseWriter, r *http.Request) {
	var info models.WorkerInfo
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	restarted := a.registry.Register(&info)
	log.Printf("[api] worker registered: %s (%s) instance=%s", info.ID, info.Hostname, shortID(info.Instance))
	if restarted && a.onWorkerRestart != nil {

		log.Printf("[api] worker %s es un proceso nuevo: reclamando sus sub-tareas huérfanas", info.ID)
		a.onWorkerRestart(r.Context(), info.ID)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (a *API) unregisterWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload struct {
		Instance string `json:"instance"`
	}
	json.NewDecoder(r.Body).Decode(&payload)
	if !a.registry.Remove(id, payload.Instance) {
		w.WriteHeader(http.StatusNoContent)
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
		CPU        float64             `json:"cpu_percent"`
		Mem        float64             `json:"mem_percent"`
		ActiveJobs int                 `json:"active_jobs"`
		Metrics    *models.NodeMetrics `json:"metrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if !a.registry.Heartbeat(id, payload.CPU, payload.Mem, payload.ActiveJobs, payload.Metrics) {

		http.Error(w, "worker not registered", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *API) listWorkers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a.registry.All())
}

func (a *API) getStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.StatsSnapshot())
}

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

	var err error
	switch payload.Status {
	case string(models.StatusRunning):

		_, err = a.db.Exec(`UPDATE jobs SET status='running', progress=$1,
			started_at=COALESCE(started_at, NOW())
			WHERE id=$2 AND status IN ('pending','assigned','running')`, payload.Progress, id)

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
