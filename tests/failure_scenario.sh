#!/usr/bin/env bash
# tests/failure_scenario.sh
# Failure test: kills a worker mid-job, verifies the coordinator
# reclaims the job and another worker picks it up.
#
# Prerequisites: make up must be running before you run this.
# Usage: bash tests/failure_scenario.sh

set -euo pipefail

COORDINATOR="http://localhost:8080"
PASS=0
FAIL=0

log()  { echo "  $*"; }
ok()   { echo "  ✅ $*"; PASS=$((PASS+1)); }
fail() { echo "  ❌ $*"; FAIL=$((FAIL+1)); }

echo ""
echo "═══════════════════════════════════════════════════"
echo "  MediaCase — Failure Scenario Test"
echo "═══════════════════════════════════════════════════"
echo ""

# ── 1. Submit a batch of jobs ────────────────────────────────────────────────
echo "▶ Step 1: Submit 10 jobs to create load"

BATCH='[
  {"file_path":"dataset/files/video_short_1_mp4.mp4","operation":"convert","priority":5},
  {"file_path":"dataset/files/video_short_2_mp4.mp4","operation":"convert","priority":5},
  {"file_path":"dataset/files/video_short_3_mp4.mp4","operation":"convert","priority":5},
  {"file_path":"dataset/files/video_short_4_mp4.mp4","operation":"convert","priority":5},
  {"file_path":"dataset/files/video_short_5_mp4.mp4","operation":"convert","priority":5},
  {"file_path":"dataset/files/video_short_1_mp4.mp4","operation":"thumbnail","priority":5},
  {"file_path":"dataset/files/video_short_2_mp4.mp4","operation":"thumbnail","priority":5},
  {"file_path":"dataset/files/video_short_3_mp4.mp4","operation":"thumbnail","priority":5},
  {"file_path":"dataset/files/video_short_1_mp4.mp4","operation":"extract_audio","priority":5},
  {"file_path":"dataset/files/video_short_2_mp4.mp4","operation":"extract_audio","priority":5}
]'

SUBMIT_RESP=$(curl -s -X POST "$COORDINATOR/batch" \
  -H "Content-Type: application/json" \
  -d "$BATCH")

