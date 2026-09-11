#!/usr/bin/env bash
# Genera el dataset multimedia de prueba (consigna §"Dataset"): 400-600 archivos con audio,
# video e imágenes, formatos variados, TRES NIVELES POR TAMAÑO REAL (liviano < 5 MB,
# mediano 20-50 MB, pesado 150-400 MB) y metadatos de agrupación (evento, sesión, lote,
# usuario) en dataset/manifest.json para que cmd/ingest arme casos automáticamente.
#
# Cómo se logra el tamaño: cada archivo recibe un tamaño objetivo dentro de su nivel y la
# duración se calcula a partir del bitrate del códec; después de codificar se mide con `stat`
# y, si se sale del rango, se reintenta una vez reescalando el bitrate (video) o la duración
# (audio). Si aun así no entra, el script aborta. Ningún archivo queda fuera de rango.
#
# Reproducible: usa un generador congruencial propio (no $RANDOM, que bash ≥ 5.1 re-siembra
# en cada subshell), así que con la misma semilla salen los mismos nombres, formatos,
# duraciones y metadatos en cualquier máquina. Reanudable: salta los archivos que ya existen
# y están dentro de rango.
#
# Por construcción hay casos homogéneos y heterogéneos al agrupar por sesión: la sesión s1 de
# cada evento solo tiene video, la s2 solo audio, y s3/s4 mezclan los tres tipos.
#
# Uso:
#   bash dataset/scripts/generate_dataset.sh                 # perfil full  (492 archivos, ~14 GB, ~1 h)
#   bash dataset/scripts/generate_dataset.sh --profile quick # perfil quick (38 archivos, ~0.6 GB, ~3 min)
#   bash dataset/scripts/generate_dataset.sh --seed 7 --out /otra/carpeta --manifest /otra/manifest.json
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT_DIR="$ROOT_DIR/dataset/files"
MANIFEST="$ROOT_DIR/dataset/manifest.json"
PROFILE="full"
SEED=42

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --seed)    SEED="$2"; shift 2 ;;
    --out)     OUT_DIR="$2"; shift 2 ;;
    --manifest) MANIFEST="$2"; shift 2 ;;
    *) echo "opción desconocida: $1"; exit 1 ;;
  esac
done

command -v ffmpeg  >/dev/null || { echo "falta ffmpeg en el PATH"; exit 1; }
command -v ffprobe >/dev/null || { echo "falta ffprobe en el PATH"; exit 1; }
mkdir -p "$OUT_DIR"

# ── Cantidades por perfil: (liviano mediano pesado) por tipo ──────────────────────────────
case "$PROFILE" in
  full)  V_LIGHT=140; V_MED=80; V_HEAVY=30;  A_LIGHT=100; A_MED=60; A_HEAVY=12;  IMG=70 ;;   # 492 archivos, ~14 GB
  quick) V_LIGHT=10;  V_MED=4;  V_HEAVY=1;   A_LIGHT=8;   A_MED=4;  A_HEAVY=1;   IMG=10 ;;   # 38 archivos, ~0.6 GB
  *) echo "perfil desconocido: $PROFILE (full|quick)"; exit 1 ;;
esac

MB=1048576
VIDEO_FORMATS=(mp4 mkv avi mov webm)
AUDIO_FORMATS=(mp3 wav flac aac ogg)
HEAVY_AUDIO_FORMATS=(wav flac)         # comprimidos a 150+ MB serían horas de audio
IMAGE_FORMATS=(jpg png webp)
VIDEO_SOURCES=(testsrc2 mandelbrot life cellauto smptebars)
EVENTS=(boda concierto clase entrevista partido documental)
USERS=(leno jennifer jonathan)

