package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/lenokeckler/mediacase-platform/internal/monitoring"
	"github.com/lenokeckler/mediacase-platform/internal/multimedia"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/websocket"
)

type workerConfig struct {
	workerID       string
	role           string
	coordinatorURL string
	poolSize       int
	poolAuto       bool
}

var allPools = []string{"video", "audio", "metadata"}

func RoleCapabilities(role string) []string {
	switch role {
	case "video", "audio", "metadata":
		return []string{role}
	default:
		return append([]string(nil), allPools...)
	}
}

func loadConfig() workerConfig {
	poolSize, auto := poolSizeFromEnv()
	return workerConfig{
		workerID:       getEnv("WORKER_ID", "worker-1"),
		role:           getEnv("WORKER_ROLE", "all"),
		coordinatorURL: getEnv("COORDINATOR_URL", "http://coordinator:8080"),
		poolSize:       poolSize,
		poolAuto:       auto,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type jobAssignment struct {
	JobID      string             `json:"id"`
	FilePath   string             `json:"file_path"`
	Operation  string             `json:"operation"`
	Target     string             `json:"target"`
	Width      int                `json:"width"`
	Priority   int                `json:"priority"`
	Enrichment *models.Enrichment `json:"enrichment,omitempty"`
}

type wsMsg struct {
	Type   string      `json:"type"`
	Job    *models.Job `json:"job,omitempty"`
	JobID  string      `json:"job_id,omitempty"`
	Reason string      `json:"reason,omitempty"`
}

type progressUpdate struct {
	JobID     string `json:"job_id"`
	Progress  int    `json:"progress"`
	Status    string `json:"status"`
	ResultURL string `json:"result_url,omitempty"`
	ErrorMsg  string `json:"error,omitempty"`
}

type worker struct {
	hardware *monitoring.Collector
	cfg      workerConfig
	instance string
	storage  *storage.MinIOClient
	jobCh    chan jobAssignment
	wg       sync.WaitGroup
	mu       sync.Mutex
	active   int
}

func newWorker(cfg workerConfig, s *storage.MinIOClient) *worker {
	return &worker{
		cfg:      cfg,
		instance: uuid.New().String(),
		storage:  s,
		jobCh:    make(chan jobAssignment, cfg.poolSize*2),
		hardware: monitoring.NewCollector(),
	}
}

func (w *worker) startPool(ctx context.Context) {
	for i := 0; i < w.cfg.poolSize; i++ {
		w.wg.Add(1)
		go func(slotID int) {
			defer w.wg.Done()
			log.Printf("[pool-slot-%d] goroutine iniciada", slotID)
			for {
				select {
				case <-ctx.Done():
					log.Printf("[pool-slot-%d] apagando", slotID)
					return
				case job, ok := <-w.jobCh:
					if !ok {
						return
					}
					w.mu.Lock()
					w.active++
					w.mu.Unlock()

					monitoring.ActiveJobs.WithLabelValues(w.cfg.workerID).Inc()
					start := time.Now()

					w.processJob(ctx, job)

					elapsed := time.Since(start).Seconds()
					monitoring.JobDuration.WithLabelValues(w.cfg.workerID, job.Operation).Observe(elapsed)
					monitoring.ActiveJobs.WithLabelValues(w.cfg.workerID).Dec()

					w.mu.Lock()
					w.active--
					w.mu.Unlock()
				}
			}
		}(i)
	}
}

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
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		log.Printf("[stream] canal abierto con %s", w.cfg.coordinatorURL)
		w.serveStream(ctx, conn)
		conn.Close()
		if ctx.Err() == nil {
			log.Printf("[stream] canal cerrado; reconectando")
		}
	}
}