JOB_COUNT=$(echo "$SUBMIT_RESP" | python -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

if [ "$JOB_COUNT" -eq 10 ]; then
  ok "Submitted $JOB_COUNT jobs successfully"
else
  fail "Expected 10 jobs, got $JOB_COUNT — is the coordinator running?"
  exit 1
fi

# ── 2. Wait for jobs to start running ────────────────────────────────────────
echo ""
echo "▶ Step 2: Wait for jobs to reach 'running' state (up to 15s)"

RUNNING=0
for i in $(seq 1 15); do
  sleep 1
  RUNNING=$(curl -s "$COORDINATOR/jobs?status=running" | \
    python -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  log "  attempt $i/15 — running jobs: $RUNNING"
  if [ "$RUNNING" -gt 0 ]; then
    break
  fi
done

if [ "$RUNNING" -gt 0 ]; then
  ok "$RUNNING jobs are running"
else
  fail "No jobs reached running state — check worker logs: make logs"
  exit 1
fi

# ── 3. Record which worker is running jobs ────────────────────────────────────
echo ""
echo "▶ Step 3: Identify an active worker"

ACTIVE_WORKER=$(curl -s "$COORDINATOR/jobs?status=running" | \
  python -c "
import sys, json
jobs = json.load(sys.stdin)
workers = [j['worker_id'] for j in jobs if j.get('worker_id')]
print(workers[0] if workers else '')
" 2>/dev/null || echo "")

if [ -z "$ACTIVE_WORKER" ]; then
  fail "Could not identify an active worker"
  exit 1
fi

ok "Active worker: $ACTIVE_WORKER"

# ── 4. Kill that worker container ────────────────────────────────────────────
echo ""
echo "▶ Step 4: Kill container '$ACTIVE_WORKER'"

if docker compose stop "$ACTIVE_WORKER" > /dev/null 2>&1; then
  ok "Container '$ACTIVE_WORKER' stopped"
else
  fail "Could not stop container '$ACTIVE_WORKER' — is Docker Compose running?"
  exit 1
fi

# ── 5. Wait for coordinator to detect the lost heartbeat ─────────────────────
echo ""
echo "▶ Step 5: Wait for coordinator to evict dead worker (up to 30s)"

EVICTED=false
for i in $(seq 1 30); do
  sleep 1
  WORKER_STATUS=$(curl -s "$COORDINATOR/workers" | \
    python -c "
import sys, json, time
workers = json.load(sys.stdin)
for w in workers:
    if w['id'] == '$ACTIVE_WORKER':
        print(w['status'])
        sys.exit(0)
print('gone')
" 2>/dev/null || echo "unknown")

  log "  attempt $i/30 — $ACTIVE_WORKER status: $WORKER_STATUS"

  if [ "$WORKER_STATUS" = "gone" ] || [ "$WORKER_STATUS" = "offline" ]; then
    EVICTED=true
    break
  fi
done

if $EVICTED; then
  ok "Worker '$ACTIVE_WORKER' evicted from registry"
else
  fail "Worker '$ACTIVE_WORKER' was not evicted within 30s"
fi

# ── 6. Verify jobs were reclaimed (back to pending) ──────────────────────────
echo ""
echo "▶ Step 6: Verify jobs reclaimed back to pending"

sleep 3
PENDING=$(curl -s "$COORDINATOR/jobs?status=pending" | \
  python -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

if [ "$PENDING" -gt 0 ]; then
  ok "$PENDING jobs reclaimed to pending"
else
  # They may have already been picked up by another worker
  RUNNING_NOW=$(curl -s "$COORDINATOR/jobs?status=running" | \
    python -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  if [ "$RUNNING_NOW" -gt 0 ]; then
    ok "Jobs already picked up by another worker ($RUNNING_NOW running)"
  else
    fail "No jobs in pending or running — they may be stuck. Check: make logs"
  fi
fi

# ── 7. Restart the killed worker ─────────────────────────────────────────────
echo ""
echo "▶ Step 7: Restart '$ACTIVE_WORKER'"

if docker compose start "$ACTIVE_WORKER" > /dev/null 2>&1; then
  ok "Container '$ACTIVE_WORKER' restarted"
else
  fail "Could not restart container '$ACTIVE_WORKER'"
fi

# ── 8. Wait for all jobs to complete ─────────────────────────────────────────
echo ""
echo "▶ Step 8: Wait for all jobs to complete (up to 120s)"

ALL_DONE=false
for i in $(seq 1 60); do
  sleep 2
  COMPLETED=$(curl -s "$COORDINATOR/stats" | \
    python -c "import sys,json; s=json.load(sys.stdin); print(s.get('completed',0))" 2>/dev/null || echo "0")
  FAILED=$(curl -s "$COORDINATOR/stats" | \
    python -c "import sys,json; s=json.load(sys.stdin); print(s.get('failed',0))" 2>/dev/null || echo "0")
  STILL_RUNNING=$(curl -s "$COORDINATOR/jobs?status=running" | \
    python -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
  STILL_PENDING=$(curl -s "$COORDINATOR/jobs?status=pending" | \
    python -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

  log "  [${i}0s] completed=$COMPLETED  failed=$FAILED  running=$STILL_RUNNING  pending=$STILL_PENDING"

  if [ "$STILL_RUNNING" -eq 0 ] && [ "$STILL_PENDING" -eq 0 ]; then
    ALL_DONE=true
    break
  fi
done

if $ALL_DONE; then
  ok "All jobs finished — completed=$COMPLETED  failed=$FAILED"
else
  fail "Jobs did not all finish within 120s"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "═══════════════════════════════════════════════════"
echo "  Results: $PASS passed, $FAIL failed"
echo "═══════════════════════════════════════════════════"
echo ""

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi