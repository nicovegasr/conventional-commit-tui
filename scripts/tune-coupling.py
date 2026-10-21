#!/usr/bin/env python3
"""ADR-0010 tuning protocol: run `nib evaluate` over the configuration grid.

Usage: scripts/tune-coupling.py WORKDIR tune|validate [CONFIG ...]

WORKDIR holds a `nib` binary and full clones named blog, fzf and astro, each
indexed and with the shared .nibignore (docs/research/file-coupling-benchmarks.md).
CONFIG restricts the grid, e.g. s1/f50/recent90. Prints precision@5 per
repository, best mean first, and writes WORKDIR/grid-<split>.json.
"""
import itertools
import json
import subprocess
import sys

S = sys.argv[1]
repos = {
    "blog":  ["--prefix", "src/", "--ext", ".ts,.js,.astro"],
    "fzf":   ["--ext", ".go"],
    "astro": ["--prefix", "packages/astro/src/", "--ext", ".ts,.js,.astro"],
}
split = sys.argv[2]
windows = {
    ("blog", "tune"): ["--offset", "78", "--window", "78", "--stride", "1"],
    ("blog", "validate"): ["--window", "78", "--stride", "1"],
}
for r in ("fzf", "astro"):
    windows[(r, "tune")] = ["--offset", "400", "--window", "1600", "--stride", "3"]
    windows[(r, "validate")] = ["--window", "400", "--stride", "3"]
ranks = [("shared", []), ("jaccard", []), ("recent90", ["--rank", "recent", "--half-life", "90"]), ("recent365", ["--rank", "recent", "--half-life", "365"])]
ranks = [(n, a or ["--rank", n]) for n, a in ranks]
configs = sys.argv[3:]
rows = []
for support, files, (rank, rargs) in itertools.product([1, 2, 3], [20, 50, 100], ranks):
    name = f"s{support}/f{files}/{rank}"
    if configs and name not in configs:
        continue
    row = {"config": name}
    for repo, src in repos.items():
        out = subprocess.run([f"{S}/nib", "evaluate", "--json", "--min-support", str(support), "--max-files", str(files), *rargs, *src, *windows[(repo, split)]],
                             cwd=f"{S}/{repo}", capture_output=True, text=True, check=True).stdout
        e = json.loads(out)
        by = {p["name"]: p for p in e["predictors"]}
        k5 = lambda p: next(m for m in by[p]["metrics"] if m["k"] == 5)
        row[repo] = {"cases": len(e["cases"]), "nib": k5("nib")["precision"], "nib_recall": k5("nib")["mean_recall"],
                     "cochange": k5("cochange")["precision"], "frequency": k5("frequency")["precision"]}
    row["mean"] = sum(row[r]["nib"] for r in repos) / len(repos)
    rows.append(row)
json.dump(rows, open(f"{S}/grid-{split}.json", "w"), indent=1)
rows.sort(key=lambda r: -r["mean"])
for r in rows:
    print(f'{r["config"]:22} mean {r["mean"]*100:5.1f}%  ' + "  ".join(f'{n} {r[n]["nib"]*100:5.1f}%' for n in repos))
b = rows[0]
print("baselines:", "  ".join(f'{n} cochange {b[n]["cochange"]*100:.1f}% freq {b[n]["frequency"]*100:.1f}% cases {b[n]["cases"]}' for n in repos))