# ── PRNG propio (LCG de glibc) — determinista, sin subshells ──────────────────────────────
RS=$SEED
next() { RS=$(( (RS * 1103515245 + 12345) & 0x7fffffff )); R=$(( RS >> 8 )); }
rnd()  { next; R=$(( $1 + R % ($2 - $1 + 1) )); }          # entero en [a, b] → $R
pick() { local -n arr=$1; next; R=${arr[$(( R % ${#arr[@]} ))]}; }   # elemento → $R

# ── Rangos por nivel (bytes) y objetivo aleatorio dentro de una franja segura ─────────────
tier_range() {   # → LO HI
  case "$1" in
    light)  LO=0;             HI=$(( 5 * MB )) ;;
    medium) LO=$(( 20 * MB ));  HI=$(( 50 * MB )) ;;
    heavy)  LO=$(( 150 * MB )); HI=$(( 400 * MB )) ;;
  esac
}
tier_target() {  # → R (bytes objetivo, lejos de los bordes para absorber ±10 % del códec)
  case "$1" in
    light)  rnd 1536 4096 ;;         # 1.5 – 4 MB
    medium) rnd 25600 46080 ;;       # 25 – 45 MB
    heavy)  rnd 174080 266240 ;;     # 170 – 260 MB
  esac
  R=$(( R * 1024 ))
}
in_range() { tier_range "$2"; (( $1 >= LO && $1 <= HI )); }
fsize()    { stat -c %s "$1" 2>/dev/null || stat -f %z "$1"; }
mb()       { local t=$(( $1 * 10 / MB )); echo "$(( t / 10 )).$(( t % 10 ))"; }

manifest_entries=()
count=0
total=$(( V_LIGHT + V_MED + V_HEAVY + A_LIGHT + A_MED + A_HEAVY + IMG ))

# ── Video ─────────────────────────────────────────────────────────────────────────────────
# encode_video <out> <fmt> <dur_s> <w> <h> <kbps> <fuente>
encode_video() {
  local out=$1 fmt=$2 dur=$3 w=$4 h=$5 kbps=$6 src=$7
  local vargs=() acodec="aac"
  case "$fmt" in
    # libvpx dobla el tamaño si se le pasa -minrate/-maxrate; solo con -b:v respeta la tasa
    webm) vargs=(-c:v libvpx -deadline realtime -cpu-used 8 -b:v "${kbps}k"); acodec="libvorbis" ;;
    # x264 en CBR estricto (nal-hrd=cbr rellena si el contenido es simple): tamaño = bitrate × duración
    *)    vargs=(-c:v libx264 -preset ultrafast -x264-params nal-hrd=cbr:force-cfr=1
                 -b:v "${kbps}k" -minrate "${kbps}k" -maxrate "${kbps}k" -bufsize "$(( kbps * 2 ))k")
          [[ "$fmt" == avi ]] && acodec="mp3" ;;
  esac
  ffmpeg -y -loglevel error -threads 0 \
    -f lavfi -i "$src=size=${w}x${h}:rate=25" \
    -f lavfi -i "anoisesrc=color=pink:amplitude=0.3:sample_rate=44100,volume=0.5" \
    -t "$dur" "${vargs[@]}" -c:a "$acodec" -b:a 128k -pix_fmt yuv420p -shortest "$out"
}

# make_video <nombre> <fmt> <tier> <fuente> <bytes_objetivo>
make_video() {
  local name=$1 fmt=$2 tier=$3 src=$4 target=$5
  local out="$OUT_DIR/$name.$fmt" w h kbps
  case "$tier" in
    light)  w=640;  h=360;  kbps=1500 ;;
    medium) w=1280; h=720;  kbps=4000 ;;
    heavy)  w=1920; h=1080; kbps=6000 ;;
  esac
  # mandelbrot se vuelve más lento conforme avanza el zoom: solo para livianos. VP8 solo
  # controla bien la tasa con fuentes de complejidad estable: en webm siempre testsrc2.
  if [[ "$tier" != light && "$src" == mandelbrot ]]; then src=testsrc2; fi
  if [[ "$fmt" == webm ]]; then src=testsrc2; fi
  local dur=$(( target / ((kbps + 128) * 125) )); (( dur < 3 )) && dur=3
  if [[ -s "$out" ]] && in_range "$(fsize "$out")" "$tier"; then
    register "$name.$fmt" video "$fmt" "$tier" "(ya existía)"; return
  fi
  local t0=$SECONDS
  encode_video "$out" "$fmt" "$dur" "$w" "$h" "$kbps" "$src"
  local size; size=$(fsize "$out")
  if ! in_range "$size" "$tier"; then
    local kbps2=$(( kbps * target / size )); (( kbps2 < 100 )) && kbps2=100
    echo "   ↻ $name.$fmt: $(mb "$size") MB fuera de rango $tier → reintento a ${kbps2} kbps"
    encode_video "$out" "$fmt" "$dur" "$w" "$h" "$kbps2" "$src"
    size=$(fsize "$out")
    in_range "$size" "$tier" || { echo "ABORTO: $name.$fmt sigue fuera de rango ($(mb "$size") MB)"; exit 1; }
  fi
  register "$name.$fmt" video "$fmt" "$tier" "$src ${dur}s $(( SECONDS - t0 ))s"
}

