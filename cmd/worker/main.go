// cmd/worker/main.go
// Nodo worker: pool de goroutines, integración FFmpeg, upload a MinIO, métricas.
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/lenokeckler/mediacase-platform/internal/monitoring"
	"github.com/lenokeckler/mediacase-platform/internal/multimedia"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/websocket"
)

// ── Configuración ────────────────────────────────────────────────────────────

type workerConfig struct {
	workerID       string
	coordinatorURL string
	poolSize       int
}

func loadConfig() workerConfig {
	poolSize := 4
	if v := os.Getenv("WORKER_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			poolSize = n
		}
	}
	return workerConfig{
		workerID:       getEnv("WORKER_ID", "worker-1"),
		coordinatorURL: getEnv("COORDINATOR_URL", "http://coordinator:8080"),
		poolSize:       poolSize,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Tipos de mensajes ────────────────────────────────────────────────────────

type jobAssignment struct {
	JobID     string `json:"id"`
	FilePath  string `json:"file_path"`
	Operation string `json:"operation"`
	Priority  int    `json:"priority"`
}

// wsMsg es el mensaje del canal con el coordinador (misma forma que en internal/coordinator).
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

// ── Worker ───────────────────────────────────────────────────────────────────

type worker struct {
	cfg     workerConfig
	storage *storage.MinIOClient
	jobCh   chan jobAssignment
	wg      sync.WaitGroup
	mu      sync.Mutex
	active  int
}

func newWorker(cfg workerConfig, s *storage.MinIOClient) *worker {
	return &worker{
		cfg:     cfg,
		storage: s,
		jobCh:   make(chan jobAssignment, cfg.poolSize*2),
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

// ── Handlers HTTP ────────────────────────────────────────────────────────────

// ── Canal con el coordinador ─────────────────────────────────────────────────
// El worker abre la conexión hacia el coordinador y la mantiene viva; las sub-tareas
// llegan por ahí. Nunca escucha un puerto para recibir trabajo, así que funciona detrás
// de cualquier router o firewall sin configurar nada.

// streamLoop mantiene el canal abierto. Si se cae, reintenta con espera creciente (1 s → 30 s).
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

// serveStream atiende un canal ya abierto hasta que se cierre.
func (w *worker) serveStream(ctx context.Context, conn *websocket.Conn) {
	var sendMu sync.Mutex
	send := func(m wsMsg) {
		sendMu.Lock()
		defer sendMu.Unlock()
		if err := websocket.JSON.Send(conn, m); err != nil {
			log.Printf("[stream] envío falló: %v", err)
		}
	}
	// Cerrar el socket cuando el worker se apaga, para que Receive retorne.
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

// ── Handlers HTTP (solo diagnóstico) ─────────────────────────────────────────

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

// ── Procesamiento ────────────────────────────────────────────────────────────

func (w *worker) processJob(ctx context.Context, job jobAssignment) {
	log.Printf("[job %s] inicio — op=%s file=%s", job.JobID, job.Operation, job.FilePath)

	w.reportProgress(job.JobID, 0, string(models.StatusRunning), "", "")

	// FilePath es la clave del objeto en el bucket de entradas. Se baja a un directorio
	// temporal propio del job (así dos jobs sobre el mismo archivo no se pisan) y se borra al final.
	inDir := filepath.Join(os.TempDir(), "mediacase-in", job.JobID)
	defer os.RemoveAll(inDir) // también si la descarga falla a medias
	localInput, dlErr := w.storage.Download(ctx, storage.DatasetBucket, job.FilePath, inDir)
	if dlErr != nil {
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

	switch job.Operation {
	case string(models.OpConvert):
		resultPath, opErr = multimedia.Convert(ctx, localInput, progressCB)
	case string(models.OpExtractAudio):
		resultPath, opErr = multimedia.ExtractAudio(ctx, localInput, progressCB)
	case string(models.OpThumbnail):
		resultPath, opErr = multimedia.Thumbnail(ctx, localInput, progressCB)
	case string(models.OpConvertAudio):
		resultPath, opErr = multimedia.ConvertAudio(ctx, localInput, progressCB)
	default:
		opErr = fmt.Errorf("operación desconocida: %s", job.Operation)
	}

	if opErr != nil {
		log.Printf("[job %s] FALLÓ: %v", job.JobID, opErr)
		monitoring.JobsFailed.WithLabelValues(w.cfg.workerID, job.Operation).Inc()
		w.reportProgress(job.JobID, 0, string(models.StatusFailed), "", opErr.Error())
		return
	}

	url, uploadErr := w.storage.Upload(ctx, job.JobID, resultPath)
	if uploadErr != nil {
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
	resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
	if err != nil {
		log.Printf("[progress] POST falló para job %s: %v", jobID, err)
		return
	}
	defer resp.Body.Close()
}

// ── Registro y heartbeat ─────────────────────────────────────────────────────

func (w *worker) register() error {
	host, _ := os.Hostname()
	payload := map[string]interface{}{
		"id":       w.cfg.workerID,
		"hostname": host, // solo informativo: el coordinador ya no necesita alcanzar al worker
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(
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

			cpu, mem := monitoring.GetSystemStats()
			payload := map[string]interface{}{
				"cpu_percent": cpu,
				"mem_percent": mem,
				"active_jobs": active,
			}
			body, _ := json.Marshal(payload)
			url := fmt.Sprintf("%s/workers/%s/heartbeat", w.cfg.coordinatorURL, w.cfg.workerID)
			resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:gosec
			if err != nil {
				log.Printf("[heartbeat] falló: %v", err)
				continue
			}
			resp.Body.Close()
			// El coordinador se reinició y ya no nos conoce: volver a registrarse.
			if resp.StatusCode == http.StatusNotFound {
				log.Printf("[heartbeat] el coordinador no nos reconoce; re-registrando")
				if err := w.register(); err != nil {
					log.Printf("[heartbeat] re-registro falló: %v", err)
				}
			}
		}
	}
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()
	log.Printf("=== MediaCase Worker ===")
	log.Printf("ID=%s | pool=%d | coordinator=%s", cfg.workerID, cfg.poolSize, cfg.coordinatorURL)

	minioClient, err := storage.NewMinIOClient()
	if err != nil {
		log.Fatalf("[storage] init MinIO: %v", err)
	}

	monitoring.Init(cfg.workerID)

	w := newWorker(cfg, minioClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	go w.streamLoop(ctx) // canal saliente: por aquí llegan las sub-tareas

	mux := http.NewServeMux()
	mux.HandleFunc("/health", w.handleHealth)
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         ":8090",
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Println("[http] diagnóstico (/health, /metrics) en :8090 — opcional, no se usa para recibir trabajo")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] error fatal: %v", err)
		}
	}()

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
	log.Println("[shutdown] worker detenido limpiamente")
}
