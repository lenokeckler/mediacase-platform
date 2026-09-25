#!/usr/bin/env bash
# Descarga el material REAL del dataset (películas abiertas de Blender, NASA, Wikimedia Commons,
# Musopen, LibriVox, Prelinger), verifica el sha256 de cada descarga contra dataset/real_sources.json
# y después genera, de forma determinista, las variantes derivadas con ffmpeg y los casos límite.
#
# Todo lo que hace está descrito en el archivo de especificación (versionado); este script solo lo
# ejecuta. Es reanudable: salta lo que ya está en dataset/files con el hash correcto, retoma
# descargas cortadas (curl -C -) y no vuelve a codificar derivados que ya existen.
#
# Uso:
#   bash dataset/scripts/fetch_real.sh                  # descarga + derivados + casos límite
#   bash dataset/scripts/fetch_real.sh --pin            # (mantenimiento) anota en el spec los sha256 que falten
#   bash dataset/scripts/fetch_real.sh --force-derived  # vuelve a generar derivados y casos límite
#   bash dataset/scripts/fetch_real.sh --keep-cache     # conserva los .zip descargados en dataset/.cache
# Después: python dataset/scripts/build_manifest.py   (arma dataset/manifest.json v3)
#
# Requisitos: bash (Git Bash sirve), curl, unzip, sha256sum, python 3, ffmpeg y ffprobe en el PATH.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SPEC="$ROOT_DIR/dataset/real_sources.json"
OUT_DIR="$ROOT_DIR/dataset/files"
CACHE_DIR="$ROOT_DIR/dataset/.cache"
PIN=0
KEEP_CACHE=0
FORCE_DERIVED=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --spec)  SPEC="$2"; shift 2 ;;
    --out)   OUT_DIR="$2"; shift 2 ;;
    --cache) CACHE_DIR="$2"; shift 2 ;;
    --pin)   PIN=1; shift ;;
    --keep-cache) KEEP_CACHE=1; shift ;;
    --force-derived) FORCE_DERIVED=1; shift ;;
    *) echo "opción desconocida: $1"; exit 1 ;;
  esac
done

for tool in curl unzip sha256sum python ffmpeg ffprobe; do
  command -v "$tool" >/dev/null || { echo "falta $tool en el PATH"; exit 1; }
done
export PYTHONIOENCODING=utf-8 PYTHONUTF8=1
mkdir -p "$OUT_DIR" "$CACHE_DIR"
PINS="$CACHE_DIR/pins.tsv"
: > "$PINS"
# Si ffmpeg falla a mitad de un derivado, no dejar el archivo temporal en dataset/files.
trap 'rm -f "$OUT_DIR"/.derivando_*' EXIT
T0=$SECONDS

# Lee el spec con python y lo entrega como campos separados por NUL (los nombres llevan espacios
# y tildes; NUL es lo único que no puede aparecer en ellos).
spec_stream() {
  python - "$SPEC" "$1" <<'PY'
import json, sys
spec = json.load(open(sys.argv[1], encoding="utf-8"))
part = sys.argv[2]
out = sys.stdout
def emit(*fields):
    for f in fields:
        out.write(("" if f is None else str(f)) + "\0")
keys = {s["id"]: s["key"] for s in spec["sources"]}
def resolve(ref):
    return ref.split(":", 1)[1] if ref.startswith("derived:") else keys[ref]
if part == "ua":
    emit(spec["user_agent"])
elif part == "sources":
    for s in spec["sources"]:
        emit(s["id"], s["url"], s["key"], s.get("sha256") or "", s.get("zip_member") or "",
             s.get("file_sha256") or "", s.get("verify", "strict"))
elif part == "derived":
    for d in spec["derived"]:
        inp = d.get("input", [])
        emit(d["key"], resolve(d["from"]), len(inp), *inp, len(d["args"]), *d["args"])
elif part == "edge":
    for e in spec["edge"]:
        emit(e["key"], e["recipe"], resolve(e["from"]) if e.get("from") else "",
             e.get("fraction", ""), e.get("offset_fraction", ""), e.get("length", ""), e.get("text", ""))
PY
}