# ── Audio ─────────────────────────────────────────────────────────────────────────────────
# bytes/segundo medidos por formato (estéreo 44.1 kHz): define la duración para un tamaño dado
audio_bps() {
  case "$1" in
    wav) echo 176400 ;; flac) echo 92700 ;; mp3) echo 24000 ;; aac) echo 20300 ;; ogg) echo 45800 ;;
  esac
}
# encode_audio <out> <fmt> <dur_s> <freq_hz>
encode_audio() {
  local out=$1 fmt=$2 dur=$3 freq=$4 aargs=()
  case "$fmt" in
    mp3)  aargs=(-c:a libmp3lame -b:a 192k) ;;
    aac)  aargs=(-c:a aac -b:a 160k) ;;
    ogg)  aargs=(-c:a libvorbis -q:a 10) ;;   # ~370 kbps: menos minutos de audio por MB
    flac) aargs=(-c:a flac) ;;
    wav)  aargs=(-c:a pcm_s16le) ;;
  esac
  ffmpeg -y -loglevel error \
    -f lavfi -i "sine=frequency=${freq}:sample_rate=44100" \
    -f lavfi -i "anoisesrc=color=brown:amplitude=0.2:sample_rate=44100" \
    -filter_complex "[0:a][1:a]amix=inputs=2,tremolo=f=0.5:d=0.4" \
    -t "$dur" -ac 2 "${aargs[@]}" "$out"
}

# make_audio <nombre> <fmt> <tier> <freq> <bytes_objetivo>
make_audio() {
  local name=$1 fmt=$2 tier=$3 freq=$4 target=$5
  local out="$OUT_DIR/$name.$fmt"
  local dur=$(( target / $(audio_bps "$fmt") )); (( dur < 3 )) && dur=3
  if [[ -s "$out" ]] && in_range "$(fsize "$out")" "$tier"; then
    register "$name.$fmt" audio "$fmt" "$tier" "(ya existía)"; return
  fi
  local t0=$SECONDS
  encode_audio "$out" "$fmt" "$dur" "$freq"
  local size; size=$(fsize "$out")
  if ! in_range "$size" "$tier"; then
    local dur2=$(( dur * target / size )); (( dur2 < 3 )) && dur2=3
    echo "   ↻ $name.$fmt: $(mb "$size") MB fuera de rango $tier → reintento con ${dur2}s"
    encode_audio "$out" "$fmt" "$dur2" "$freq"
    size=$(fsize "$out")
    in_range "$size" "$tier" || { echo "ABORTO: $name.$fmt sigue fuera de rango ($(mb "$size") MB)"; exit 1; }
    dur=$dur2
  fi
  register "$name.$fmt" audio "$fmt" "$tier" "${freq}Hz ${dur}s $(( SECONDS - t0 ))s"
}

# ── Imágenes (siempre livianas) ───────────────────────────────────────────────────────────
# make_image <nombre> <fmt> <fuente> <ancho> <alto>
make_image() {
  local name=$1 fmt=$2 src=$3 w=$4 h=$5
  local out="$OUT_DIR/$name.$fmt"
  if [[ -s "$out" ]] && in_range "$(fsize "$out")" light; then
    register "$name.$fmt" image "$fmt" light "(ya existía)"; return
  fi
  ffmpeg -y -loglevel error -f lavfi -i "$src=size=${w}x${h}" -frames:v 1 -q:v 3 "$out"
  in_range "$(fsize "$out")" light || { echo "ABORTO: $name.$fmt supera 5 MB"; exit 1; }
  register "$name.$fmt" image "$fmt" light "$src ${w}x${h}"
}

