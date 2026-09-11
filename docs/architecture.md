# MediaCase — Architecture

## Overview

MediaCase is a distributed multimedia processing system. It receives audio and video files, distributes processing jobs across multiple worker nodes running in parallel, and provides real-time monitoring through a web dashboard.

## Component Map

```
┌─────────────────────────────────────────────────────────────┐
│                        CLIENT                               │
│              cmd/client  ·  HTTP POST /jobs                 │
└───────────────────────────┬─────────────────────────────────┘
                            │  submits jobs
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                     JOB QUEUE                               │
│              Redis Streams  ·  3 priority levels            │
│         jobs:high  ·  jobs:normal  ·  jobs:low              │
└───────────────────────────┬─────────────────────────────────┘
                            │  coordinator reads
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    COORDINATOR                              │
│  Scheduler → least-loaded worker assignment                 │
│  Registry  → heartbeat tracking, stale eviction            │
│  HTTP API  → /jobs /workers /stats /ws                      │
└──────────┬──────────────────┬──────────────────┬────────────┘
           ▼                  ▼                  ▼
    ┌────────────┐     ┌────────────┐     ┌────────────┐
    │  WORKER 1  │     │  WORKER 2  │     │  WORKER 3  │
    │ pool=4     │     │ pool=4     │     │ pool=4     │
    │ FFmpeg ops │     │ FFmpeg ops │     │ FFmpeg ops │
    │ MinIO up.  │     │ MinIO up.  │     │ MinIO up.  │
    └─────┬──────┘     └─────┬──────┘     └─────┬──────┘
          └─────────────────┬┘──────────────────┘
                            │
              ┌─────────────┴─────────────┐
              ▼                           ▼
    ┌──────────────────┐       ┌──────────────────┐
    │   PostgreSQL     │       │      MinIO        │
    │  job state       │       │  result files     │
    └──────────────────┘       └──────────────────┘
              │
              ▼
    ┌──────────────────────────┐
    │  DASHBOARD (React)       │
    │  WebSocket /ws           │
    │  worker cards, job table │
    └──────────────────────────┘
              ▲
    ┌─────────┴──────────┐
    │  Prometheus/Grafana │
    │  metrics per worker │
    └────────────────────┘
```

## Job Lifecycle

```
PENDING → ASSIGNED → RUNNING → COMPLETED
                   ↘ FAILED  → PENDING (retry, up to max_retries)
```

| Transition | Who triggers it |
|---|---|
| Created → PENDING | Coordinator on job receipt |
| PENDING → ASSIGNED | Coordinator scheduler |
| ASSIGNED → RUNNING | Worker on job start |
| RUNNING → COMPLETED | Worker after MinIO upload |
| RUNNING → FAILED | Worker on FFmpeg error |
| ASSIGNED → PENDING | Coordinator on lost heartbeat |

## Key Design Decisions

**Redis Streams for the queue** — survives coordinator restarts, supports consumer groups so Redis tracks which messages are unacknowledged. If a worker dies mid-job, the message can be redelivered.

**Three priority streams** — `jobs:high` (priority ≥ 8), `jobs:normal` (4–7), `jobs:low` (< 4). The scheduler drains high before normal before low, directly implementing multi-level queue scheduling.

**Least-loaded scheduling** — the scheduler picks the worker with the fewest active jobs, breaking ties by CPU%. This distributes work evenly without a central task queue per worker.

**Goroutine pool** — each worker runs a fixed pool of goroutines (configurable via `WORKER_POOL_SIZE`). Jobs are sent over a buffered channel. If the channel is full the coordinator gets a 429 and retries the job.

**PostgreSQL as source of truth** — Redis holds what needs to run, PostgreSQL holds what happened. The dashboard and client query PostgreSQL for rich historical data.

**MinIO for results** — workers upload processed files and store a presigned URL in PostgreSQL. Clients can download results without accessing the worker filesystem.

## Port Reference

| Service | Port | Purpose |
|---|---|---|
| Coordinator | 8080 | REST API + WebSocket |
| Workers | 8090 | Job assignment + health + metrics |
| PostgreSQL | 5432 | Job and worker state |
| Redis | 6379 | Priority job queue |
| MinIO API | 9000 | Object storage |
| MinIO UI | 9001 | Browser console |
| Prometheus | 9090 | Metrics scraping |
| Grafana | 3001 | Metrics dashboard |
| Dashboard | 5173 | Live UI |

## Deployment

### Prerequisites
- Docker Desktop running
- Go 1.26+
- `make hooks` run once after cloning

### Start the system
```bash
make up       # builds all images, starts all services
make logs     # tail logs from all services
make down     # stop and wipe all volumes
```

### Submit a test job
```bash
# Single job
go run ./cmd/client -file dataset/files/video_short_1_mp4.mp4 -op convert -watch

# Batch load test (50 concurrent)
go run ./cmd/client -batch -manifest dataset/manifest.json -concurrency 50

# Check stats
go run ./cmd/client -stats
```

### Grafana dashboards
Open http://localhost:3001 (admin / admin). The "MediaCase" dashboard is auto-provisioned and shows CPU/RAM per worker, active jobs, job throughput rates, and duration percentiles.

## Prometheus Metrics Exported by Workers

| Metric | Type | Description |
|---|---|---|
| `worker_cpu_percent` | gauge | Process CPU usage 0–100 |
| `worker_memory_mb` | gauge | RSS memory in MB |
| `worker_host_mem_percent` | gauge | Host memory usage % |
| `worker_goroutines` | gauge | Active goroutines |
| `worker_active_jobs` | gauge | Jobs currently processing |
| `worker_jobs_completed_total` | counter | Total successful jobs |
| `worker_jobs_failed_total` | counter | Total failed jobs |
| `worker_job_duration_seconds` | histogram | Processing time per operation |