#!/usr/bin/env python3
import json
import os
import re
import sys
from collections import Counter, defaultdict

MB = 1024 * 1024
ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
TIER_RANGES = {
    "light": (0, 5 * MB),
    "medium": (20 * MB, 50 * MB),
    "heavy": (150 * MB, 400 * MB),
}
REAL_TIER_CUTS = (10 * MB, 100 * MB)
TIERS = ("light", "medium", "heavy")
TYPES = ("video", "audio", "image")
SOURCES = ("synthetic", "real", "edge")
V3_FIELDS = ("source", "origin", "license", "author", "url", "note")
ENRICH_FIELDS = {"title", "artist", "album", "date", "comment", "lyrics", "description"}

FALLBACK_RULES = {
    "ops_by_type": {
        "video": ["convert", "extract_audio", "thumbnail", "metadata", "enrich_video"],
        "audio": ["convert_audio", "thumbnail", "metadata", "enrich_audio"],
        "image": ["thumbnail", "metadata"],
    },
    "targets_by_op": {
        "convert": ["mp4", "mkv", "webm"], "extract_audio": ["mp3", "wav", "flac", "aac"],
        "convert_audio": ["flac", "mp3", "wav", "aac", "ogg"], "thumbnail": ["jpg", "png", "webp"],
        "metadata": ["json"], "enrich_audio": ["mp3", "flac", "ogg", "m4a"], "enrich_video": ["mp4", "mkv", "mov"],
    },
    "identity_excluded": ["convert", "convert_audio"],
    "aliases": {"jpeg": "jpg", "tiff": "tif", "aiff": "aif", "m4a": "aac", "mpeg": "mpg"},
    "widths": [320, 640, 1280],
}

def load_rules():
    try:
        router = open(os.path.join(ROOT, "internal", "cases", "router.go"), encoding="utf-8").read()
        job = open(os.path.join(ROOT, "internal", "models", "job.go"), encoding="utf-8").read()
        ops = dict(re.findall(r"(Op\w+)\s+Operation\s*=\s*\"(\w+)\"", job))

        def block(name):
            return re.search(r"var " + name + r" = [^{]*\{(.*?)\n\}", router, re.S).group(1)

        ops_by_type = {t.lower(): [ops[o] for o in re.findall(r"models\.(Op\w+)", body)]
                       for t, body in re.findall(r"models\.File(\w+):\s*\{([^}]*)\}", block("opsByType"))}
        targets = {ops[o]: re.findall(r"\"(\w+)\"", body)
                   for o, body in re.findall(r"models\.(Op\w+):\s*\{([^}]*)\}", block("targetsByOp"))}
        excluded = [ops[o] for o in re.findall(r"models\.(Op\w+)",
                                               re.search(r"identityExcludedOps = \[\][^{]*\{([^}]*)\}", router).group(1))]
        aliases = dict(re.findall(r"\"(\w+)\":\s*\"(\w+)\"", re.search(r"extAliases = map\[string\]string\{([^}]*)\}", router).group(1)))
        widths = [int(w) for w in re.findall(r"\d+", re.search(r"ThumbnailWidths = \[\]int\{([^}]*)\}", router).group(1))]
        if set(ops_by_type) != set(TYPES) or not targets or not widths:
            raise ValueError("forma inesperada")
        return {"ops_by_type": ops_by_type, "targets_by_op": targets, "identity_excluded": excluded,
                "aliases": aliases, "widths": widths}, "internal/cases/router.go"
    except (OSError, AttributeError, KeyError, ValueError):
        return FALLBACK_RULES, "reglas de respaldo (router.go no legible)"

def real_tier(size):
    if size < REAL_TIER_CUTS[0]:
        return "light"
    return "medium" if size < REAL_TIER_CUTS[1] else "heavy"

def gb(xs):
    return sum(x["size_bytes"] for x in xs) / MB / 1024

def markdown_tables(files, v3):
    names = {"light": "liviano", "medium": "mediano", "heavy": "pesado"}
    if v3:
        print("| Procedencia | Archivos | video | audio | imagen | Volumen |")
        print("|---|---:|---:|---:|---:|---:|")
        for s in SOURCES:
            sub = [x for x in files if x["source"] == s]
            c = Counter(x["type"] for x in sub)
            print(f"| {s} | {len(sub)} | {c['video']} | {c['audio']} | {c['image']} | {gb(sub):.2f} GB |")
        c = Counter(x["type"] for x in files)
        print(f"| **total** | **{len(files)}** | {c['video']} | {c['audio']} | {c['image']} | **{gb(files):.2f} GB** |")
        print()
    print("| Tipo | " + " | ".join(names[t] for t in TIERS) + " | Total | Volumen |")
    print("|---|" + "---:|" * (len(TIERS) + 2))
    for typ in TYPES:
        row = [x for x in files if x["type"] == typ]
        cells = []
        for t in TIERS:
            sub = [x for x in row if x["tier"] == t]
            cells.append(f"{len(sub)} ({gb(sub):.2f} GB)" if sub else "—")
        print(f"| {typ} | " + " | ".join(cells) + f" | {len(row)} | {gb(row):.2f} GB |")
    print(f"| **total** | " + " | ".join(str(sum(1 for x in files if x['tier'] == t)) for t in TIERS)
          + f" | **{len(files)}** | **{gb(files):.2f} GB** |")
    print()
    head = "| Formato | Tipo | Archivos |" + (" sintéticos | reales | límite |" if v3 else "") + " Volumen | Duración total |"
    print(head)
    print("|---|---|---:|" + ("---:|---:|---:|" if v3 else "") + "---:|---:|")
    for (typ, fmt), n in sorted(Counter((x["type"], x["format"]) for x in files).items(),
                                key=lambda kv: (TYPES.index(kv[0][0]), -kv[1], kv[0][1])):
        sub = [x for x in files if x["format"] == fmt and x["type"] == typ]
        dur = sum(x["duration_s"] for x in sub)
        src = ""
        if v3:
            cs = Counter(x["source"] for x in sub)
            src = " " + " | ".join(str(cs[s] or "—") for s in SOURCES) + " |"
        size = sum(x["size_bytes"] for x in sub) / MB
        print(f"| {fmt} | {typ} | {n} |{src} {size:.0f} MB | {dur // 3600}h {dur % 3600 // 60:02d}m |")
    print()
    print("| Criterio | Valores | Archivos por caso | Casos homogéneos | Casos heterogéneos |")
    print("|---|---:|---:|---:|---:|")
    for k in ("event", "session", "batch", "user", "type", "tier") + (("source",) if v3 else ()):
        types = defaultdict(set)
        sizes = Counter()
        for x in files:
            types[x[k]].add(x["type"])
            sizes[x[k]] += 1
        hom = sum(1 for t in types.values() if len(t) == 1)
        print(f"| {k} | {len(types)} | {min(sizes.values())}–{max(sizes.values())} | {hom} | {len(types) - hom} |")

def norm(ext, aliases):
    ext = ext.lower()
    return aliases.get(ext, ext)

def check_test_cases(cases, files, rules, problems):
    by_key = {x["key"]: x for x in files}
    ids = Counter(c.get("id") for c in cases)
    for i, n in ids.items():
        if n > 1:
            problems.append(f"test_case {i} repetido")
    for c in cases:
        cid = c.get("id", "?")
        for field in ("id", "name", "description", "kind", "files"):
            if not c.get(field):
                problems.append(f"test_case {cid}: falta {field}")
        if c.get("kind") not in ("homogeneous", "heterogeneous"):
            problems.append(f"test_case {cid}: kind inválido {c.get('kind')!r}")
        seen_types, seen_ops = set(), set()
        for f in c.get("files", []):
            key = f.get("key")
            e = by_key.get(key)
            if e is None:
                problems.append(f"test_case {cid}: la clave {key!r} no está en files")
                continue
            typ = e["type"]
            op = f.get("operation") or rules["ops_by_type"][typ][0]
            seen_types.add(typ)
            seen_ops.add(op)
            if op not in rules["ops_by_type"][typ]:
                problems.append(f"test_case {cid}: {op} no aplica a {typ} ({key})")
                continue
            target = f.get("target")
            if target:
                valid = rules["targets_by_op"].get(op, [])
                src_ext = key.rsplit(".", 1)[-1]
                if target not in valid:
                    problems.append(f"test_case {cid}: destino {target} no válido para {op} ({key}); válidos: {valid}")
                elif op in rules["identity_excluded"] and norm(target, rules["aliases"]) == norm(src_ext, rules["aliases"]):
                    problems.append(f"test_case {cid}: {key} ya está en {target} ({op} no acepta el formato de origen)")
            w = f.get("width")
            if w is not None and (op != "thumbnail" or w not in rules["widths"]):
                problems.append(f"test_case {cid}: ancho {w} inválido para {op} ({key})")
            enr = f.get("enrichment")
            if enr is not None:
                if not op.startswith("enrich_"):
                    problems.append(f"test_case {cid}: enrichment en una operación {op} ({key})")
                extra = set(enr) - ENRICH_FIELDS
                if extra:
                    problems.append(f"test_case {cid}: campos de enrichment desconocidos {sorted(extra)} ({key})")
            if e.get("source") == "edge" and f.get("operation"):
                problems.append(f"test_case {cid}: el caso límite {key} no debe fijar operación (la decide el coordinador)")
        homog = len(seen_types) == 1 and len(seen_ops) == 1
        if c.get("kind") == "homogeneous" and not homog:
            problems.append(f"test_case {cid}: dice homogeneous pero mezcla tipos {sorted(seen_types)} u operaciones {sorted(seen_ops)}")
        if c.get("kind") == "heterogeneous" and homog:
            problems.append(f"test_case {cid}: dice heterogeneous pero todos sus archivos son {seen_types} con {seen_ops}")

def main():
    path = sys.argv[1] if len(sys.argv) > 1 and not sys.argv[1].startswith("--") else "dataset/manifest.json"
    profile = sys.argv[sys.argv.index("--profile") + 1] if "--profile" in sys.argv else "full"
    with open(path, encoding="utf-8") as f:
        m = json.load(f)
    files = m["files"]
    v3 = m.get("version", 2) >= 3
    total_bytes = sum(x["size_bytes"] for x in files)

    by_type = Counter(x["type"] for x in files)
    by_fmt = Counter(x["format"] for x in files)
    size_by_tier = defaultdict(list)
    for x in files:
        size_by_tier[x["tier"]].append(x["size_bytes"])
    groups = {k: Counter(x[k] for x in files) for k in ("event", "session", "batch", "user")}

    print(f"manifest v{m.get('version')}: {len(files)} archivos   volumen: {total_bytes / MB / 1024:.2f} GB")
    if v3:
        print("por procedencia:", dict(Counter(x.get("source") for x in files)))
    print("por tipo   :", dict(by_type))
    print("por formato:", dict(sorted(by_fmt.items(), key=lambda kv: -kv[1])))
    for tier in TIERS:
        s = size_by_tier.get(tier, [])
        if s:
            print(f"tier {tier:<6}: {len(s):3} archivos, {min(s) / MB:7.2f} – {max(s) / MB:7.1f} MB")
    for k, c in groups.items():
        print(f"agrupación por {k:<8}: {len(c)} valores, {min(c.values())}–{max(c.values())} archivos por grupo")

    problems = []
    keys = Counter(x["key"] for x in files)
    problems += [f"clave repetida: {k}" for k, n in keys.items() if n > 1]
    for x in files:
        if x["type"] not in TYPES:
            problems.append(f"{x['key']}: tipo desconocido {x['type']!r}")
        if x["tier"] not in TIERS:
            problems.append(f"{x['key']}: nivel desconocido {x['tier']!r}")
        for k in ("event", "session", "batch", "user"):
            if not x.get(k):
                problems.append(f"{x['key']}: falta {k}")
        if v3:
            for k in V3_FIELDS:
                if k not in x:
                    problems.append(f"{x['key']}: falta el campo v3 {k}")
            src = x.get("source")
            if src not in SOURCES:
                problems.append(f"{x['key']}: source inválido {src!r}")
            elif src != "synthetic":
                if x["tier"] != real_tier(x["size_bytes"]):
                    problems.append(f"{x['key']}: nivel {x['tier']} no corresponde a {x['size_bytes'] / MB:.1f} MB")
                if not x.get("license") or not x.get("author"):
                    problems.append(f"{x['key']}: material {src} sin licencia o autor")
                if src == "real" and not x.get("url"):
                    problems.append(f"{x['key']}: material real sin URL de origen")
                if src == "edge" and not x.get("note"):
                    problems.append(f"{x['key']}: caso límite sin nota que explique el comportamiento esperado")

    if profile == "full":
        if not 400 <= len(files) <= 600:
            problems.append(f"fuera del rango 400-600 de la consigna ({len(files)})")
        for t in TYPES:
            if by_type[t] == 0:
                problems.append(f"sin archivos de tipo {t}")
        for tier, (lo, hi) in TIER_RANGES.items():
            for t in ("video", "audio"):
                sizes = [x["size_bytes"] for x in files if x["tier"] == tier and x["type"] == t
                         and x.get("source", "synthetic") == "synthetic"]
                if not sizes:
                    problems.append(f"sin {t} sintético de tier {tier}")
                elif not all(lo <= s <= hi for s in sizes):
                    problems.append(f"{t} sintético de tier {tier} fuera de rango [{lo / MB:.0f}, {hi / MB:.0f}] MB: "
                                    f"{min(sizes) / MB:.1f}–{max(sizes) / MB:.1f}")
        for k, c in groups.items():
            if len(c) < 3:
                problems.append(f"criterio {k} con menos de 3 valores")
        types_by_session = defaultdict(set)
        for x in files:
            types_by_session[x["session"]].add(x["type"])
        hom = sum(1 for t in types_by_session.values() if len(t) == 1)
        het = sum(1 for t in types_by_session.values() if len(t) > 1)
        print(f"sesiones homogéneas: {hom}   heterogéneas: {het}")
        if hom == 0 or het == 0:
            problems.append("agrupar por session no produce casos homogéneos y heterogéneos a la vez")

    if v3:
        cases = m.get("test_cases", [])
        rules, where = load_rules()
        print(f"test_cases: {len(cases)} ({sum(len(c.get('files', [])) for c in cases)} archivos en total; reglas de {where})")
        if profile == "full" and not 8 <= len(cases) <= 12:
            problems.append(f"se esperan 8-12 test_cases, hay {len(cases)}")
        check_test_cases(cases, files, rules, problems)
        kinds = Counter(c.get("kind") for c in cases)
        if profile == "full" and (not kinds["homogeneous"] or not kinds["heterogeneous"]):
            problems.append("test_cases sin casos homogéneos y heterogéneos a la vez")

    if "--markdown" in sys.argv:
        print()
        markdown_tables(files, v3)
    if problems:
        print("\nNO CUMPLE:")
        for p in problems:
            print("  -", p)
        sys.exit(1)
    print("\nmanifest OK" + ("" if profile == "full" else " (perfil quick: mínimos de la consigna no exigidos)"))

if __name__ == "__main__":
    main()
