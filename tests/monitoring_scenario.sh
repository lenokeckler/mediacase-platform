#!/usr/bin/env bash
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
PROM="${PROMETHEUS_URL:-http://localhost:9090}"
GRAFANA="${GRAFANA_URL:-http://admin:admin@localhost:3001}"
CASES="${CASES:-20}"; CONCURRENCY="${CONCURRENCY:-5}"
export PYTHONIOENCODING=utf-8
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INGEST="$here/bin/ingest.exe"; [[ -x "$INGEST" ]] || INGEST="$here/bin/ingest"
fail() { echo "FALLÓ: $*"; exit 1; }

FAMILIES=$(curl -s "$COORD/metrics" | grep '^mediacase_' | sed 's/[{ ].*//' | sort -u)
N_FAM=$(echo "$FAMILIES" | grep -c .)
echo "→ /metrics: $N_FAM familias mediacase_*"; echo "$FAMILIES" | sed 's/^/     /'
(( N_FAM >= 6 )) || fail "menos de 6 familias"
for f in worker_cpu_percent worker_mem_percent worker_active_jobs queue_depth cases jobs; do
  echo "$FAMILIES" | grep -q "^mediacase_$f\$" || fail "falta mediacase_$f"
done

TARGET=$(curl -s "$PROM/api/v1/targets" | python -c "import sys,json; print(' '.join(t['health'] for t in json.load(sys.stdin)['data']['activeTargets']))")
echo "→ Prometheus scrape del coordinador: $TARGET"
[[ "$TARGET" == *up* ]] || fail "Prometheus no scrapea al coordinador (¿host.docker.internal:8080?)"
N_WORKERS=$(curl -s "$PROM/api/v1/query?query=mediacase_worker_cpu_percent" | python -c "import sys,json; print(len(json.load(sys.stdin)['data']['result']))")
echo "→ mediacase_worker_cpu_percent en Prometheus: $N_WORKERS workers"
(( N_WORKERS >= 3 )) || fail "Prometheus ve $N_WORKERS workers, esperaba >= 3"

DASH=$(curl -s "$GRAFANA/api/search?query=MediaCase" | python -c "import sys,json; print(' '.join(d['uid'] for d in json.load(sys.stdin)))")
echo "→ Grafana: dashboard(s) $DASH"
[[ "$DASH" == *mediacase-main* ]] || fail "Grafana no tiene el dashboard mediacase-main provisionado"

if [[ -z "${SKIP_LOAD:-}" ]]; then
  echo "→ ingest load --cases $CASES --concurrency $CONCURRENCY"
  "$INGEST" load --cases "$CASES" --concurrency "$CONCURRENCY" --group-by session --coordinator "$COORD" | tail -1
fi

SEEN_QUEUE=0; SEEN_BYCASE=0; SEEN_BUSY=0
for i in $(seq 1 24); do
  sleep 5
  read -r QV RUNNING BUSY <<< "$(curl -s "$COORD/metrics" | python -c "
import sys, re
qv = running = busy = 0
for line in sys.stdin:
    m = re.match(r'mediacase_queue_depth\{pool=\"video\",priority=\"(\w+)\"\} (\d+)', line)
    if m: qv += int(m.group(2))
    m = re.match(r'mediacase_jobs\{pool=\"\w+\",status=\"running\"\} (\d+)', line)
    if m: running += int(m.group(1))
    m = re.match(r'mediacase_worker_active_jobs\{.*\} (\d+)', line)
    if m and int(m.group(1)) > 0: busy += 1
print(qv, running, busy)")"
  read -r N_CASES SUM_RUN SUM_PEND <<< "$(curl -s "$COORD/stats" | python -c "
import sys, json
bc = json.load(sys.stdin).get('by_case') or []
print(len(bc), sum(c['running'] for c in bc), sum(c['pending'] for c in bc))")"
  printf "   [%3ds] cola video=%-4s running=%-3s workers ocupados=%s | by_case: %s casos, %s ejec., %s espera\n" \
    "$((i*5))" "$QV" "$RUNNING" "$BUSY" "$N_CASES" "$SUM_RUN" "$SUM_PEND"
  (( QV > 0 )) && SEEN_QUEUE=1
  (( N_CASES >= 3 && SUM_RUN > 0 && SUM_PEND > 0 )) && SEEN_BYCASE=1
  (( BUSY >= 3 )) && SEEN_BUSY=1
  (( SEEN_QUEUE && SEEN_BYCASE && SEEN_BUSY )) && break
done
(( SEEN_QUEUE ))  || fail "la cola del pool video nunca fue > 0"
(( SEEN_BYCASE )) || fail "/stats.by_case nunca mostró >= 3 casos con sub-tareas en ejecución y en espera"
(( SEEN_BUSY ))   || fail "nunca hubo 3 workers con sub-tareas activas a la vez"

SNAP=$(python - <<EOF
import json
try:
    import websocket
except ImportError:
    print("skip"); raise SystemExit
ws = websocket.create_connection("${COORD/http/ws}/ws", timeout=10)
d = json.loads(ws.recv()); ws.close()
print(len(d.get("by_case") or []))
EOF
)
echo "→ snapshot WebSocket: by_case con $SNAP casos"
[[ "$SNAP" == skip ]] || (( SNAP >= 1 )) || fail "el snapshot del dashboard no trae by_case"

MAXQ=$(curl -s -G "$PROM/api/v1/query" --data-urlencode 'query=max_over_time(sum(mediacase_queue_depth{pool="video"})[5m:5s])' | python -c "import sys,json; r=json.load(sys.stdin)['data']['result']; print(int(float(r[0]['value'][1])) if r else 0)")
echo "→ Prometheus: máximo de la cola video en 5 min = $MAXQ"
(( MAXQ > 0 )) || fail "Prometheus no registró cola > 0 en el pool video"

if [[ -n "${WAIT:-}" ]]; then
  echo "→ esperando a que cierren los $CASES casos…"
  while :; do
    OPEN=$(curl -s "$COORD/metrics" | grep '^mediacase_active_cases' | awk '{print $2}' || true)
    if [[ "$OPEN" == "0" ]]; then break; fi
    sleep 10
  done
  CLOSED=$(curl -s "$COORD/metrics" | grep '^mediacase_case_duration_seconds_count' | awk '{s+=$2} END {print s+0}')
  echo "→ casos observados en el histograma de duración: $CLOSED"
  (( CLOSED >= CASES )) || fail "histograma con $CLOSED casos, esperaba >= $CASES"
fi
echo
echo "HITO OK"
