#!/usr/bin/env bash
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
VIDEO_WORKER="${VIDEO_WORKER:-node1}"
AUDIO_WORKER="${AUDIO_WORKER:-merge-breaker}"
META_WORKER="${META_WORKER:-disruptor-specialist}"
export PYTHONIOENCODING=utf-8
py() { python -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "→ workers y sus pools:"
curl -s "$COORD/workers" | python -c "
import sys, json
for w in json.load(sys.stdin): print(f\"   {w['id']:<12} rol={w.get('role','-'):<9} pools={','.join(w.get('capabilities') or ['(genérico)'])}\")"

tmp="$(mktemp -d)"
ffmpeg -y -loglevel error -f lavfi -i "testsrc2=size=640x360:rate=25" -f lavfi -i "sine=frequency=440" -t 4 -c:v libx264 -c:a aac "$tmp/pool_video.mp4"
ffmpeg -y -loglevel error -f lavfi -i "sine=frequency=520:sample_rate=44100" -t 4 "$tmp/pool_audio.wav"
ffmpeg -y -loglevel error -f lavfi -i "mandelbrot=size=800x600" -frames:v 1 "$tmp/pool_imagen.png"
for f in pool_video.mp4 pool_audio.wav pool_imagen.png; do
  bash "$here/tests/upload_input.sh" "$tmp/$f" "$f" >/dev/null
done
rm -rf "$tmp"

ID=$(curl -s -X POST "$COORD/cases" -H 'Content-Type: application/json' -d '{
  "name":"hito-fase-2","priority":8,
  "files":[{"key":"pool_video.mp4"},{"key":"pool_audio.wav"},{"key":"pool_imagen.png"},
           {"key":"pool_video.mp4","operation":"extract_audio"}]}' | py "d['id']")
echo "→ caso $ID"

ST=""
for i in $(seq 1 90); do
  ST=$(curl -s "$COORD/cases/$ID" | py "d['status']")
  printf "   [%3ds] %s\n" "$((i*2))" "$ST"
  case "$ST" in completed|partially_completed|failed|cancelled) break;; esac
  sleep 2
done
[[ "$ST" == "completed" ]] || { echo "FALLÓ: esperaba completed, fue $ST"; curl -s "$COORD/cases/$ID/report" | python -m json.tool | head -40; exit 1; }

curl -s "$COORD/cases/$ID/report" | VIDEO_WORKER="$VIDEO_WORKER" AUDIO_WORKER="$AUDIO_WORKER" META_WORKER="$META_WORKER" python -c "
import sys, json, os
r = json.load(sys.stdin)
esperado = {'video': os.environ['VIDEO_WORKER'], 'audio': os.environ['AUDIO_WORKER'], 'image': os.environ['META_WORKER']}
print()
print('   resumen:', r['summary'])
ok = True
for s in r['sub_tasks']:
    want = esperado[s['file_type']]
    mark = 'OK ' if s['worker_id'] == want else 'MAL'
    ok = ok and s['worker_id'] == want
    print(f\"   {mark} {s['file']:<18} {s['file_type']:<6} {s['operation']:<14} corrió en {s['worker_id']:<10} (esperado {want})\")
print()
if not ok:
    print('FALLÓ: alguna sub-tarea no corrió en el pool de su tipo'); sys.exit(1)
print('HITO OK — cada sub-tarea corrió en el nodo de su pool')
"
