#!/usr/bin/env python3
"""Valida dataset/manifest.json contra la consigna y resume su composición.

Uso: python dataset/scripts/check_manifest.py dataset/manifest.json [--profile full|quick] [--markdown]
--markdown imprime además las tablas de composición para pegar en docs/dataset.md.
Sale con código 1 si el perfil "full" no cumple los mínimos (>= 400 archivos, audio+video+imagen,
tres niveles de tamaño reales, >= 3 valores por criterio de agrupación).
"""
import json
import sys
from collections import Counter, defaultdict

MB = 1024 * 1024
TIER_RANGES = {  # límites por tamaño real (plan 2, Fase 4): livianos, medianos y pesados
    "light": (0, 5 * MB),
    "medium": (20 * MB, 50 * MB),
    "heavy": (150 * MB, 400 * MB),
}


def markdown_tables(files):
    """Tablas de composición (tipo × nivel, y por formato) en Markdown."""
    tiers = ("light", "medium", "heavy")
    names = {"light": "liviano (< 5 MB)", "medium": "mediano (20-50 MB)", "heavy": "pesado (150-400 MB)"}
    print("| Tipo | " + " | ".join(names[t] for t in tiers) + " | Total | Volumen |")
    print("|---|" + "---:|" * (len(tiers) + 2))
    for typ in ("video", "audio", "image"):
        row = [x for x in files if x["type"] == typ]
        cells = []
        for t in tiers:
            sub = [x for x in row if x["tier"] == t]
            cells.append(f"{len(sub)} ({sum(x['size_bytes'] for x in sub) / MB / 1024:.2f} GB)" if sub else "—")
        print(f"| {typ} | " + " | ".join(cells) + f" | {len(row)} | {sum(x['size_bytes'] for x in row) / MB / 1024:.2f} GB |")
    print(f"| **total** | " + " | ".join(str(sum(1 for x in files if x['tier'] == t)) for t in tiers)
          + f" | **{len(files)}** | **{sum(x['size_bytes'] for x in files) / MB / 1024:.2f} GB** |")
    print()
    print("| Formato | Archivos | Volumen | Duración total |")
    print("|---|---:|---:|---:|")
    for fmt, n in sorted(Counter(x["format"] for x in files).items(), key=lambda kv: (-kv[1], kv[0])):
        sub = [x for x in files if x["format"] == fmt]
        dur = sum(x["duration_s"] for x in sub)
        print(f"| {fmt} | {n} | {sum(x['size_bytes'] for x in sub) / MB:.0f} MB | {dur // 3600}h {dur % 3600 // 60:02d}m |")
    print()
    print("| Criterio | Valores | Archivos por caso | Casos homogéneos | Casos heterogéneos |")
    print("|---|---:|---:|---:|---:|")
    for k in ("event", "session", "batch", "user", "type", "tier"):
        types = defaultdict(set)
        sizes = Counter()
        for x in files:
            types[x[k]].add(x["type"])
            sizes[x[k]] += 1
        hom = sum(1 for t in types.values() if len(t) == 1)
        print(f"| {k} | {len(types)} | {min(sizes.values())}–{max(sizes.values())} | {hom} | {len(types) - hom} |")


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "dataset/manifest.json"
    profile = sys.argv[sys.argv.index("--profile") + 1] if "--profile" in sys.argv else "full"
    with open(path, encoding="utf-8") as f:
        m = json.load(f)
    files = m["files"]
    total_bytes = sum(x["size_bytes"] for x in files)

    by_type = Counter(x["type"] for x in files)
    by_fmt = Counter(x["format"] for x in files)
    by_tier = Counter(x["tier"] for x in files)
    size_by_tier = defaultdict(list)
    for x in files:
        size_by_tier[x["tier"]].append(x["size_bytes"])
    groups = {k: Counter(x[k] for x in files) for k in ("event", "session", "batch", "user")}

    print(f"archivos: {len(files)}   volumen: {total_bytes / MB / 1024:.2f} GB   perfil: {m.get('profile')}   semilla: {m.get('seed')}")
    print("por tipo   :", dict(by_type))
    print("por formato:", dict(by_fmt))
    for tier in ("light", "medium", "heavy"):
        s = size_by_tier.get(tier, [])
        if s:
            print(f"tier {tier:<6}: {len(s):3} archivos, {min(s) / MB:7.1f} – {max(s) / MB:7.1f} MB (video+audio+imagen)")
    for k, c in groups.items():
        print(f"agrupación por {k:<8}: {len(c)} valores, {min(c.values())}–{max(c.values())} archivos por grupo")

    problems = []
    if profile == "full":
        if len(files) < 400:
            problems.append(f"menos de 400 archivos ({len(files)})")
        for t in ("video", "audio", "image"):
            if by_type[t] == 0:
                problems.append(f"sin archivos de tipo {t}")
        for tier, (lo, hi) in TIER_RANGES.items():
            for t in ("video", "audio"):
                sizes = [x["size_bytes"] for x in files if x["tier"] == tier and x["type"] == t]
                if not sizes:
                    problems.append(f"sin {t} de tier {tier}")
                elif not all(lo <= s <= hi for s in sizes):
                    problems.append(f"{t} de tier {tier} fuera de rango [{lo / MB:.0f}, {hi / MB:.0f}] MB: "
                                    f"{min(sizes) / MB:.1f}–{max(sizes) / MB:.1f}")
        for k, c in groups.items():
            if len(c) < 3:
                problems.append(f"criterio {k} con menos de 3 valores")
        # por sesión debe haber casos homogéneos (un solo tipo) y heterogéneos (mezcla)
        types_by_session = defaultdict(set)
        for x in files:
            types_by_session[x["session"]].add(x["type"])
        hom = sum(1 for t in types_by_session.values() if len(t) == 1)
        het = sum(1 for t in types_by_session.values() if len(t) > 1)
        print(f"sesiones homogéneas: {hom}   heterogéneas: {het}")
        if hom == 0 or het == 0:
            problems.append("agrupar por session no produce casos homogéneos y heterogéneos a la vez")
    if "--markdown" in sys.argv:
        print()
        markdown_tables(files)
    if problems:
        print("\nNO CUMPLE:")
        for p in problems:
            print("  -", p)
        sys.exit(1)
    print("\nmanifest OK" + ("" if profile == "full" else " (perfil quick: mínimos de la consigna no exigidos)"))


if __name__ == "__main__":
    main()