func (w *worker) serveStream(ctx context.Context, conn *websocket.Conn) {
	var sendMu sync.Mutex
	send := func(m wsMsg) {
		sendMu.Lock()
		defer sendMu.Unlock()
		if err := websocket.JSON.Send(conn, m); err != nil {
			log.Printf("[stream] envío falló: %v", err)
		}
	}

	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	for {
		var m wsMsg
		if err := websocket.JSON.Receive(conn, &m); err != nil {
			return
		}
		if m.Type != "assign" || m.Job == nil {
			continue
		}
		if m.Job.ID == "" || m.Job.FilePath == "" || m.Job.Operation == "" {
			send(wsMsg{Type: "reject", JobID: m.Job.ID, Reason: "faltan campos requeridos"})
			continue
		}
		job := jobAssignment{JobID: m.Job.ID, FilePath: m.Job.FilePath,
			Operation: string(m.Job.Operation), Target: m.Job.Target, Width: m.Job.Width, Priority: m.Job.Priority,
			Enrichment: m.Job.Enrichment}
		select {
		case w.jobCh <- job:
			log.Printf("[assign] job %s aceptado (op=%s → %s)", job.JobID, job.Operation, job.Target)
			send(wsMsg{Type: "accept", JobID: job.JobID})
		default:
			log.Printf("[assign] pool lleno, rechazando job %s", job.JobID)
			send(wsMsg{Type: "reject", JobID: job.JobID, Reason: "pool lleno"})
		}
	}
}

func (w *worker) handleHealth(rw http.ResponseWriter, _ *http.Request) {
	w.mu.Lock()
	active := w.active
	w.mu.Unlock()
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(map[string]interface{}{
		"status":      "ok",
		"worker_id":   w.cfg.workerID,
		"active_jobs": active,
		"pool_size":   w.cfg.poolSize,
	})
}

func (w *worker) processJob(ctx context.Context, job jobAssignment) {
	log.Printf("[job %s] inicio — op=%s file=%s", job.JobID, job.Operation, job.FilePath)

	w.reportProgress(job.JobID, 0, string(models.StatusRunning), "", "")

	inDir := filepath.Join(os.TempDir(), "mediacase-in", job.JobID)
	os.RemoveAll(inDir)
	defer os.RemoveAll(inDir)
	localInput, dlErr := w.storage.Download(ctx, storage.DatasetBucket, job.FilePath, inDir)
	if dlErr != nil {
		if w.shuttingDown(ctx, job.JobID) {
			return
		}
		log.Printf("[job %s] descarga FALLÓ: %v", job.JobID, dlErr)
		monitoring.JobsFailed.WithLabelValues(w.cfg.workerID, job.Operation).Inc()
		w.reportProgress(job.JobID, 0, string(models.StatusFailed), "", "descarga de entrada: "+dlErr.Error())
		return
	}

	var resultPath string
	var opErr error

	progressCB := func(pct int) {
		w.reportProgress(job.JobID, pct, string(models.StatusRunning), "", "")
	}

	target := job.Target
	opErr = multimedia.CheckInput(localInput)
	switch {
	case opErr != nil:

	case job.Operation == string(models.OpConvert):
		if target == "" {
			target = "mp4"
		}
		resultPath, opErr = multimedia.ConvertTo(ctx, localInput, target, progressCB)
	case job.Operation == string(models.OpExtractAudio):
		if target == "" {
			target = "mp3"
		}
		resultPath, opErr = multimedia.ExtractAudioTo(ctx, localInput, target, progressCB)
	case job.Operation == string(models.OpThumbnail):
		if target == "" {
			target = "jpg"
		}
		resultPath, opErr = multimedia.ThumbnailTo(ctx, localInput, target, job.Width, progressCB)
	case job.Operation == string(models.OpConvertAudio):
		if target == "" {
			target = "flac"
		}
		resultPath, opErr = multimedia.ConvertAudioTo(ctx, localInput, target, progressCB)
	case job.Operation == string(models.OpMetadata):
		resultPath, opErr = multimedia.Metadata(ctx, localInput, progressCB)
	case job.Operation == string(models.OpEnrichAudio) || job.Operation == string(models.OpEnrichVideo):
		if target == "" {
			target = map[string]string{string(models.OpEnrichAudio): "mp3", string(models.OpEnrichVideo): "mp4"}[job.Operation]
		}
		resultPath, opErr = multimedia.Enrich(ctx, localInput, target, job.Enrichment, progressCB)
	default:
		opErr = fmt.Errorf("operación desconocida: %s", job.Operation)
	}

	if opErr != nil {
		if w.shuttingDown(ctx, job.JobID) {
			return
		}
		log.Printf("[job %s] FALLÓ: %v", job.JobID, opErr)
		monitoring.JobsFailed.WithLabelValues(w.cfg.workerID, job.Operation).Inc()
		w.reportProgress(job.JobID, 0, string(models.StatusFailed), "", multimedia.CleanError(opErr, localInput))
		return
	}

	url, uploadErr := w.storage.Upload(ctx, job.JobID, resultPath)
	if uploadErr != nil {
		if w.shuttingDown(ctx, job.JobID) {
			return
		}
		log.Printf("[job %s] upload FALLÓ: %v", job.JobID, uploadErr)
		monitoring.JobsFailed.WithLabelValues(w.cfg.workerID, job.Operation).Inc()
		w.reportProgress(job.JobID, 100, string(models.StatusFailed), "", uploadErr.Error())
		return
	}

	_ = os.Remove(resultPath)

	log.Printf("[job %s] COMPLETADO — resultado en %s", job.JobID, url)
	monitoring.JobsCompleted.WithLabelValues(w.cfg.workerID, job.Operation).Inc()
	w.reportProgress(job.JobID, 100, string(models.StatusCompleted), url, "")
}

