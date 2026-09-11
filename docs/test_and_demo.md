# Testing Results & Demo Notes

## How to Run All Tests

```bash
# 1. Start the system
make up
make logs   # in a separate terminal, watch for errors

# 2. Generate the dataset (if not already done)
bash dataset/scripts/generate_dataset.sh

# 3. Run the full load test
go run ./cmd/client -batch -manifest dataset/manifest.json -concurrency 50 -watch

# 4. After it finishes, measure processing times
bash tests/measure_times.sh

# 5. Run the failure scenario test
bash tests/failure_scenario.sh

# 6. Open the dashboard
open http://localhost:5173

# 7. Open Grafana
open http://localhost:3001   # admin / admin → MediaCase dashboard
```

---

## Processing Time Results

> Fill this in after running `bash tests/measure_times.sh` against the full dataset.

| Operation | Count | Avg (s) | p50 (s) | p90 (s) | p99 (s) |
|---|---|---|---|---|---|
| convert | — | — | — | — | — |
| extract_audio | — | — | — | — | — |
| thumbnail | — | — | — | — | — |

### Load distribution

| Worker | Total jobs | Completed | Failed | Avg time (s) |
|---|---|---|---|---|
| worker-1 | — | — | — | — |
| worker-2 | — | — | — | — |
| worker-3 | — | — | — | — |

### Throughput

- Peak: — jobs/min  
- Average: — jobs/min  
- Total jobs processed: —  
- Total time (400 files): —

---

## Failure Scenario Results

Test: kill one worker mid-job, verify retry.

| Step | Expected | Result |
|---|---|---|
| Submit 10 jobs | 10 accepted | — |
| Jobs reach running | ≥1 running | — |
| Kill active worker | Container stops | — |
| Coordinator evicts worker | Within 15s | — |
| Jobs reclaimed to pending | ≥1 reclaimed | — |
| Remaining workers pick up | Jobs resume | — |
| All jobs complete | 0 stuck | — |

---

## Demo Script (5 minutes)

**1. Show the system starting (30s)**
```bash
make down && make up && make logs
```
Point out: coordinator, 3 workers, Redis, Postgres, MinIO all starting.

**2. Submit a single job and watch it live (1 min)**
```bash
go run ./cmd/client \
  -file dataset/files/video_short_1_mp4.mp4 \
  -op convert -watch
```
Show: job goes `pending → assigned → running → completed`.  
Open dashboard at http://localhost:5173 — show the job appearing in the table with a live progress bar.

**3. Submit the full batch (2 min)**
```bash
go run ./cmd/client \
  -batch -manifest dataset/manifest.json \
  -concurrency 50 -watch
```
Switch to the dashboard — show all three worker cards lighting up with active jobs and CPU rising.  
Switch to Grafana (http://localhost:3001) — show the active jobs graph and job duration histogram.

**4. Failure scenario (1 min)**
```bash
bash tests/failure_scenario.sh
```
Or manually in another terminal:
```bash
docker compose stop worker-2
```
Point at the dashboard — worker-2 card goes offline, its jobs reappear as pending, worker-1 and worker-3 pick them up.  
Then restart: `docker compose start worker-2`

**5. Final stats (30s)**
```bash
go run ./cmd/client -stats
bash tests/measure_times.sh
```
Show the completed/failed counts and average processing times per operation.