sha_of() { sha256sum "$1" | cut -d' ' -f1; }
fsize()  { stat -c %s "$1" 2>/dev/null || stat -f %z "$1"; }
mb()     { local t=$(( $1 * 10 / 1048576 )); echo "$(( t / 10 )).$(( t % 10 ))"; }

UA=""
while IFS= read -r -d '' f; do UA=$f; done < <(spec_stream ua)

# fetch_url <url> <destino> : descarga reanudable (si el destino parcial existe, continúa)
fetch_url() {
  local url=$1 dest=$2
  if ! curl -L --fail --silent --show-error --retry 3 --retry-delay 3 -A "$UA" -C - -o "$dest" "$url"; then
    echo "   ↻ reintento desde cero: $url"
    rm -f "$dest"
    curl -L --fail --silent --show-error --retry 3 --retry-delay 3 -A "$UA" -o "$dest" "$url"
  fi
}

# check_hash <archivo> <esperado> <verify> <id> <campo> : 0 si coincide (o si se está fijando)
check_hash() {
  local file=$1 want=$2 verify=$3 id=$4 field=$5 got
  got=$(sha_of "$file")
  if [[ -z "$want" ]]; then
    if (( PIN )); then printf '%s\t%s\t%s\n' "$id" "$field" "$got" >> "$PINS"; return 0; fi
    echo "ERROR: $id no tiene $field en el spec (corra con --pin para fijarlo)"; return 1
  fi
  [[ "$got" == "$want" ]] && return 0
  if [[ "$verify" == warn ]]; then
    echo "   ⚠ $id: $field distinto ($got); la fuente es un render de Wikimedia que puede regenerarse. Se conserva."
    return 0
  fi
  echo "ERROR: $id: $field esperado $want, obtenido $got"; return 1
}

# ── 1. Fuentes reales ─────────────────────────────────────────────────────────────────────
echo "→ fuentes reales ($SPEC)"
n=0; downloaded=0; skipped=0; bytes_dl=0
while IFS= read -r -d '' id && IFS= read -r -d '' url && IFS= read -r -d '' key \
   && IFS= read -r -d '' sha && IFS= read -r -d '' member && IFS= read -r -d '' fsha \
   && IFS= read -r -d '' verify; do
  n=$(( n + 1 ))
  final="$OUT_DIR/$key"
  want_final=$sha; [[ -n "$member" ]] && want_final=$fsha
  if [[ -s "$final" && -n "$want_final" ]] && [[ "$(sha_of "$final")" == "$want_final" ]]; then
    skipped=$(( skipped + 1 )); continue
  fi
  if [[ -s "$final" && "$verify" == warn && -n "$want_final" ]]; then
    skipped=$(( skipped + 1 )); continue      # render regenerable: ya existe, no se vuelve a bajar
  fi
  t=$SECONDS
  if [[ -n "$member" ]]; then
    zip="$CACHE_DIR/$id.zip"
    if ! [[ -s "$zip" && -n "$sha" && "$(sha_of "$zip")" == "$sha" ]]; then
      fetch_url "$url" "$zip"
      bytes_dl=$(( bytes_dl + $(fsize "$zip") ))
    fi
    check_hash "$zip" "$sha" "$verify" "$id" sha256
    unzip -p "$zip" "$member" > "$final.part"
    check_hash "$final.part" "$fsha" "$verify" "$id" file_sha256
    mv -f "$final.part" "$final"
    (( KEEP_CACHE )) || rm -f "$zip"
  else
    part="$CACHE_DIR/$id.part"
    fetch_url "$url" "$part"
    bytes_dl=$(( bytes_dl + $(fsize "$part") ))
    check_hash "$part" "$sha" "$verify" "$id" sha256
    mv -f "$part" "$final"
  fi
  downloaded=$(( downloaded + 1 ))
  printf '   [%2d] %-44s %8s MB  %3ss\n' "$n" "$key" "$(mb "$(fsize "$final")")" "$(( SECONDS - t ))"
done < <(spec_stream sources)
echo "   $n fuentes: $downloaded descargadas ($(mb $bytes_dl) MB), $skipped ya estaban"

if (( PIN )) && [[ -s "$PINS" ]]; then
  python - "$SPEC" "$PINS" <<'PY'
