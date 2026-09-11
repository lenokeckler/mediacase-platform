package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
)

// Scheduler reads jobs from the queue and assigns them to workers.
type Scheduler struct {
	queue     *queue.Queue
	registry  *Registry
	workerHub *WorkerHub
	db        *sql.DB
	barrier   *cases.Barrier
}

func NewScheduler(q *queue.Queue, reg *Registry, workerHub *WorkerHub, db *sql.DB, barrier *cases.Barrier) *Scheduler {
	return &Scheduler{queue: q, registry: reg, workerHub: workerHub, db: db, barrier: barrier}
}

// Run is the main loop - it runs indefinitely in its own goroutine.
func (s *Scheduler) Run(ctx context.Context) {
	log.Println("[scheduler] started")
	evictTicker := time.NewTicker(10 * time.Second)
	stuckTicker := time.NewTicker(30 * time.Second)
	defer evictTicker.Stop()
	defer stuckTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[scheduler] stopped")
			return

		case <-evictTicker.C:
			// Evict workers without a recent heartbeat and reclaim their jobs.
			evicted := s.registry.EvictStale()
			for _, id := range evicted {
				log.Printf("[scheduler] evicted stale worker: %s", id)
				s.ReclaimWorkerJobs(ctx, id)
			}

		case <-stuckTicker.C:
			// Mark jobs that have been running for too long as failed.
			s.reclaimStuckJobs(ctx)

		default:
			// Try to dispatch one job per pool.
			if err := s.dispatch(ctx); err != nil {
				if err != queue.ErrNoMessages && err != errNoWorkers {
					log.Printf("[scheduler] dispatch error: %v", err)
				}
				// Sin trabajo o sin workers: pausa corta para no quemar CPU.
				time.Sleep(200 * time.Millisecond)
			}
		}
	}
}

// dispatch recorre los pools: para cada uno, si hay un worker de ese pool con canal abierto,
// saca UNA sub-tarea de su cola y se la asigna (least-loaded dentro del pool). Si un pool no
// tiene workers, sus sub-tareas esperan en la cola — visible en el dashboard como by_pool.
func (s *Scheduler) dispatch(ctx context.Context) error {
	dispatched := false
	anyWorker := false
	for _, pool := range queue.Pools {
		worker := s.registry.LeastLoadedFor(pool)
		if worker == nil || !s.workerHub.IsConnected(worker.ID) {
			continue // sin worker vivo para este pool
		}
		anyWorker = true
		job, msgID, err := s.queue.Dequeue(ctx, "coordinator", pool)
		if err != nil {
			log.Printf("[scheduler] dequeue %s: %v", pool, err)
			continue
		}
		if job == nil {
			continue // cola de este pool vacía
		}
		dispatched = true
		s.assign(ctx, worker, job, msgID)
	}
	if !anyWorker {
		return errNoWorkers
	}
	if !dispatched {
		return queue.ErrNoMessages
	}
	return nil
}

var errNoWorkers = fmt.Errorf("no workers available")

// assign entrega una sub-tarea ya sacada de la cola al worker elegido.
func (s *Scheduler) assign(ctx context.Context, worker *models.WorkerInfo, job *models.Job, msgID string) {
	stream := queue.StreamFor(job.Pool, job.Priority)

	// Si el caso se canceló mientras la sub-tarea esperaba en cola, no se ejecuta.
	var st string
	s.db.QueryRow(`SELECT status FROM jobs WHERE id=$1`, job.ID).Scan(&st)
	if st == string(models.StatusCancelled) {
		s.queue.Ack(ctx, stream, msgID)
		return
	}

	log.Printf("[scheduler] assigning job %s (%s/%s) to worker %s", job.ID, job.Pool, job.Operation, worker.ID)

	if err := s.updateJobStatus(job.ID, models.StatusAssigned, worker.ID); err != nil {
		log.Printf("[scheduler] db update failed, skipping job %s: %v", job.ID, err)
		return // el mensaje queda pendiente en el stream; se reintenta
	}

	if err := s.sendToWorker(ctx, worker, job); err != nil {
		if errors.Is(err, ErrWorkerBusy) {
			log.Printf("[scheduler] worker %s is full, re-queueing job %s without incrementing retries", worker.ID, job.ID)
			if err := s.queue.Enqueue(ctx, job); err != nil {
				log.Printf("[scheduler] re-enqueue failed for job %s: %v", job.ID, err)
			}
			s.updateJobStatus(job.ID, models.StatusPending, "")
			s.queue.Ack(ctx, stream, msgID)
			time.Sleep(500 * time.Millisecond) // dar aire al worker
			return
		}
		log.Printf("[scheduler] failed to send job %s to worker %s: %v", job.ID, worker.ID, err)
		s.requeueJob(ctx, job) // otro worker del pool la tomará (cuenta reintento)
		s.updateJobStatus(job.ID, models.StatusPending, "")
		s.queue.Ack(ctx, stream, msgID)
		return
	}

	s.queue.Ack(ctx, stream, msgID)
}

