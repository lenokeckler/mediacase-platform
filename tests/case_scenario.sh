#!/usr/bin/env bash
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
export PYTHONIOENCODING=utf-8
py() { python -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

tmp="$(mktemp -d)"
ffmpeg -y -loglevel error -f lavfi -i "testsrc2=size=320x240:rate=25" -f lavfi -i "sine=frequency=440" -t 3 -c:v libx264 -c:a aac "$tmp/hito_video.mp4"
ffmpeg -y -loglevel error -f lavfi -i "sine=frequency=330:sample_rate=44100" -t 3 "$tmp/hito_audio.wav"
echo "esto no es un video" > "$tmp/hito_corrupto.mp4"
for f in hito_video.mp4 hito_audio.wav hito_corrupto.mp4; do
  bash "$here/tests/upload_input.sh" "$tmp/$f" "$f" >/dev/null
done
rm -rf "$tmp"
echo "→ entradas en MinIO: hito_video.mp4, hito_audio.wav, hito_corrupto.mp4"

ID=$(curl -s -X POST "$COORD/cases" -H 'Content-Type: application/json' -d '{
  "name":"hito-fase-1","priority":8,
  "files":[{"key":"hito_video.mp4"},{"key":"hito_audio.wav"},{"key":"hito_corrupto.mp4"}]}' | py "d['id']")
echo "→ caso $ID"

curl -s "$COORD/cases/$ID" | python -c "
import sys, json
c = json.load(sys.stdin)
for j in c['jobs']:
    print(f\"   {j['file_path']:<20} {j['file_type']:<6} -> {j['operation']:<14} pool={j['pool']}\")"

ST=""
for i in $(seq 1 90); do
  ST=$(curl -s "$COORD/cases/$ID" | py "d['status']")
  printf "   [%3ds] %s\n" "$((i*2))" "$ST"
  case "$ST" in completed|partially_completed|failed|cancelled) break;; esac
  sleep 2
done

[[ "$ST" == "partially_completed" ]] || { echo "FALLÓ: esperaba partially_completed, fue $ST"; exit 1; }
REP=$(curl -s "$COORD/cases/$ID/report")
echo "$REP" | python -c "
import sys, json
r = json.load(sys.stdin)
print()
print('   resumen :', r['summary'])
print('   duración:', round(r['duration_seconds'], 2), 's')
for s in r['sub_tasks']:
    print(f\"   {s['file']:<20} {s['operation']:<14} {s['status']:<10} {s.get('worker_id','-'):<10} {s['duration_seconds']:.2f}s  {s.get('error','')[:50]}\")
t = r['totals']
assert t['total'] == 3 and t['completed'] == 2 and t['failed'] == 1, f'totales: {t}'
assert r['summary'].startswith('de 3 archivos'), r['summary']
assert '1 fallido' in r['summary'], r['summary']
ops = {s['file']: s['operation'] for s in r['sub_tasks']}
assert ops['hito_video.mp4'] == 'convert' and ops['hito_audio.wav'] == 'convert_audio', 'routing por tipo incorrecto: %s' % ops
bad = [s for s in r['sub_tasks'] if s['file'] == 'hito_corrupto.mp4'][0]
assert bad['status'] == 'failed' and bad['error'], 'la corrupta debe fallar con detalle'
assert all(s['worker_id'] for s in r['sub_tasks'] if s['status'] == 'completed'), 'sub-tarea sin worker'
assert all(s['started_at'] and s['completed_at'] for s in r['sub_tasks'] if s['status'] == 'completed'), 'faltan tiempos'
assert r['started_at'] and r['completed_at'], 'faltan tiempos del caso'
print()
print('HITO OK —', r['summary'])
"
