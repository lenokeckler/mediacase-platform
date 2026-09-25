#!/usr/bin/env bash
set -euo pipefail
COORD="${COORDINATOR_URL:-http://localhost:8080}"
LIMIT="${LIMIT:-10}"
export PYTHONIOENCODING=utf-8
py() { python -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="${MANIFEST:-$here/dataset/manifest.json}"
FILES_DIR="${FILES_DIR:-$here/dataset/files}"
INGEST="$here/bin/ingest.exe"; [[ -x "$INGEST" ]] || INGEST="$here/bin/ingest"
[[ -x "$INGEST" ]] || { echo "falta bin/ingest: go build -o bin/ingest.exe ./cmd/ingest"; exit 1; }
[[ -f "$MANIFEST" ]] || { echo "falta $MANIFEST: bash dataset/scripts/generate_dataset.sh"; exit 1; }

if [[ -z "${SKIP_UPLOAD:-}" ]]; then
  echo "→ subiendo dataset a MinIO (lo que falte)"
  "$INGEST" upload --dir "$FILES_DIR" --manifest "$MANIFEST" --concurrency 4
fi
N_BUCKET=$(curl -s "$COORD/dataset" | py "len(d)")
MANIFEST_PY="$MANIFEST"; command -v cygpath >/dev/null && MANIFEST_PY="$(cygpath -w "$MANIFEST")"
N_MANIFEST=$(python -c "import json; print(len(json.load(open(r'$MANIFEST_PY', encoding='utf-8'))['files']))")
echo "→ objetos en dataset/: $N_BUCKET (manifest: $N_MANIFEST)"
(( N_BUCKET >= N_MANIFEST )) || { echo "FALLÓ: faltan objetos en el bucket"; exit 1; }

BEFORE=$(curl -s "$COORD/cases" | py "len(d)")
echo "→ ingest cases --group-by session --limit $LIMIT"
"$INGEST" cases --manifest "$MANIFEST" --group-by session --limit "$LIMIT" --priority 6 --coordinator "$COORD"
IDS=$(curl -s "$COORD/cases" | python -c "
import sys, json
cs = json.load(sys.stdin)
cs.sort(key=lambda c: c['created_at'])
print(' '.join(c['id'] for c in cs[$BEFORE:]))")
N_IDS=$(echo "$IDS" | wc -w)
(( N_IDS == LIMIT )) || { echo "FALLÓ: se crearon $N_IDS casos, esperaba $LIMIT"; exit 1; }

echo "→ esperando a que cierren $N_IDS casos"
for i in $(seq 1 600); do
  OPEN=0; LINE=""
  for id in $IDS; do
    ST=$(curl -s "$COORD/cases/$id" | py "d['status']")
    case "$ST" in completed|partially_completed|failed|cancelled) ;; *) OPEN=$((OPEN+1));; esac
  done
  SUMMARY=$(curl -s "$COORD/cases" | python -c "
import sys, json, collections
ids = set('$IDS'.split())
c = collections.Counter(x['status'] for x in json.load(sys.stdin) if x['id'] in ids)
print('  '.join(f'{k}={v}' for k, v in sorted(c.items())))")
  printf "   [%4ds] %s\n" "$((i*5))" "$SUMMARY"
  (( OPEN == 0 )) && break
  sleep 5
done
(( OPEN == 0 )) || { echo "FALLÓ: quedaron $OPEN casos abiertos tras 50 min"; exit 1; }

for id in $IDS; do curl -s "$COORD/cases/$id"; echo; done | python -c "
import sys, json
cases = [json.loads(l) for l in sys.stdin if l.strip()]
hom_ok = het_ok = 0
sin_worker = []
print()
for c in cases:
    types = sorted({j['file_type'] for j in c['jobs']})
    hom = len(types) == 1
    st = c['status']
    done = sum(1 for j in c['jobs'] if j['status'] == 'completed')
    print(f\"   {c['name']:<28} {'homogéneo' if hom else 'heterogéneo':<12} {st:<20} {done}/{len(c['jobs'])} sub-tareas ok  tipos={'+'.join(types)}\")
    if hom and st == 'completed': hom_ok += 1
    if not hom and st in ('completed', 'partially_completed'): het_ok += 1
    sin_worker += [j['file_path'] for j in c['jobs'] if not j.get('worker_id')]
print()
assert hom_ok >= 1, 'ningún caso homogéneo terminó completed'
assert het_ok >= 1, 'ningún caso heterogéneo terminó completed/partially_completed'
assert not sin_worker, f'sub-tareas sin worker_id: {sin_worker}'
print(f'   homogéneos completed: {hom_ok}   heterogéneos cerrados: {het_ok}   sub-tareas sin worker: 0')
"
echo
echo "HITO OK"
