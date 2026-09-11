package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/coordinator"
	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
	"github.com/lenokeckler/mediacase-platform/internal/storage"
)

func main() {
	log.Println("[coordinator] starting...")

	// ── Conexión a infraestructura ─────────────────────────────────────────
	database, err := db.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	log.Println("[coordinator] database ready")

	q := queue.New(os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_PASSWORD"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.EnsureGroups(ctx)
	log.Println("[coordinator] queue ready")

	// ── Inicializar componentes ────────────────────────────────────────────
	registry := coordinator.NewRegistry(database)
	hub := coordinator.NewHub()             // dashboard
	workerHub := coordinator.NewWorkerHub() // canal saliente de cada worker

	// MinIO (opcional para el coordinador): guarda una copia del reporte junto a los resultados.
	minioClient, err := storage.NewMinIOClient()
	if err != nil {
		log.Printf("[coordinator] MinIO no disponible (%v): los reportes solo quedan en Postgres", err)
		minioClient = nil
	}

	// Barrier/join: cierra el caso cuando todas sus sub-tareas resolvieron y genera el reporte.
	barrier := cases.NewBarrier(database, nil)
	buildReport := func(caseID string) {
		c, err := db.GetCase(database, caseID)
		if err != nil {
			log.Printf("[report] get case %s: %v", caseID, err)
			return
		}
		jobs, _ := db.ListJobsByCase(database, caseID)
		rep := cases.BuildReport(c, jobs)
		raw, _ := json.MarshalIndent(rep, "", "  ")
		if err := db.SaveCaseReport(database, caseID, raw); err != nil {
			log.Printf("[report] save %s: %v", caseID, err)
		}
		if minioClient != nil {
			tmp := filepath.Join(os.TempDir(), "mediacase-report-"+caseID+".json")
			if os.WriteFile(tmp, raw, 0o644) == nil {
				if err := minioClient.UploadObject(ctx, "results", "cases/"+caseID+"/report.json", tmp); err != nil {
					log.Printf("[report] upload %s: %v", caseID, err)
				}
				os.Remove(tmp)
			}
		}
		log.Printf("[report] caso %s: %s", caseID, rep.Summary)
	}
	barrier.SetOnClose(buildReport)

	scheduler := coordinator.NewScheduler(q, registry, workerHub, database, barrier)
	api := coordinator.NewAPI(q, registry, hub, workerHub, database, barrier)
	api.SetOnWorkerRestart(scheduler.ReclaimWorkerJobs) // proceso nuevo con ID conocido → re-encolar lo suyo
	api.SetOnCaseClosed(buildReport)                    // al cancelar también hay reporte

	// ── WebSocket broadcast loop ───────────────────────────────────────────
	hub.StartBroadcastLoop(func() coordinator.SystemSnapshot {
		jobs, _ := db.ListJobs(database, "")
		stats, _ := db.GetStats(database)

		// Fetch live queue depths from Redis
		high, _ := q.StreamLen(ctx, queue.StreamHigh)
		normal, _ := q.StreamLen(ctx, queue.StreamNormal)
		low, _ := q.StreamLen(ctx, queue.StreamLow)

		return coordinator.SystemSnapshot{
			Workers: registry.All(),
			Jobs:    jobs,
			Stats:   stats,
			QueueDepth: coordinator.QueueDepthSnapshot{
				High:   int(high),
				Normal: int(normal),
				Low:    int(low),
			},
		}
	})

	// ── Scheduler en su propia goroutine ──────────────────────────────────
	go scheduler.Run(ctx)
	log.Println("[coordinator] scheduler running")

	// ── HTTP server ───────────────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      api.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[coordinator] HTTP listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[coordinator] shutting down...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
	log.Println("[coordinator] stopped")
}
