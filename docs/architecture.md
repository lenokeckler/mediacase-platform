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

## Ciclo de vida de un caso (consigna v2.0)

La unidad de trabajo es el **caso**: un conjunto de archivos relacionados que entra como una
sola solicitud (`POST /cases`) y se procesa como un grupo de sub-tareas distribuidas.

```
POST /cases {files:[a.mp4, b.mp3, c.jpg]}
        │
        ▼
 1. ROUTING POR TIPO  (internal/cases/router.go)
    el coordinador inspecciona cada archivo y decide operación y pool:
      a.mp4 → video  → convert        → pool video
      b.mp3 → audio  → convert_audio  → pool audio
      c.jpg → image  → thumbnail      → pool metadata
    el cliente puede sugerir una operación; solo se acepta si aplica a ese tipo.
        │
        ▼
 2. REGISTRO Y DESCOMPOSICIÓN   cases(id, status=queued, total_jobs=3)
                                jobs(id, case_id, file_type, pool, operation, ...)
        │
        ▼
 3. ENCOLADO POR POOL Y PRIORIDAD   Redis Streams  jobs:<pool>:<high|normal|low>
        │
        ▼
 4. ASIGNACIÓN   por cada pool con un worker vivo: least-loaded dentro del pool
                 la sub-tarea viaja por el WebSocket que el worker mantiene abierto
        │
        ▼
 5. EJECUCIÓN CONCURRENTE   cada worker baja la entrada de MinIO, corre ffmpeg,
                            sube el resultado, reporta progreso por HTTP
        │
        ▼
 6. BARRIER / JOIN   (internal/cases/barrier.go)
    cada vez que una sub-tarea llega a completed/failed, el coordinador bloquea la fila
    del caso (SELECT ... FOR UPDATE) y cuenta. Solo cuando resueltas == total:
      todas OK           → completed
      alguna falló       → partially_completed
      todas fallaron     → failed
    Mientras falte una, el caso sigue abierto aunque las demás ya estén listas.
        │
        ▼
 7. REPORTE CONSOLIDADO   (internal/cases/report.go)
    cases.report (JSONB) + MinIO results/cases/<id>/report.json
    GET /cases/{id}/report
```

Estados del caso: `queued → processing → completed | partially_completed | failed`, más
`retrying` (sus sub-tareas fueron re-encoladas porque el worker que las tenía murió) y
`cancelled` (`POST /cases/{id}/cancel`: las sub-tareas no iniciadas se cancelan, las que corren
terminan, el caso no vuelve a cambiar).

El barrier se dispara en **cuatro** puntos, y en los cuatro llega al mismo código:
el worker reporta `completed`/`failed`; se agotan los reintentos de entrega; una sub-tarea
excede el tiempo máximo en `running`; y (para `retrying`) el reclaim de un worker caído.

## Modelo de asignación: pools especializados por tipo de contenido (Unidad 1)

El sistema usa tres pools de workers — `video`, `audio` y `metadata` — y el coordinador enruta
cada sub-tarea al pool que corresponde a su tipo de contenido. Cada worker declara su rol al
registrarse (`WORKER_ROLE=video|audio|metadata|all`), y el scheduler solo saca una sub-tarea de
la cola de un pool cuando hay un worker de ese pool con capacidad.

**Por qué especializados y no genéricos.** La decisión sigue la lógica de heterogeneidad de
cómputo de la Unidad 1: así como una GPU es más eficiente para procesamiento paralelo masivo y
una NPU para inferencia, en esta plataforma las operaciones tienen perfiles de costo muy
distintos, y conviene asignarlas al nodo que mejor las atiende:

| Pool | Operaciones | Perfil de cómputo | Nodo que lo atiende |
|---|---|---|---|
| `video` | `convert` (transcodificación H.264), `extract_audio`, `thumbnail` de video | CPU intensivo y sostenido: minutos por archivo pesado | **node-1**, el más potente (Ryzen 7, 6 núcleos / 12 hilos) |
| `audio` | `convert_audio` | CPU moderado, segundos | node-2 (2 vCPU) |
| `metadata` | `thumbnail` de imágenes, asociación de metadatos | Liviano, sub-segundo | node-3 (1 vCPU), el más modesto |

Con workers genéricos, una sub-tarea de video podía caer en el nodo más débil y **retrasar el
cierre de todo el caso**, porque el barrier espera a la sub-tarea más lenta. Con pools, el trabajo
pesado va siempre al nodo que lo termina antes.

**Efecto sobre el balanceo de carga.** Dentro de cada pool el scheduler aplica *least-loaded*:
elige el worker con menos sub-tareas activas y, en empate, el de menor CPU (reportada por
heartbeat cada segundo). Entre pools **no hay robo de trabajo**: si el pool `video` está saturado,
sus sub-tareas esperan en `jobs:video:*` aunque `metadata` esté ocioso. Se acepta ese costo a
cambio de que un nodo débil nunca reciba trabajo pesado. La saturación es visible en el dashboard
(`queue_depth.by_pool`), que es lo que la consigna pide poder observar.

**Flexibilidad.** El rol `all` declara las tres capacidades; un worker así (por ejemplo el que
cualquiera descarga desde `/connect`) atiende cualquier pool y actúa como comodín, lo que da un
modelo híbrido sin cambiar el scheduler. Un worker que no declara rol se trata como genérico.

**Alternativa descartada: solo workers genéricos.** Es más simple y nunca deja pools ociosos,
pero pierde la asignación consciente del hardware que la consigna pide justificar, y hace que la
duración de un caso dependa del nodo más lento que haya tocado alguna de sus sub-tareas.

## Key Design Decisions

**Redis Streams for the queue** — survives coordinator restarts, supports consumer groups so Redis tracks which messages are unacknowledged. If a worker dies mid-job, the message can be redelivered.

**Nine streams: one per (pool, priority)** — `jobs:<video|audio|metadata>:<high|normal|low>` (priority ≥ 8 → high, 4–7 → normal, < 4 → low). Within a pool the scheduler drains high before normal before low (multi-level queue scheduling); across pools it only reads a queue when a worker of that pool is available.

**Least-loaded scheduling within the pool** — the scheduler picks, among the live workers of the sub-task's pool, the one with the fewest active jobs, breaking ties by CPU%. See "Modelo de asignación" above.

**Outbound worker channel** — each worker opens a WebSocket *to* the coordinator (`GET /workers/{id}/stream`) and keeps it alive; assignments travel down that channel. The coordinator never connects to a worker, so workers need no open port, no firewall rule and no reachable IP: they run behind any home router. If the channel drops the worker reconnects with exponential backoff; if the worker comes back as a new process (a new `instance` id), its in-flight sub-tasks are re-queued immediately.

**Goroutine pool** — each worker runs a fixed pool of goroutines (configurable via `WORKER_POOL_SIZE`). Jobs are sent over a buffered channel. If the channel is full the worker replies `reject` and the coordinator re-queues the sub-task without counting a retry.

**PostgreSQL as source of truth** — Redis holds what needs to run, PostgreSQL holds what happened. The dashboard and client query PostgreSQL for rich historical data.

**MinIO for inputs and results** — inputs live in the `dataset` bucket and workers download only the object they were assigned, so no node needs a local copy of the dataset (this is what makes the downloadable worker portable). Results go to `results/jobs/<job>/...` and each case report to `results/cases/<case>/report.json`. Storage is centralized on node-1 ("almacenamiento local compartido", one of the options the assignment allows); what is distributed is the processing. node-1 is therefore a single point of failure, accepted and documented: everything it holds persists to disk and nothing is lost on restart.

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