#!/usr/bin/env bash
# dataset/scripts/generate_dataset.sh
# Genera 400+ archivos multimedia sintéticos para pruebas de carga.
# Requiere: ffmpeg y python3 instalados.
# Uso:
#   chmod +x dataset/scripts/generate_dataset.sh
#   ./dataset/scripts/generate_dataset.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT_DIR="$ROOT_DIR/dataset/files"
MANIFEST="$ROOT_DIR/dataset/manifest.json"

mkdir -p "$OUT_DIR"

echo "=== MediaCase Dataset Generator ==="
echo "Directorio de salida: $OUT_DIR"

# ── Genera video silencioso ──────────────────────────────────────────────────
make_video() {
  local name="$1" dur="$2" w="$3" h="$4" fmt="$5"
  local out="$OUT_DIR/${name}.${fmt}"
  [[ -f "$out" ]] && return 0
  ffmpeg -y \
    -f lavfi -i "color=c=blue:size=${w}x${h}:rate=25" \
    -f lavfi -i "sine=frequency=440:sample_rate=44100" \
    -t "$dur" \
    -c:v libx264 -preset ultrafast -crf 35 \
    -c:a aac -b:a 64k \
    "$out" -loglevel error
}

# ── Genera audio sintético ───────────────────────────────────────────────────
make_audio() {
  local name="$1" dur="$2" freq="$3" fmt="$4"
  local out="$OUT_DIR/${name}.${fmt}"
  [[ -f "$out" ]] && return 0
  ffmpeg -y \
    -f lavfi -i "sine=frequency=${freq}:sample_rate=44100" \
    -t "$dur" \
    "$out" -loglevel error
}

TOTAL=0
VIDEO_FORMATS=("mp4" "mkv" "avi" "mov")
AUDIO_FORMATS=("mp3" "wav" "aac" "flac" "ogg")

echo "Generando videos cortos (5s)..."
for i in $(seq 1 35); do
  for fmt in "${VIDEO_FORMATS[@]}"; do
    make_video "video_short_${i}_${fmt}" 5 640 360 "$fmt"
    TOTAL=$((TOTAL+1))
  done
done

echo "Generando videos medios (30s)..."
for i in $(seq 1 24); do
  for fmt in "${VIDEO_FORMATS[@]}"; do
    make_video "video_medium_${i}_${fmt}" 30 1280 720 "$fmt"
    TOTAL=$((TOTAL+1))
  done
done

echo "Generando videos largos (60s)..."
for i in $(seq 1 15); do
  for fmt in "${VIDEO_FORMATS[@]}"; do
    make_video "video_long_${i}_${fmt}" 60 1280 720 "$fmt"
    TOTAL=$((TOTAL+1))
  done
done

echo "Generando audios cortos (5s)..."
FREQS=(220 330 440 550 660)
for i in $(seq 1 22); do
  for fmt in "${AUDIO_FORMATS[@]}"; do
    freq="${FREQS[$((RANDOM % 5))]}"
    make_audio "audio_short_${i}_${fmt}" 5 "$freq" "$fmt"
    TOTAL=$((TOTAL+1))
  done
done

echo "Generando audios medios (30s)..."
for i in $(seq 1 15); do
  for fmt in "${AUDIO_FORMATS[@]}"; do
    freq="${FREQS[$((RANDOM % 5))]}"
    make_audio "audio_medium_${i}_${fmt}" 30 "$freq" "$fmt"
    TOTAL=$((TOTAL+1))
  done
done

echo "Generando audios largos (60s)..."
for i in $(seq 1 8); do
  for fmt in "${AUDIO_FORMATS[@]}"; do
    freq="${FREQS[$((RANDOM % 5))]}"
    make_audio "audio_long_${i}_${fmt}" 60 "$freq" "$fmt"
    TOTAL=$((TOTAL+1))
  done
done

echo ""
echo "Generados $TOTAL archivos en $OUT_DIR"

echo "Construyendo manifest.json..."

# Try to find Python - handle Windows where python3 may not exist
PYTHON_CMD=""
if command -v python3 &> /dev/null; then
  PYTHON_CMD="python3"
elif command -v python &> /dev/null; then
  PYTHON_CMD="python"
else
  echo "ERROR: Python no encontrado. Por favor instala Python 3 y añádelo al PATH."
  exit 1
fi

$PYTHON_CMD - <<PYEOF
import os, json, mimetypes, pathlib

files_dir = "$OUT_DIR"
manifest  = []

for fname in sorted(os.listdir(files_dir)):
    fpath = os.path.join(files_dir, fname)
    if not os.path.isfile(fpath):
        continue
    ext  = pathlib.Path(fname).suffix.lower()
    size = os.path.getsize(fpath)
    mime, _ = mimetypes.guess_type(fpath)
    kind = "video" if mime and mime.startswith("video") else "audio"
    manifest.append({
        "filename":   fname,
        "path":       f"dataset/files/{fname}",
        "type":       kind,
        "format":     ext.lstrip("."),
        "size_bytes": size,
        "mime_type":  mime or "application/octet-stream",
        "operations": ["convert","thumbnail"] if kind=="video" else ["convert","extract_audio"],
    })

with open("$MANIFEST", "w") as f:
    json.dump({"total": len(manifest), "files": manifest}, f, indent=2)

print(f"Manifest escrito: {len(manifest)} entradas → $MANIFEST")
PYEOF

echo ""
echo "=== Dataset listo ==="
echo "Total archivos: $TOTAL"
echo "Manifest: $MANIFEST"