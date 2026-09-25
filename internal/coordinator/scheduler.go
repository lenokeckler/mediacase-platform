package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/cases"
	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
	"github.com/lib/pq"
)

type Scheduler struct {
	queue     *queue.Queue
	registry  *Registry
	workerHub *WorkerHub
	db        *sql.DB
	barrier   *cases.Barrier

	strictPools bool
}

func NewScheduler(q *queue.Queue, reg *Registry, workerHub *WorkerHub, db *sql.DB, barrier *cases.Barrier) *Scheduler {
	strict := os.Getenv("SCHEDULER_STRICT_POOLS") == "true"
	if strict {
		log.Println("[scheduler] SCHEDULER_STRICT_POOLS=true: pools estrictos, sin ayuda entre nodos")
	} else {
		log.Println("[scheduler] afinidad por pool + ayuda entre nodos + conciencia de carga")
	}
	return &Scheduler{queue: q, registry: reg, workerHub: workerHub, db: db, barrier: barrier, strictPools: strict}
}

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

			evicted := s.registry.EvictStale()
			for _, id := range evicted {
				log.Printf("[scheduler] evicted stale worker: %s", id)
				s.ReclaimWorkerJobs(ctx, id)
			}

		case <-stuckTicker.C:

			s.reclaimStuckJobs(ctx)

		default:

			if err := s.dispatch(ctx); err != nil {
				if err != queue.ErrNoMessages && err != errNoWorkers {
					log.Printf("[scheduler] dispatch error: %v", err)
				}

				time.Sleep(200 * time.Millisecond)
			}
		}
	}
}

func (s *Scheduler) dispatch(ctx context.Context) error {
	dispatched := false
	anyWorker := false
	for _, pool := range queue.Pools {
		worker, how := s.registry.PickFor(pool, s.strictPools, s.workerHub.IsConnected)
		if worker == nil {
			continue
		}
		anyWorker = true
		job, msgID, err := s.queue.Dequeue(ctx, "coordinator", pool)
		if err != nil {
			log.Printf("[scheduler] dequeue %s: %v", pool, err)
			continue
		}
		if job == nil {
			continue
		}
		dispatched = true
		job.Assignment = how
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

func (s *Scheduler) assign(ctx context.Context, worker *models.WorkerInfo, job *models.Job, msgID string) {
	stream := queue.StreamFor(job.Pool, job.Priority)

	var st string
	s.db.QueryRow(`SELECT status FROM jobs WHERE id=$1`, job.ID).Scan(&st)
	if st == string(models.StatusCancelled) {
		s.queue.Ack(ctx, stream, msgID)
		return
	}

	log.Printf("[scheduler] assigning job %s (%s/%s) to worker %s (%s)", job.ID, job.Pool, job.Operation, worker.ID, job.Assignment)

	if err := s.updateJobStatus(job.ID, models.StatusAssigned, worker.ID); err != nil {
		log.Printf("[scheduler] db update failed, skipping job %s: %v", job.ID, err)
		return
	}

	s.setAssignment(job.ID, job.Assignment)
	if err := s.sendToWorker(ctx, worker, job); err != nil {
		if errors.Is(err, ErrWorkerBusy) {
			log.Printf("[scheduler] worker %s is full, re-queueing job %s without incrementing retries", worker.ID, job.ID)
			if err := s.queue.Enqueue(ctx, job); err != nil {
				log.Printf("[scheduler] re-enqueue failed for job %s: %v", job.ID, err)
			}
			s.updateJobStatus(job.ID, models.StatusPending, "")
			s.queue.Ack(ctx, stream, msgID)
			time.Sleep(500 * time.Millisecond)
			return
		}
		log.Printf("[scheduler] failed to send job %s to worker %s: %v", job.ID, worker.ID, err)
		s.requeueJob(ctx, job)
		s.updateJobStatus(job.ID, models.StatusPending, "")
		s.queue.Ack(ctx, stream, msgID)
		return
	}

	s.registry.NoteAssigned(worker.ID)
	s.queue.Ack(ctx, stream, msgID)
}

func (s *Scheduler) sendToWorker(ctx context.Context, worker *models.WorkerInfo, job *models.Job) error {
	return s.workerHub.Assign(ctx, worker.ID, job)
}

func (s *Scheduler) ReclaimWorkerJobs(ctx context.Context, workerID string) {
	rows, err := s.db.QueryContext(ctx,
		`UPDATE jobs SET status='pending', worker_id=NULL, progress=0, started_at=NULL
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

	for caseID := range affected {
		s.db.ExecContext(ctx, `UPDATE cases SET status='retrying' WHERE id=$1 AND status='processing'`, caseID)
		log.Printf("[scheduler] caso %s → retrying (sub-tareas re-encoladas)", caseID)
	}
}

func (s *Scheduler) reclaimStuckJobs(ctx context.Context) {
	s.requeueStaleAssigned(ctx)
	rows, err := s.db.QueryContext(ctx,
		`UPDATE jobs
		 SET status='failed', completed_at=NOW(),
		     error_msg='job timed out: worker did not report completion within 15 minutes'
		 WHERE status='running'
		   AND started_at < NOW() - INTERVAL '15 minutes'
		   AND NOT (COALESCE(worker_id, '') = ANY($1))
		 RETURNING id, COALESCE(case_id, '')`,
		pq.Array(s.workerHub.Connected()),
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
		s.barrier.OnJobResolved(ctx, caseID)
	}
}

func (s *Scheduler) requeueStaleAssigned(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx,
		`UPDATE jobs SET status='pending', worker_id=NULL, progress=0, started_at=NULL
		 WHERE status='assigned' AND COALESCE(assigned_at, created_at) < NOW() - INTERVAL '15 minutes'
		 RETURNING id, file_path, operation, priority, retries, max_retries, COALESCE(case_id,''), file_type, pool`)
	if err != nil {
		log.Printf("[scheduler] stale-assigned reclaim failed: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		job := &models.Job{}
		if err := rows.Scan(&job.ID, &job.FilePath, &job.Operation, &job.Priority, &job.Retries, &job.MaxRetries,
			&job.CaseID, &job.FileType, &job.Pool); err != nil {
			continue
		}
		log.Printf("[scheduler] job %s llevaba > 15 min en assigned sin noticias — re-encolado", job.ID)
		if err := s.queue.Enqueue(ctx, job); err != nil {
			log.Printf("[scheduler] re-enqueue failed for stale job %s: %v", job.ID, err)
		}
	}
}

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
		`UPDATE jobs SET status=$1, worker_id=NULLIF($2,''),
		 assigned_at=CASE WHEN $1='assigned' THEN NOW() ELSE assigned_at END WHERE id=$3`,
		status, workerID, jobID,
	)
	return err
}

func (s *Scheduler) setAssignment(jobID, how string) {
	if how == "" {
		return
	}
	s.db.Exec(`UPDATE jobs SET assignment=$1 WHERE id=$2`, how, jobID)
}