func (w *worker) shuttingDown(ctx context.Context, jobID string) bool {

	if ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case <-time.After(500 * time.Millisecond):
			return false
		}
	}
	log.Printf("[job %s] interrumpida por apagado del worker; el coordinador la re-encolará", jobID)
	return true
}

func (w *worker) unregister() {
	url := fmt.Sprintf("%s/workers/%s/unregister", w.cfg.coordinatorURL, w.cfg.workerID)
	body, _ := json.Marshal(map[string]string{"instance": w.instance})
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
	if err != nil {
		log.Printf("[shutdown] no se pudo avisar al coordinador: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[shutdown] coordinador avisado; sus sub-tareas vuelven a la cola")
}

var apiClient = &http.Client{Timeout: 10 * time.Second}

const terminalReportRetry = 15 * time.Minute

func (w *worker) reportProgress(jobID string, pct int, status, resultURL, errMsg string) {
	payload := progressUpdate{
		JobID:     jobID,
		Progress:  pct,
		Status:    status,
		ResultURL: resultURL,
		ErrorMsg:  errMsg,
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/jobs/%s/progress", w.cfg.coordinatorURL, jobID)
	err := postReport(url, body, status, jobID)
	if err == nil {
		return
	}
	log.Printf("[progress] POST falló para job %s: %v", jobID, err)
	if status != string(models.StatusCompleted) && status != string(models.StatusFailed) {
		return
	}

	go func() {
		deadline := time.Now().Add(terminalReportRetry)
		for wait := 2 * time.Second; time.Now().Before(deadline); wait = min(wait*2, 30*time.Second) {
			time.Sleep(wait)
			if err := postReport(url, body, status, jobID); err == nil {
				log.Printf("[progress] reporte %s de job %s entregado tras reintentos", status, jobID)
				return
			}
		}
		log.Printf("[progress] reporte %s de job %s perdido: el coordinador no respondió en %s", status, jobID, terminalReportRetry)
	}()
}

func postReport(url string, body []byte, status, jobID string) error {
	resp, err := apiClient.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("coordinador respondió %d al reporte %s de job %s", resp.StatusCode, status, jobID)
	}
	return nil
}

func (w *worker) register() error {
	host, _ := os.Hostname()
	payload := map[string]interface{}{
		"id":           w.cfg.workerID,
		"instance":     w.instance,
		"hostname":     host,
		"role":         w.cfg.role,
		"capabilities": RoleCapabilities(w.cfg.role),
		"capacity":     w.cfg.poolSize,
		"hardware":     w.hardware.Hardware(),
	}
	body, _ := json.Marshal(payload)
	resp, err := apiClient.Post(
		w.cfg.coordinatorURL+"/workers/register",
		"application/json",
		bytes.NewReader(body),
	) //nolint:gosec
	if err != nil {
		return fmt.Errorf("POST /workers/register falló: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("registro retornó %d", resp.StatusCode)
	}
	log.Printf("[register] registrado como %s en %s", w.cfg.workerID, w.cfg.coordinatorURL)
	return nil
}

func (w *worker) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			active := w.active
			w.mu.Unlock()

			m := w.hardware.Last()
			payload := map[string]interface{}{
				"cpu_percent": m.CPUPercent,
				"mem_percent": m.MemPercent,
				"active_jobs": active,
				"metrics":     m,
			}
			body, _ := json.Marshal(payload)
			url := fmt.Sprintf("%s/workers/%s/heartbeat", w.cfg.coordinatorURL, w.cfg.workerID)
			resp, err := apiClient.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
			if err != nil {
				log.Printf("[heartbeat] falló: %v", err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				log.Printf("[heartbeat] el coordinador no nos reconoce; re-registrando")
				if err := w.register(); err != nil {
					log.Printf("[heartbeat] re-registro falló: %v", err)
				}
			}
		}
	}
}

