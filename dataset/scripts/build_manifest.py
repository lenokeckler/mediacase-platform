#!/usr/bin/env python3
import argparse
import datetime as dt
import hashlib
import json
import os
import subprocess
import sys

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
MB = 1024 * 1024

REAL_TIER_CUTS = (10 * MB, 100 * MB)

EXT_TYPE = {
    **dict.fromkeys(["mp4", "mkv", "avi", "mov", "webm", "m4v", "flv", "wmv", "ts", "mts", "3gp", "mpg", "mpeg"], "video"),
    **dict.fromkeys(["mp3", "wav", "flac", "aac", "ogg", "m4a", "opus", "wma", "aiff", "aif", "dsf", "dff"], "audio"),
    **dict.fromkeys(["jpg", "jpeg", "png", "gif", "webp", "bmp", "tif", "tiff"], "image"),
}
USERS = ("leno", "jennifer", "jonathan")
SESSIONS_BY_TYPE = {"video": (1, 3, 4), "audio": (2, 3, 4), "image": (3, 4)}
BATCH = {"real": "lote-2026-09-20", "derived": "lote-2026-09-21", "edge": "lote-2026-09-22"}
SYNTHETIC_ORIGIN = "generate_dataset.sh (ffmpeg, semilla 42)"

def stable(key, salt, n):
    return int(hashlib.sha1(f"{salt}:{key}".encode("utf-8")).hexdigest(), 16) % n

def real_tier(size):
    if size < REAL_TIER_CUTS[0]:
        return "light"
    return "medium" if size < REAL_TIER_CUTS[1] else "heavy"

def probe_duration(path):
    try:
        out = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path],
                             capture_output=True, text=True, timeout=120)
        return int(float(out.stdout.strip().splitlines()[0]))
    except (ValueError, IndexError, subprocess.SubprocessError):
        return 0

def grouping(key, typ, event, kind):
    if kind == "edge":
        session = f"{event}-s3"
    else:
        opts = SESSIONS_BY_TYPE[typ]
        session = f"{event}-s{opts[stable(key, 'session', len(opts))]}"
    return {"event": event, "session": session, "batch": BATCH[kind], "user": USERS[stable(key, 'user', len(USERS))]}

def describe(files_dir, key, typ, fmt, kind, event):
    path = os.path.join(files_dir, key)
    if not os.path.isfile(path):
        sys.exit(f"falta {path}: corra primero bash dataset/scripts/fetch_real.sh")
    size = os.path.getsize(path)
    dur = probe_duration(path) if typ != "image" and size > 0 else 0
    e = {"filename": key, "key": key, "type": typ, "format": fmt, "size_bytes": size, "duration_s": dur,
         "tier": real_tier(size)}
    e.update(grouping(key, typ, event, kind))
    return e

