#!/usr/bin/env bash
# Informe de tiempos: consulta PostgreSQL y resume, con números reales, lo que la consigna pide
# poder analizar — tiempos a nivel de sub-tarea (por operación, por pool, por nivel de tamaño)
# y a nivel de caso (por clase homogéneo/heterogéneo), distribución entre workers, espera en
# cola y throughput. Correrlo DESPUÉS de una carga (ingest load / dataset_scenario.sh).
#
#   bash tests/measure_times.sh                # todo el historial
#   SINCE='2 hours' bash tests/measure_times.sh   # solo lo creado en las últimas 2 horas
#   bash tests/measure_times.sh --markdown     # tablas Markdown para docs/informe-pruebas.md
set -euo pipefail
SINCE="${SINCE:-100 years}"
FMT="table"; [[ "${1:-}" == "--markdown" ]] && FMT="markdown"
PSQL=(docker compose -f docker-compose.infra.yml exec -T postgres psql -U media -d mediacase -v ON_ERROR_STOP=1)
export PYTHONIOENCODING=utf-8

# run <título> <sql>: imprime la consulta como tabla psql o como tabla Markdown
run() {
  local title=$1 sql=$2
  echo; echo "## $title"; echo
  if [[ $FMT == table ]]; then
    "${PSQL[@]}" -c "$sql"
  else
    "${PSQL[@]}" -A -F '|' -c "$sql" | python -c "
import sys
rows = [l.rstrip('\n') for l in sys.stdin if l.strip() and not l.startswith('(')]
if not rows: sys.exit()
head = rows[0].split('|')
print('| ' + ' | '.join(head) + ' |'); print('|' + '---:|' * len(head))
for r in rows[1:]: print('| ' + ' | '.join(r.split('|')) + ' |')"
  fi
}
W="created_at > NOW() - INTERVAL '$SINCE'"
DUR="EXTRACT(EPOCH FROM (completed_at - started_at))"
PCT() { echo "ROUND(PERCENTILE_CONT($1) WITHIN GROUP (ORDER BY $DUR)::numeric, 1)"; }

echo "# Informe de tiempos — $(date '+%Y-%m-%d %H:%M') · ventana: últimos $SINCE"

run "Sub-tareas por estado" "
SELECT status, COUNT(*) AS n FROM jobs WHERE $W GROUP BY status ORDER BY status;"

run "Casos por estado y clase (homogéneo = un solo tipo de contenido)" "
SELECT status, clase, COUNT(*) AS casos FROM (
  SELECT c.id, c.status,
         CASE WHEN COUNT(DISTINCT j.file_type) = 1 THEN 'homogéneo' ELSE 'heterogéneo' END AS clase
  FROM cases c JOIN jobs j ON j.case_id = c.id
  WHERE c.$W GROUP BY c.id, c.status) x
GROUP BY status, clase ORDER BY status, clase;"

run "Tiempo de procesamiento por operación (sub-tareas completadas, segundos)" "
SELECT operation, pool, COUNT(*) AS n,
       ROUND(AVG($DUR)::numeric, 1) AS media,
       $(PCT 0.50) AS p50, $(PCT 0.90) AS p90, $(PCT 0.99) AS p99,
       ROUND(MAX($DUR)::numeric, 1) AS max
FROM jobs WHERE status = 'completed' AND started_at IS NOT NULL AND completed_at IS NOT NULL AND $W
GROUP BY operation, pool ORDER BY pool, operation;"

run "Tiempo de procesamiento por nivel de tamaño del archivo (según el nombre del dataset)" "
SELECT CASE WHEN file_path LIKE '%\_light\_%' THEN 'liviano (< 5 MB)'
            WHEN file_path LIKE '%\_medium\_%' THEN 'mediano (20-50 MB)'
            WHEN file_path LIKE '%\_heavy\_%' THEN 'pesado (150-400 MB)'
            WHEN file_path LIKE 'image\_%' THEN 'imagen'
            ELSE 'otro' END AS nivel,
       pool, COUNT(*) AS n,
       ROUND(AVG($DUR)::numeric, 1) AS media, $(PCT 0.50) AS p50, $(PCT 0.90) AS p90,
       ROUND(MAX($DUR)::numeric, 1) AS max
FROM jobs WHERE status = 'completed' AND started_at IS NOT NULL AND completed_at IS NOT NULL AND $W
GROUP BY 1, 2 ORDER BY 2, 1;"

run "Duración de los casos (creación → cierre por el barrier), por clase y estado final" "
SELECT clase, status, COUNT(*) AS casos,
       ROUND(AVG(subt)::numeric, 1) AS sub_tareas_media,
       ROUND(AVG(dur)::numeric, 1) AS media_s,
       ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY dur)::numeric, 1) AS p50_s,
       ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY dur)::numeric, 1) AS p90_s,
       ROUND(MAX(dur)::numeric, 1) AS max_s
FROM (
  SELECT c.id, c.status, c.total_jobs AS subt,
         EXTRACT(EPOCH FROM (c.completed_at - c.created_at)) AS dur,
         CASE WHEN COUNT(DISTINCT j.file_type) = 1 THEN 'homogéneo' ELSE 'heterogéneo' END AS clase
  FROM cases c JOIN jobs j ON j.case_id = c.id
  WHERE c.completed_at IS NOT NULL AND c.$W
  GROUP BY c.id) x
GROUP BY clase, status ORDER BY clase, status;"

run "Distribución del trabajo entre workers" "
SELECT worker_id, pool, COUNT(*) AS sub_tareas,
       COUNT(*) FILTER (WHERE status = 'completed') AS ok,
       COUNT(*) FILTER (WHERE status = 'failed') AS fallidas,
       ROUND(SUM($DUR) FILTER (WHERE status = 'completed')::numeric / 60, 1) AS minutos_cpu
FROM jobs WHERE worker_id IS NOT NULL AND $W
GROUP BY worker_id, pool ORDER BY worker_id, pool;"

run "Espera en cola (creación → inicio de ejecución), por pool" "
SELECT pool, COUNT(*) AS n,
       ROUND(AVG(EXTRACT(EPOCH FROM (started_at - created_at)))::numeric, 1) AS media_s,
       ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (started_at - created_at)))::numeric, 1) AS p50_s,
       ROUND(PERCENTILE_CONT(0.9) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (started_at - created_at)))::numeric, 1) AS p90_s,
       ROUND(MAX(EXTRACT(EPOCH FROM (started_at - created_at)))::numeric, 1) AS max_s
FROM jobs WHERE started_at IS NOT NULL AND $W GROUP BY pool ORDER BY pool;"

run "Throughput: sub-tareas completadas por minuto (últimos 30 minutos con actividad)" "
SELECT to_char(DATE_TRUNC('minute', completed_at AT TIME ZONE 'America/Costa_Rica'), 'HH24:MI') AS minuto,
       COUNT(*) AS completadas,
       COUNT(*) FILTER (WHERE pool = 'video') AS video,
       COUNT(*) FILTER (WHERE pool = 'audio') AS audio,
       COUNT(*) FILTER (WHERE pool = 'metadata') AS metadata
FROM jobs WHERE status = 'completed' AND completed_at IS NOT NULL AND $W
GROUP BY 1 ORDER BY 1 DESC LIMIT 30;"
