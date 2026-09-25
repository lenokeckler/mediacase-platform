#!/usr/bin/env bash
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
KEY="${1:-prueba.mp4}"
OP="${2:-extract_audio}"
py() { python -c "import sys,json; d=json.load(sys.stdin); print($1)"; }

echo "→ workers registrados:"
curl -s "$COORD/workers" | python -c "
import sys, json
ws = json.load(sys.stdin)
if not ws: print('   (ninguno)')
for w in ws: print(f\"   {w['id']:<14} host={w['hostname']:<14} estado={w['status']:<5} cpu={w['cpu_percent']:.0f}%\")"

echo "→ encolando: $OP sobre dataset/$KEY"
JOB=$(curl -s -X POST "$COORD/jobs" -H 'Content-Type: application/json' \
  -d "{\"file_path\":\"$KEY\",\"operation\":\"$OP\",\"priority\":5}" | py "d['id']")

ST=""; WK=""
for i in $(seq 1 90); do
  J=$(curl -s "$COORD/jobs/$JOB")
  ST=$(echo "$J" | py "d['status']")
  WK=$(echo "$J" | py "d.get('worker_id','')")
  PR=$(echo "$J" | py "d.get('progress',0)")
  printf "   [%2ds] %-10s worker=%-12s %3s%%\n" "$((i*2))" "$ST" "${WK:--}" "$PR"
  case "$ST" in completed|failed) break;; esac
  sleep 2
done

if [[ "$ST" != "completed" ]]; then
  echo "FALLÓ: status=$ST  error=$(echo "$J" | py "d.get('error_msg','')")"; exit 1
fi
if [[ -z "$WK" || "$WK" == "node1" ]]; then
  echo "FALLÓ: lo procesó '${WK:-nadie}', no un nodo remoto (¿está corriendo el worker del host?)"; exit 1
fi
echo "   resultado: $(echo "$J" | py "d['result_url']")"
echo "HITO OK — job $JOB completado por '$WK' en otra máquina"
