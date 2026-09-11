#!/usr/bin/env bash
# tests/measure_times.sh
# Queries PostgreSQL directly for processing time statistics.
# Run this AFTER a load test to document performance numbers.
#
# Usage: bash tests/measure_times.sh

set -euo pipefail

DB_URL="${DATABASE_URL:-postgres://media:media@localhost:5432/mediacase?sslmode=disable}"
PSQL="docker compose exec -T postgres psql -U media -d mediacase"

echo ""
echo "═══════════════════════════════════════════════════"
echo "  MediaCase — Processing Time Report"
echo "  $(date)"
echo "═══════════════════════════════════════════════════"
echo ""

# ── Overall counts ────────────────────────────────────────────────────────────
echo "── Job Counts by Status ────────────────────────────"
$PSQL -c "
SELECT
  status,
  COUNT(*) AS count
FROM jobs
GROUP BY status
ORDER BY status;
"

# ── Processing times per operation ───────────────────────────────────────────
echo ""
echo "── Processing Time per Operation (completed jobs only) ─"
$PSQL -c "
SELECT
  operation,
  COUNT(*)                                          AS count,
  ROUND(AVG(EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2)  AS avg_sec,
  ROUND(MIN(EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2)  AS min_sec,
  ROUND(MAX(EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2)  AS max_sec,
  ROUND(PERCENTILE_CONT(0.50) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2) AS p50_sec,
  ROUND(PERCENTILE_CONT(0.90) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2) AS p90_sec,
  ROUND(PERCENTILE_CONT(0.99) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2) AS p99_sec
FROM jobs
WHERE status = 'completed'
  AND started_at IS NOT NULL
  AND completed_at IS NOT NULL
GROUP BY operation
ORDER BY operation;
"

# ── Per-worker load distribution ─────────────────────────────────────────────
echo ""
echo "── Load Distribution per Worker ────────────────────"
$PSQL -c "
SELECT
  worker_id,
  COUNT(*)                                                                   AS total_jobs,
  SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END)                     AS completed,
  SUM(CASE WHEN status = 'failed'    THEN 1 ELSE 0 END)                     AS failed,
  ROUND(AVG(EXTRACT(EPOCH FROM (completed_at - started_at)))::numeric, 2)   AS avg_sec
FROM jobs
WHERE worker_id IS NOT NULL
GROUP BY worker_id
ORDER BY worker_id;
"

# ── Queue wait time (pending→running) ─────────────────────────────────────────
echo ""
echo "── Queue Wait Time (created → started) ─────────────"
$PSQL -c "
SELECT
  operation,
  COUNT(*)                                                                        AS count,
  ROUND(AVG(EXTRACT(EPOCH FROM (started_at - created_at)))::numeric, 2)          AS avg_wait_sec,
  ROUND(PERCENTILE_CONT(0.90) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (started_at - created_at)))::numeric, 2)          AS p90_wait_sec
FROM jobs
WHERE started_at IS NOT NULL
GROUP BY operation
ORDER BY operation;
"

# ── Throughput over time ──────────────────────────────────────────────────────
echo ""
echo "── Throughput (completed jobs per minute) ──────────"
$PSQL -c "
SELECT
  DATE_TRUNC('minute', completed_at) AS minute,
  COUNT(*) AS jobs_completed
FROM jobs
WHERE status = 'completed'
  AND completed_at IS NOT NULL
GROUP BY 1
ORDER BY 1;
"

echo ""
echo "═══════════════════════════════════════════════════"
echo "  Report complete."
echo "═══════════════════════════════════════════════════"
echo ""