def ext_of(key):
    return key.rsplit(".", 1)[-1].lower()

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--synthetic", default=os.path.join(ROOT, "dataset", "manifest.synthetic.json"))
    ap.add_argument("--drop", default=os.path.join(ROOT, "dataset", "scripts", "dropped_synthetic.txt"))
    ap.add_argument("--spec", default=os.path.join(ROOT, "dataset", "real_sources.json"))
    ap.add_argument("--cases", default=os.path.join(ROOT, "dataset", "test_cases.json"))
    ap.add_argument("--files", default=os.path.join(ROOT, "dataset", "files"))
    ap.add_argument("--out", default=os.path.join(ROOT, "dataset", "manifest.json"))
    a = ap.parse_args()

    syn = json.load(open(a.synthetic, encoding="utf-8"))
    drop = {l.strip() for l in open(a.drop, encoding="utf-8") if l.strip() and not l.startswith("#")}
    spec = json.load(open(a.spec, encoding="utf-8"))

    files = []
    unknown_drop = drop - {f["key"] for f in syn["files"]}
    if unknown_drop:
        sys.exit(f"dropped_synthetic.txt nombra archivos que no están en el manifest sintético: {sorted(unknown_drop)[:5]}")
    for f in syn["files"]:
        if f["key"] in drop:
            continue
        f = dict(f)
        f.update(source="synthetic", origin=SYNTHETIC_ORIGIN, license="propio (sintético)",
                 author="equipo MediaCase", url="", note="")
        files.append(f)

    by_id = {s["id"]: s for s in spec["sources"]}
    by_key = {}
    for s in spec["sources"]:
        typ = EXT_TYPE[ext_of(s["key"])]
        e = describe(a.files, s["key"], typ, ext_of(s["key"]), "real", s["event"])
        e.update(source="real", origin=s["origin"], license=s["license"], license_url=s["license_url"],
                 author=s["author"], url=s["url"], page=s["page"], title=s["title"], note=s.get("note", ""))
        files.append(e)
        by_key[s["key"]] = s
    for d in spec["derived"]:
        s = by_id[d["from"]]
        typ = EXT_TYPE[ext_of(d["key"])]
        e = describe(a.files, d["key"], typ, ext_of(d["key"]), "derived", s["event"])
        e.update(source="real", origin=s["origin"] + " (variante derivada con ffmpeg)", license=s["license"],
                 license_url=s["license_url"], author=s["author"], url=s["url"], page=s["page"],
                 title=s["title"], derived_from=s["key"], note=d.get("note", ""))
        files.append(e)
        by_key[d["key"]] = s
    for g in spec["edge"]:
        ref = g.get("from")
        parent = None
        if ref:
            parent_key = ref.split(":", 1)[1] if ref.startswith("derived:") else by_id[ref]["key"]
            parent = by_key[parent_key]
        e = describe(a.files, g["key"], g["type"], g["format"], "edge", "pruebas")
        e.update(source="edge", origin="fetch_real.sh (caso límite, receta «%s»)" % g["recipe"],
                 license=parent["license"] if parent else "propio",
                 author=parent["author"] if parent else "equipo MediaCase",
                 url=parent["url"] if parent else "", note=g["note"], expected=g["expected"],
                 extension=g.get("extension", ext_of(g["key"])))
        if parent:
            e["derived_from"] = parent_key
            e["license_url"] = parent["license_url"]
        files.append(e)

    keys = [f["key"] for f in files]
    dup = {k for k in keys if keys.count(k) > 1}
    if dup:
        sys.exit(f"claves repetidas: {sorted(dup)}")

    cases = json.load(open(a.cases, encoding="utf-8"))["test_cases"] if os.path.exists(a.cases) else []
    counts = {s: sum(1 for f in files if f["source"] == s) for s in ("synthetic", "real", "edge")}
    manifest = {
        "version": 3,
        "generated_at": dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "total": len(files),
        "composition": {
            "synthetic": {"files": counts["synthetic"], "manifest": "dataset/manifest.synthetic.json",
                          "profile": syn.get("profile"), "seed": syn.get("seed"),
                          "dropped": len(drop), "dropped_list": "dataset/scripts/dropped_synthetic.txt"},
            "real": {"files": counts["real"], "spec": "dataset/real_sources.json",
                     "downloads": len(spec["sources"]), "derived": len(spec["derived"])},
            "edge": {"files": counts["edge"], "spec": "dataset/real_sources.json"},
            "real_tier_rule": "real y edge: < 10 MB light, 10-100 MB medium, >= 100 MB heavy (nivel más cercano)",
        },
        "files": files,
        "test_cases": cases,
    }
    with open(a.out, "w", encoding="utf-8", newline="\n") as f:
        f.write("{\n")
        for k in ("version", "generated_at", "total", "composition"):
            f.write(f'  "{k}": {json.dumps(manifest[k], ensure_ascii=False)},\n')
        f.write('  "files": [\n')
        f.write(",\n".join("    " + json.dumps(x, ensure_ascii=False, separators=(",", ":")) for x in files))
        f.write("\n  ],\n")
        f.write('  "test_cases": ' + json.dumps(cases, ensure_ascii=False, indent=2).replace("\n", "\n  ") + "\n}\n")
    vol = sum(f["size_bytes"] for f in files)
    print(f"manifest v3: {len(files)} archivos ({counts['synthetic']} sintéticos, {counts['real']} reales, "
          f"{counts['edge']} límite), {vol / MB / 1024:.2f} GB, {len(cases)} casos de prueba → {a.out}")

if __name__ == "__main__":
    main()
