#!/usr/bin/env python3
"""Coupling tuning protocols (ADR-0010, ADR-0011, ADR-0013): run `nib evaluate` per config.

Usage: scripts/tune-coupling.py WORKDIR GRID tune|validate [CONFIG ...]

GRID is adr-0010, adr-0011 or adr-0013 (the adr-0011 grid under ADR-0012); it fixes the repositories and the configuration
grid. WORKDIR holds a `nib` binary and full clones named after the
repositories, each indexed and with the shared .nibignore
(docs/research/file-coupling-benchmarks.md). CONFIG restricts the run, e.g.
s1/f50/recent90 (support/max-files/rank). Prints precision@5 per repository,
best mean first, and writes WORKDIR/<grid>-<split>.json.
"""
import itertools
import json
import re
import subprocess
import sys

SOURCES = {
    "blog": ["--prefix", "src/", "--ext", ".ts,.js,.astro"],
    "fzf": ["--ext", ".go"],
    "astro": ["--prefix", "packages/astro/src/", "--ext", ".ts,.js,.astro"],
    "flask": ["--prefix", "src/", "--ext", ".py"],
    "ripgrep": ["--ext", ".rs"],
    "okhttp": ["--ext", ".kt,.java"],
    "django": ["--prefix", "django/", "--ext", ".py"],
}
WINDOWS = {
    "tune": ["--offset", "400", "--window", "1600", "--stride", "3"],
    "validate": ["--window", "400", "--stride", "3"],
}
SMALL_WINDOWS = {  # the blog has 156 commits
    "tune": ["--offset", "78", "--window", "78", "--stride", "1"],
    "validate": ["--window", "78", "--stride", "1"],
}
GRIDS = {
    "adr-0010": (["blog", "fzf", "astro"], ["shared", "jaccard", "recent90", "recent365"]),
    "adr-0011": (list(SOURCES), ["shared", "recent14", "recent30", "recent90", "recent365"]),
}
GRIDS["adr-0013"] = GRIDS["adr-0011"]


def config_args(name):
    support, files, rank = re.fullmatch(r"s(\d+)/f(\d+)/(\w+)", name).groups()
    args = ["--min-support", support, "--max-files", files]
    days = re.fullmatch(r"recent(\d+)", rank)
    return args + (["--rank", "recent", "--half-life", days.group(1)] if days else ["--rank", rank])


def evaluate(workdir, repo, split, config):
    windows = SMALL_WINDOWS if repo == "blog" else WINDOWS
    command = [f"{workdir}/nib", "evaluate", "--json", *config_args(config), "--case-max-files", "50", *SOURCES[repo], *windows[split]]
    report = json.loads(subprocess.run(command, cwd=f"{workdir}/{repo}", capture_output=True, text=True, check=True).stdout)
    at5 = {p["name"]: next(m for m in p["metrics"] if m["k"] == 5) for p in report["predictors"]}
    return {"cases": len(report["cases"]), "nib": at5["nib"]["precision"], "nib_recall": at5["nib"]["mean_recall"],
            "cochange": at5["cochange"]["precision"], "frequency": at5["frequency"]["precision"]}


def main(workdir, grid, split, *only):
    repos, ranks = GRIDS[grid]
    configs = only or [f"s{s}/f{f}/{r}" for s, f, r in itertools.product([1, 2, 3], [20, 50, 100], ranks)]
    rows = []
    for config in configs:
        row = {"config": config, **{repo: evaluate(workdir, repo, split, config) for repo in repos}}
        row["mean"] = sum(row[repo]["nib"] for repo in repos) / len(repos)
        rows.append(row)
    with open(f"{workdir}/{grid}-{split}.json", "w") as out:
        json.dump(rows, out, indent=1)
    for row in sorted(rows, key=lambda r: -r["mean"]):
        print(f'{row["config"]:18} mean {row["mean"] * 100:5.1f}%  ' + "  ".join(f'{r} {row[r]["nib"] * 100:5.1f}%' for r in repos))
    base = rows[0]
    print("baselines: " + "  ".join(f'{r} co {base[r]["cochange"] * 100:.1f}% fq {base[r]["frequency"] * 100:.1f}% n={base[r]["cases"]}' for r in repos))


if __name__ == "__main__":
    main(*sys.argv[1:])
