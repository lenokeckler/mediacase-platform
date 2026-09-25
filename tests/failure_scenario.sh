#!/usr/bin/env bash
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
VICTIM="${VICTIM:-node1}"; VICTIM_PORT="${VICTIM_PORT:-8090}"
export PYTHONIOENCODING=utf-8
py() { python -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fail() { echo "FALLÓ: $*"; exit 1; }
WORKER_BIN="$here/bin/mediacase-worker-host.exe"; [[ -x "$WORKER_BIN" ]] || WORKER_BIN="$here/bin/worker-linux-amd64"
[[ -x "$WORKER_BIN" ]] || fail "falta el binario del worker (scripts/run-worker.ps1 lo compila)"
ENVFILE="$here/infra/env/worker-host.env"; [[ -f "$ENVFILE" ]] || fail "falta $ENVFILE"

echo "→ levantando worker temporal 'tmp-video' (pool video)"
( set -a; source "$ENVFILE"; set +a
  WORKER_ID=tmp-video WORKER_ROLE=video WORKER_POOL_SIZE=4 WORKER_DIAG_ADDR= "$WORKER_BIN" >/tmp/tmp-video.log 2>&1 ) &
TMP_PID=$!
for i in $(seq 1 20); do
  curl -s "$COORD/workers" | grep -q '"id":"tmp-video"' && break; sleep 1
done
curl -s "$COORD/workers" | grep -q '"id":"tmp-video"' || fail "tmp-video no se registró (ver /tmp/tmp-video.log)"
curl -s "$COORD/workers" | grep -q "\"id\":\"$VICTIM\"" || fail "$VICTIM no está conectado"

echo "→ caso de 8 videos medianos (prioridad 9)"
FILES=$(python -c "
import json
m = json.load(open(r'$(cygpath -w "$here/dataset/manifest.json" 2>/dev/null || echo "$here/dataset/manifest.json")', encoding='utf-8'))
vs = [f['key'] for f in m['files'] if f['type'] == 'video' and f['tier'] == 'medium'][:8]
print(json.dumps([{'key': k} for k in vs]))")
ID=$(curl -s -X POST "$COORD/cases" -H 'Content-Type: application/json' \
  -d "{\"name\":\"fallo-worker\",\"priority\":9,\"files\":$FILES}" | py "d['id']")
echo "   caso $ID"

for i in $(seq 1 60); do
  N=$(curl -s "$COORD/cases/$ID" | py "sum(1 for j in d['jobs'] if j['status']=='running' and j.get('worker_id')=='$VICTIM')")
  (( N >= 2 )) && break; sleep 1
done
(( N >= 2 )) || fail "$VICTIM no llegó a tener 2 sub-tareas en ejecución"
VICTIM_JOBS=$(curl -s "$COORD/cases/$ID" | py "' '.join(j['id'] for j in d['jobs'] if j.get('worker_id')=='$VICTIM' and j['status'] in ('assigned','running'))")
echo "→ $VICTIM tiene $N en ejecución; matando su proceso (kill -9, sin despedida)"
if command -v netstat.exe >/dev/null; then
  VPID=$(netstat.exe -ano | grep "LISTENING" | grep ":$VICTIM_PORT " | awk '{print $NF}' | head -1)
  [[ -n "$VPID" ]] || fail "no encuentro el proceso que escucha en :$VICTIM_PORT"
  taskkill.exe //F //PID "$VPID" >/dev/null
else
  VPID=$(ss -ltnp | grep ":$VICTIM_PORT " | sed 's/.*pid=\([0-9]*\).*/\1/' | head -1)
  [[ -n "$VPID" ]] || fail "no encuentro el proceso que escucha en :$VICTIM_PORT"
  kill -9 "$VPID"
fi
T0=$(date +%s)

SEEN_RETRY=0; SEEN_EVICT=0
for i in $(seq 1 90); do
  ST=$(curl -s "$COORD/cases/$ID" | py "d['status']")
  curl -s "$COORD/workers" | grep -q "\"id\":\"$VICTIM\"" || SEEN_EVICT=1
  [[ "$ST" == retrying ]] && SEEN_RETRY=1
  printf "   [%3ds] caso=%-10s víctima %s\n" "$(( $(date +%s) - T0 ))" "$ST" "$([[ $SEEN_EVICT == 1 ]] && echo expulsada || echo registrada)"
  case "$ST" in completed|partially_completed|failed) break;; esac
  sleep 3
done
(( SEEN_EVICT )) || fail "el coordinador nunca expulsó a $VICTIM"
(( SEEN_RETRY )) || echo "   (el caso no pasó por retrying visible: las sub-tareas se re-asignaron entre dos consultas)"
[[ "$ST" == completed ]] || fail "el caso terminó $ST, esperaba completed"

curl -s "$COORD/cases/$ID" | python -c "
import sys, json
d = json.load(sys.stdin)
victim_jobs = set('$VICTIM_JOBS'.split())
moved = [j for j in d['jobs'] if j['id'] in victim_jobs]
print()
for j in moved:
    print(f\"   {j['file_path']:<26} estaba en $VICTIM → terminó en {j['worker_id']:<10} {j['status']}\")
assert all(j['status'] == 'completed' for j in d['jobs']), 'hay sub-tareas no completadas'
assert moved and all(j['worker_id'] == 'tmp-video' for j in moved), 'las sub-tareas de la víctima no las tomó tmp-video'
assert all(j.get('worker_id') for j in d['jobs']), 'sub-tareas sin worker'
print(f'   {len(moved)} sub-tareas redistribuidas, {len(d[\"jobs\"])}/{len(d[\"jobs\"])} completadas, 0 fallidas')
"
echo "   cierre completo $(( $(date +%s) - T0 )) s después de la caída"

kill "$TMP_PID" 2>/dev/null || true
echo "→ relanzando $VICTIM"
( set -a; source "$ENVFILE"; set +a
  WORKER_ID="$VICTIM" WORKER_DIAG_ADDR=":$VICTIM_PORT" nohup "$WORKER_BIN" >/tmp/$VICTIM-relaunch.log 2>&1 & )
sleep 4
curl -s "$COORD/workers" | grep -q "\"id\":\"$VICTIM\"" && echo "   $VICTIM de vuelta" || echo "   ($VICTIM no volvió solo: scripts/run-worker.ps1)"
echo
echo "HITO OK"
