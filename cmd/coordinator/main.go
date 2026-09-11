package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/coordinator"
	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
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
	scheduler := coordinator.NewScheduler(q, registry, workerHub, database)
	api := coordinator.NewAPI(q, registry, hub, workerHub, database)

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