# ── Manifest ──────────────────────────────────────────────────────────────────────────────
# register <archivo> <tipo> <formato> <tier> <nota>  → mide el archivo y asigna metadatos
register() {
  local file=$1 type=$2 fmt=$3 tier=$4 note=$5
  local size; size=$(fsize "$OUT_DIR/$file")
  local dur=0
  if [[ "$type" != image ]]; then
    dur=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$OUT_DIR/$file"); dur=${dur%.*}
  fi
  pick EVENTS; local ev=$R
  # s1 = solo video, s2 = solo audio, s3/s4 = mezcla → casos homogéneos y heterogéneos garantizados
  local sess_choices
  case "$type" in video) sess_choices=(1 3 4) ;; audio) sess_choices=(2 3 4) ;; *) sess_choices=(3 4) ;; esac
  pick sess_choices; local sess="$ev-s$R"
  rnd 1 6; local batch; batch=$(printf 'lote-2026-09-%02d' "$R")
  pick USERS; local user=$R
  manifest_entries+=("{\"filename\":\"$file\",\"key\":\"$file\",\"type\":\"$type\",\"format\":\"$fmt\",\"size_bytes\":$size,\"duration_s\":$dur,\"tier\":\"$tier\",\"event\":\"$ev\",\"session\":\"$sess\",\"batch\":\"$batch\",\"user\":\"$user\"}")
  count=$(( count + 1 ))
  printf '   [%3d/%3d] %-6s %-6s %-24s %7s MB  %s\n' "$count" "$total" "$type" "$tier" "$file" "$(mb "$size")" "$note"
}

echo "Perfil: $PROFILE · semilla: $SEED · salida: $OUT_DIR · $total archivos"
echo "→ videos"
i=0
for tier in light medium heavy; do
  case "$tier" in light) n=$V_LIGHT ;; medium) n=$V_MED ;; heavy) n=$V_HEAVY ;; esac
  for _ in $(seq 1 "$n"); do
    i=$(( i + 1 ))
    pick VIDEO_FORMATS; fmt=$R
    tier_target "$tier"; target=$R
    pick VIDEO_SOURCES; src=$R
    make_video "video_${tier}_$i" "$fmt" "$tier" "$src" "$target"
  done
done
echo "→ audios"
i=0
for tier in light medium heavy; do
  case "$tier" in light) n=$A_LIGHT ;; medium) n=$A_MED ;; heavy) n=$A_HEAVY ;; esac
  for _ in $(seq 1 "$n"); do
    i=$(( i + 1 ))
    if [[ "$tier" == heavy ]]; then pick HEAVY_AUDIO_FORMATS; else pick AUDIO_FORMATS; fi; fmt=$R
    tier_target "$tier"; target=$R
    rnd 220 880; freq=$R
    make_audio "audio_${tier}_$i" "$fmt" "$tier" "$freq" "$target"
  done
done
echo "→ imágenes"
for i in $(seq 1 "$IMG"); do
  pick IMAGE_FORMATS; fmt=$R
  pick VIDEO_SOURCES; src=$R
  rnd 800 1920; w=$R
  rnd 600 1080; h=$R
  make_image "image_$i" "$fmt" "$src" "$w" "$h"
done

{
  echo "{"
  echo "  \"version\": 2,"
  echo "  \"profile\": \"$PROFILE\","
  echo "  \"seed\": $SEED,"
  echo "  \"generated_at\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
  echo "  \"total\": ${#manifest_entries[@]},"
  echo "  \"files\": ["
  for k in "${!manifest_entries[@]}"; do
    sep=","; [[ $k -eq $(( ${#manifest_entries[@]} - 1 )) ]] && sep=""
    echo "    ${manifest_entries[$k]}$sep"
  done
  echo "  ]"
  echo "}"
} > "$MANIFEST"

# Archivos sueltos de corridas anteriores (otra semilla u otro perfil) no pertenecen al dataset
extra=0
for f in "$OUT_DIR"/*; do
  [[ -f "$f" ]] || continue
  grep -q "\"filename\":\"$(basename "$f")\"" "$MANIFEST" || { rm -f "$f"; extra=$(( extra + 1 )); }
done
(( extra > 0 )) && echo "eliminados $extra archivos que no están en el manifest"

echo
echo "Generados ${#manifest_entries[@]} archivos en $OUT_DIR ($(du -sh "$OUT_DIR" | cut -f1)) · manifest: $MANIFEST"
PYTHONIOENCODING=utf-8 python "$SCRIPT_DIR/check_manifest.py" "$MANIFEST" --profile "$PROFILE"