// sendToWorker entrega la sub-tarea por el canal que el propio worker abrió.
// El coordinador nunca inicia una conexión hacia el worker.
func (s *Scheduler) sendToWorker(ctx context.Context, worker *models.WorkerInfo, job *models.Job) error {
	return s.workerHub.Assign(ctx, worker.ID, job)
}

// ReclaimWorkerJobs re-enqueues all ASSIGNED or RUNNING jobs of a worker that stopped
// responding (evicted) or that came back as a new process (re-registered with another instance).
func (s *Scheduler) ReclaimWorkerJobs(ctx context.Context, workerID string) {
	rows, err := s.db.QueryContext(ctx,
		`UPDATE jobs SET status='pending', worker_id=NULL
		 WHERE worker_id=$1 AND status IN ('assigned','running')
		 RETURNING id, file_path, operation, priority, retries, max_retries, COALESCE(case_id,''), file_type, pool`,
		workerID,
	)
	if err != nil {
		log.Printf("[scheduler] reclaim query failed: %v", err)
		return
	}
	defer rows.Close()
	affected := map[string]bool{}
	for rows.Next() {
		job := &models.Job{}
		if err := rows.Scan(&job.ID, &job.FilePath, &job.Operation, &job.Priority, &job.Retries, &job.MaxRetries,
			&job.CaseID, &job.FileType, &job.Pool); err != nil {
			continue
		}
		log.Printf("[scheduler] reclaimed job %s from dead worker %s — re-enqueuing", job.ID, workerID)
		if err := s.queue.Enqueue(ctx, job); err != nil {
			log.Printf("[scheduler] re-enqueue failed for reclaimed job %s: %v", job.ID, err)
		}
		if job.CaseID != "" {
			affected[job.CaseID] = true
		}
	}
	// Los casos afectados pasan a 'retrying' hasta que alguna sub-tarea vuelva a correr
	// (jobProgress los regresa a 'processing').
	for caseID := range affected {
		s.db.ExecContext(ctx, `UPDATE cases SET status='retrying' WHERE id=$1 AND status='processing'`, caseID)
		log.Printf("[scheduler] caso %s → retrying (sub-tareas re-encoladas)", caseID)
	}
}

// reclaimStuckJobs marks as failed any job that has been in 'running' state
// for longer than 15 minutes — these are jobs whose worker silently dropped them.
func (s *Scheduler) reclaimStuckJobs(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx,
		`UPDATE jobs
		 SET status='failed', completed_at=NOW(),
		     error_msg='job timed out: worker did not report completion within 15 minutes'
		 WHERE status='running'
		   AND started_at < NOW() - INTERVAL '15 minutes'
		 RETURNING id, COALESCE(case_id, '')`,
	)
	if err != nil {
		log.Printf("[scheduler] stuck-job reclaim failed: %v", err)
		return
	}
	defer rows.Close()
	n := 0
	affected := map[string]bool{}
	for rows.Next() {
		var id, caseID string
		if rows.Scan(&id, &caseID) == nil {
			n++
			if caseID != "" {
				affected[caseID] = true
			}
		}
	}
	if n > 0 {
		log.Printf("[scheduler] marked %d stuck running job(s) as failed", n)
	}
	for caseID := range affected {
		s.barrier.OnJobResolved(ctx, caseID) // una sub-tarea vencida también resuelve el barrier
	}
}

// requeueJob puts a job back in Redis.
func (s *Scheduler) requeueJob(ctx context.Context, job *models.Job) {
	job.Retries++
	if job.Retries >= job.MaxRetries {
		log.Printf("[scheduler] job %s exceeded max retries, marking failed", job.ID)
		s.db.Exec(`UPDATE jobs SET status='failed', worker_id=NULL, completed_at=NOW(),
			error_msg='no se pudo entregar a ningún worker tras '||retries||' intentos' WHERE id=$1`, job.ID)
		s.barrier.OnJobResolved(ctx, job.CaseID)
		return
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		log.Printf("[scheduler] re-enqueue failed for job %s: %v", job.ID, err)
	}
}

func (s *Scheduler) updateJobStatus(jobID string, status models.JobStatus, workerID string) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET status=$1, worker_id=NULLIF($2,'') WHERE id=$3`,
		status, workerID, jobID,
	)
	return err
}