import json, sys
spec_path, pins = sys.argv[1], sys.argv[2]
spec = json.load(open(spec_path, encoding="utf-8"))
by_id = {s["id"]: s for s in spec["sources"]}
n = 0
for line in open(pins, encoding="utf-8"):
    sid, field, h = line.rstrip("\n").split("\t")
    if not by_id[sid].get(field):
        by_id[sid][field] = h; n += 1
with open(spec_path, "w", encoding="utf-8", newline="\n") as f:
    json.dump(spec, f, ensure_ascii=False, indent=2); f.write("\n")
print(f"   fijados {n} hashes en {spec_path}")
PY
fi

# ── 2. Variantes derivadas ────────────────────────────────────────────────────────────────
# -fflags +bitexact y -map_metadata -1: sin versión del codificador ni fechas en el archivo, para
# que dos corridas con la misma versión de ffmpeg den el mismo resultado.
echo "→ variantes derivadas (ffmpeg $(ffmpeg -version | head -1 | cut -d' ' -f3))"
nd=0
while IFS= read -r -d '' key && IFS= read -r -d '' from && IFS= read -r -d '' nin; do
  inp=(); for (( i = 0; i < nin; i++ )); do IFS= read -r -d '' a; inp+=("$a"); done
  IFS= read -r -d '' nargs
  args=(); for (( i = 0; i < nargs; i++ )); do IFS= read -r -d '' a; args+=("$a"); done
  nd=$(( nd + 1 ))
  out="$OUT_DIR/$key"
  if [[ -s "$out" ]] && (( ! FORCE_DERIVED )); then continue; fi
  [[ -s "$OUT_DIR/$from" ]] || { echo "ERROR: falta la fuente $from de $key"; exit 1; }
  ext="${key##*.}"
  tmp="$OUT_DIR/.derivando_$nd.$ext"
  t=$SECONDS
  ffmpeg -nostdin -y -loglevel error "${inp[@]}" -i "$OUT_DIR/$from" -map_metadata -1 \
    -fflags +bitexact -flags:v +bitexact -flags:a +bitexact "${args[@]}" "$tmp" </dev/null
  mv -f "$tmp" "$out"
  printf '   [%2d] %-44s %8s MB  %3ss  ← %s\n' "$nd" "$key" "$(mb "$(fsize "$out")")" "$(( SECONDS - t ))" "$from"
done < <(spec_stream derived)
echo "   $nd derivados"

# ── 3. Casos límite (se regeneran siempre: son copias o recortes baratos) ─────────────────
echo "→ casos límite"
ne=0
while IFS= read -r -d '' key && IFS= read -r -d '' recipe && IFS= read -r -d '' from \
   && IFS= read -r -d '' frac && IFS= read -r -d '' ofrac && IFS= read -r -d '' len \
   && IFS= read -r -d '' text; do
  ne=$(( ne + 1 ))
  out="$OUT_DIR/$key"
  [[ -z "$from" || -s "$OUT_DIR/$from" ]] || { echo "ERROR: falta la fuente $from de $key"; exit 1; }
  case "$recipe" in
    empty)    : > "$out" ;;
    text)     printf '%s' "$text" > "$out" ;;
    copy)     cp -f "$OUT_DIR/$from" "$out" ;;
    truncate) size=$(fsize "$OUT_DIR/$from")
              keep=$(python -c "print(int($size * $frac))")
              head -c "$keep" "$OUT_DIR/$from" > "$out" ;;
    corrupt)  cp -f "$OUT_DIR/$from" "$out"
              size=$(fsize "$out")
              off=$(python -c "print(int($size * $ofrac))")
              dd if=/dev/zero of="$out" bs="$len" count=1 seek="$off" oflag=seek_bytes conv=notrunc status=none ;;
    *) echo "ERROR: receta desconocida $recipe ($key)"; exit 1 ;;
  esac
  printf '   [%d] %-44s %10s bytes  (%s%s)\n' "$ne" "$key" "$(fsize "$out")" "$recipe" "${from:+ de $from}"
done < <(spec_stream edge)

(( KEEP_CACHE )) || rm -f "$PINS"
echo
echo "Listo en $(( SECONDS - T0 )) s · archivos en $OUT_DIR · siguiente paso: python dataset/scripts/build_manifest.py"