func main() {
	cfg := loadConfig()
	log.Printf("=== MediaCase Worker ===")
	origin := "fijada en WORKER_POOL_SIZE"
	if cfg.poolAuto {
		origin = "según el hardware"
	}
	log.Printf("ID=%s | rol=%s (%v) | capacidad=%d (%s) | coordinator=%s", cfg.workerID, cfg.role, RoleCapabilities(cfg.role), cfg.poolSize, origin, cfg.coordinatorURL)

	if err := killChildrenWithWorker(); err != nil {
		log.Printf("[worker] aviso: si este proceso muere, sus ffmpeg podrían quedar huérfanos: %v", err)
	}

	if err := multimedia.CheckTools(); err != nil {
		log.Fatalf("[worker] %v — instalar ffmpeg (Windows: usar el ZIP de /connect o winget install Gyan.FFmpeg; Linux: apt install ffmpeg)", err)
	}

	minioClient, err := storage.NewMinIOClient()
	if err != nil {
		log.Fatalf("[storage] init MinIO: %v", err)
	}

	monitoring.Init(cfg.workerID)

	w := newWorker(cfg, minioClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.hardware.Run(ctx)
	w.startPool(ctx)

	for i := 0; i < 10; i++ {
		if err := w.register(); err == nil {
			break
		} else {
			log.Printf("[register] intento %d falló: %v — reintentando en 3s", i+1, err)
			time.Sleep(3 * time.Second)
		}
	}

	go w.heartbeatLoop(ctx)
	go w.streamLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", w.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	diagAddr := getEnv("WORKER_DIAG_ADDR", ":8090")
	srv := &http.Server{
		Addr:         diagAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[http] diagnóstico deshabilitado (%s ocupado): %v", diagAddr, err)
			return
		}
	}()
	log.Printf("[http] diagnóstico (/health, /metrics) en %s — opcional, no se usa para recibir trabajo", diagAddr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[shutdown] señal recibida, apagando...")

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)

	log.Println("[shutdown] esperando jobs en vuelo...")
	w.wg.Wait()
	w.unregister()
	log.Println("[shutdown] worker detenido limpiamente")
}
