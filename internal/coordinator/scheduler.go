package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
)

// Scheduler reads jobs from the queue and assigns them to workers.
type Scheduler struct {
	queue    *queue.Queue
	registry *Registry
	db       *sql.DB
}

func NewScheduler(q *queue.Queue, reg *Registry, db *sql.DB) *Scheduler {
	return &Scheduler{queue: q, registry: reg, db: db}
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
				s.reclaimWorkerJobs(ctx, id)
			}

		case <-stuckTicker.C:
			// Mark jobs that have been running for too long as failed.
			s.reclaimStuckJobs(ctx)

		default:
			// Try to dispatch a job.
			if err := s.dispatch(ctx); err != nil {
				if err != queue.ErrNoMessages {
					log.Printf("[scheduler] dispatch error: %v", err)
				}
				// Small pause when there are no jobs to avoid high CPU usage.
				time.Sleep(200 * time.Millisecond)
			}
		}
	}
}

// dispatch takes a job from the queue and assigns it to the best available worker.
func (s *Scheduler) dispatch(ctx context.Context) error {
	worker := s.registry.LeastLoaded()
	if worker == nil {
		return fmt.Errorf("no workers available")
	}

	job, msgID, err := s.queue.Dequeue(ctx, "coordinator")
	if err != nil {
		return queue.ErrNoMessages
	}
	if job == nil {
		return queue.ErrNoMessages
	}

	log.Printf("[scheduler] assigning job %s (op: %s) to worker %s", job.ID, job.Operation, worker.ID)

	// Update the DB state to ASSIGNED.
	if err := s.updateJobStatus(job.ID, models.StatusAssigned, worker.ID); err != nil {
		// If the DB update fails, return the job to the queue with Ack+Requeue.
		log.Printf("[scheduler] db update failed, skipping job %s: %v", job.ID, err)
		return err
	}

	// Send the job to the worker over HTTP.
	if err := s.sendToWorker(ctx, worker, job); err != nil {
		if strings.Contains(err.Error(), "returned 429") {
			log.Printf("[scheduler] worker %s is full, re-queueing job %s without incrementing retries", worker.ID, job.ID)
			// Re-enqueue without incrementing retries
			if err := s.queue.Enqueue(ctx, job); err != nil {
				log.Printf("[scheduler] re-enqueue failed for job %s: %v", job.ID, err)
			}
			s.updateJobStatus(job.ID, models.StatusPending, "")
			// Sleep a bit to allow workers to clear up
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		log.Printf("[scheduler] failed to send job %s to worker %s: %v", job.ID, worker.ID, err)
		// Re-enqueue so another worker can take it (this increments retries).
		s.requeueJob(ctx, job)
		s.updateJobStatus(job.ID, models.StatusPending, "")
		return nil
	}

	// Confirm that the message was processed.
	s.queue.Ack(ctx, queue.StreamForPriority(job.Priority), msgID)
	return nil
}

// sendToWorker sends an HTTP POST to the worker with the assigned job.
func (s *Scheduler) sendToWorker(ctx context.Context, worker *models.WorkerInfo, job *models.Job) error {
	data, _ := json.Marshal(job)
	url := fmt.Sprintf("http://%s/tasks", worker.Hostname)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("worker returned %d", resp.StatusCode)
	}
	return nil
}

// reclaimWorkerJobs re-enqueues all ASSIGNED or RUNNING jobs
// from a worker that stopped responding back into the Redis queue.
func (s *Scheduler) reclaimWorkerJobs(ctx context.Context, workerID string) {
	rows, err := s.db.QueryContext(ctx,
		`UPDATE jobs SET status='pending', worker_id=NULL
		 WHERE worker_id=$1 AND status IN ('assigned','running')
		 RETURNING id, file_path, operation, priority, retries, max_retries`,
		workerID,
	)
	if err != nil {
		log.Printf("[scheduler] reclaim query failed: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		job := &models.Job{}
		if err := rows.Scan(&job.ID, &job.FilePath, &job.Operation, &job.Priority, &job.Retries, &job.MaxRetries); err != nil {
			continue
		}
		log.Printf("[scheduler] reclaimed job %s from dead worker %s — re-enqueuing", job.ID, workerID)
		if err := s.queue.Enqueue(ctx, job); err != nil {
			log.Printf("[scheduler] re-enqueue failed for reclaimed job %s: %v", job.ID, err)
		}
	}
}

// reclaimStuckJobs marks as failed any job that has been in 'running' state
// for longer than 15 minutes — these are jobs whose worker silently dropped them.
func (s *Scheduler) reclaimStuckJobs(ctx context.Context) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE jobs
		 SET status='failed',
		     error_msg='job timed out: worker did not report completion within 15 minutes'
		 WHERE status='running'
		   AND started_at < NOW() - INTERVAL '15 minutes'`,
	)
	if err != nil {
		log.Printf("[scheduler] stuck-job reclaim failed: %v", err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		log.Printf("[scheduler] marked %d stuck running job(s) as failed", n)
	}
}

// requeueJob puts a job back in Redis.
func (s *Scheduler) requeueJob(ctx context.Context, job *models.Job) {
	job.Retries++
	if job.Retries >= job.MaxRetries {
		log.Printf("[scheduler] job %s exceeded max retries, marking failed", job.ID)
		s.updateJobStatus(job.ID, models.StatusFailed, "")
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
