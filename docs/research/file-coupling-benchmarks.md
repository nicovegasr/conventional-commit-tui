# File coupling benchmark methodology

Run `make bench` to reproduce the synthetic storage/query microbenchmarks.

Dataset: 1,000 commits, 121 distinct paths, three changes per commit. Every
commit touches `inventory.go`, one of 20 group paths and one of 100 test paths.
This deliberately makes the queried source participate in the entire history.

## Verified local run — 2026-10-20

Go 1.26.4, darwin/arm64, Apple M5. Command: `make bench`.

| Operation | Iterations | Mean time/op | Allocated bytes/op | Allocations/op |
|---|---:|---:|---:|---:|
| Rebuild 1,000 synthetic commits | 84 | 13.804 ms | 3,104,038 | 29,176 |
| Query the ubiquitous source | 236 | 4.996 ms | 1,557,330 | 27,004 |

These are `go test` benchmark means from one local run, not latency percentiles.

## Interpretation

`BenchmarkIndex1000` rebuilds a transactional bbolt index from an in-memory
source. It includes encoding, posting writes and transaction commit, but excludes
Git traversal and diff calculation. `BenchmarkCoupled1000` queries that source
with the default filters and bounded evidence. Both report allocations.

These are local microbenchmarks, not end-to-end throughput, p95 latency or a
large-repository capacity claim. The coupling query decodes each participating
commit's metadata and changes; profiling should determine whether splitting
commit payloads or changing posting representation is justified.

## Real-repository smoke — Astro — 2026-10-21

Full-history clone of `withastro/astro`, HEAD `8ea79a282e53b83c65f69c9013f5eeb5d97aadb1`.
Go 1.26.4, macOS arm64, Apple M5. The index reported 15,322 reachable commits
and 21,386 paths.

| Operation | Elapsed | Maximum resident set | Result |
|---|---:|---:|---|
| Initial full index | 18.16 s | not captured | 44 MiB allocated |
| First rebuild in place, single Git process | 18.26 s | 429 MiB | index file 104 MiB (88 MiB allocated) |
| Second rebuild in place, single Git process | 17.32 s | 435 MiB | unchanged at 104 MiB (88 MiB allocated) |
| Rebuild with 1,000-commit Git batches | 18.58 s | 201 MiB | unchanged at 104 MiB (88 MiB allocated) |
| `coupled` on `packages/astro/src/core/build/index.ts` | 0.08 s | 15 MiB | 145 eligible, 34 excluded |
| `coupled` on `packages/astro/src/actions/handler.ts` | 0.07 s | 7 MiB | 2 eligible, 2 excluded |

The first rebuild was run against the existing index file; allocated storage
increased from 44 MiB to 88 MiB. A second rebuild remained at that size, so this
sample shows a higher file high-water mark rather than repeated linear growth.
Measurements are single runs from `/usr/bin/time -l`, without cache control or
latency percentiles. They are a real-repository smoke test, not a capacity
claim.

### Isolating the memory peak

I ran the Git traversal separately, redirecting output to `/dev/null`, to
distinguish Git's diff work from Nib's parser and bbolt transaction. The
full-history `git log --numstat --no-renames` took 17.57 s and reached 434 MiB
maximum RSS. The same metadata-only history walk, without diff output, took
0.17 s and reached 39 MiB. Limiting numstat to the latest 1,000 commits took
2.22 s / 200 MiB; the latest 100 took 0.38 s / 87 MiB. Rename detection did not
materially change the full-history RSS.

A prototype enumerated OIDs with `git rev-list --reverse --topo-order`, then
processed them through 16 `git log --no-walk=unsorted --stdin` batches of 1,000.
The Git-only run took 19.29 s / 201 MiB. After implementing that approach,
`nib index --rebuild` took 18.58 s / 201 MiB: about 53% less peak RSS than the
single-process run, at the cost of about one second in these individual runs.
This points to Git's full-history diff/line-count traversal as the memory source,
rather than accumulated Go-side commit objects. It is a comparison of single
runs, not a controlled profile; repeat with controlled cache conditions and
another repository size before treating the trade-off as general.

For `core/build/index.ts`, the unfiltered ranking included `pnpm-lock.yaml` and
`packages/astro/package.json`. A temporary root `.nibignore` containing those
two basenames removed them from the results while preserving the 145-commit
denominator and the `static-build.ts` relationship (51 shared commits, 35.2%).
This confirms query-time filtering works without rebuilding the index.

Before a scale claim, measure real repositories with recorded commit/path/change
counts, Git/Go versions, CPU, cache conditions, index size, peak RSS, initial and
incremental ingestion times, and query latency distributions. A query over a
rare path and one over a ubiquitous path exercise different costs.

Historical utility is a separate experiment: a correct ratio does not prove
predictive usefulness. Train strictly on earlier commits, predict held-out
changed files and compare with simple frequency baselines. Never train on the
commit being predicted.

### Chronological holdout probe — 2026-10-21

Added `nib coupled --before <oid>` to query only strict ancestors of a held-out
commit. The cutoff commit and commits outside its ancestry are excluded; this
uses the indexed parent graph, not timestamps. The index must still be current
at HEAD. A query on the full Astro history took 0.26 s with this cutoff versus
0.08 s without it in single local runs; ancestor traversal adds measurable
query cost.

First probe: held out
`71e2b24bb365e5a8809fc0432716f91ecc6d8ab5` (Vite-resolved build output paths),
and queried `packages/astro/src/core/build/index.ts`. The commit changed 32
files. Nib's top five included two of the 31 sibling files actually changed:
`static-build.ts` and `generate.ts` (precision@5 40%). A simple prior-history
file-frequency baseline, with the same 50-file commit cap and package metadata
ignored, included one of those siblings (20%). This is one source/commit pair,
not statistically meaningful evidence of predictive value; it only confirms
that a strict holdout query can produce a promising, evidence-backed result.

An exploratory sample used every third commit among the latest 400 first-parent
commits, skipping merges, changes outside the 2–50-file range and commits
without an eligible source file. One source path was selected deterministically
per held-out commit. With default support and top five, results were:

| Repository | Cases | Nib hits | Nib precision@5 | Nib mean recall@5 | Nib coverage | Frequency hits | Baseline precision@5 | Baseline mean recall@5 | Baseline coverage |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Astro (`withastro/astro`) | 44 | 13 | 5.9% | 7.0% | 20.5% | 2 | 0.9% | 1.3% | 4.5% |
| Personal Astro blog | 31 | 8 | 5.2% | 16.0% | 22.6% | 13 | 8.4% | 16.4% | 32.3% |

Eligible anchors were `.ts`, `.js` or `.astro` files under `packages/astro/src/`
for Astro and `src/` for the blog. The selected anchor was the SHA-256-derived
index into the sorted eligible paths for that commit. Precision counts relevant
files against five slots per case; recall is averaged per held-out commit.

The baseline ranks paths by frequency across each cutoff's strict ancestors,
using single-parent commits with at most 50 changed paths and a minimum path
frequency of three. The same per-repository `.nibignore` rules apply. Nib beats
the baseline on this sample from Astro, while global frequency performs better
on the blog; absolute coverage remains low in both. The samples are exploratory,
not statistically conclusive or a generalization claim. The blog run used HEAD
`54ac79d52b9fb8acdadd2b09da60281768c29667` and its index included two commits
not present in the previous snapshot.

```sh
nib coupled packages/astro/src/core/build/index.ts \
  --before 71e2b24bb365e5a8809fc0432716f91ecc6d8ab5 --json --limit 5
```

Next evaluation should use a preregistered sample of held-out commits and
sources, report coverage and precision/recall at several cutoffs, and compare
against both global file frequency and source-conditioned co-change baselines.

## Preregistered held-out evaluation — `nib evaluate` — 2026-10-21

Protocol (ADR-0009), fixed before any result was read: every `stride`-th of the
latest `window` first-parent commits, oldest first; merges, commits outside
2–50 paths and commits without a modified source skipped; source chosen by
`SHA-256(OID)` among eligible modified paths; history = strict ancestors only.
Every repository used the same `.nibignore`: `*.lock`, `pnpm-lock.yaml`,
`package-lock.json`, `go.sum`, `.changeset/`, `CHANGELOG.md`. Default Nib
filters: support ≥ 3, ≤ 50 paths per commit. Go 1.26.4, macOS arm64, Apple M5,
single runs.

| Repository (HEAD) | Commits | Paths | Index | Sources |
|---|---:|---:|---:|---|
| Personal Astro blog (`51173e1`) | 156 | 467 | 0.72 s / 41 MiB | `src/` `.ts,.js,.astro` |
| `junegunn/fzf` (`ac28b8c`) | 3,764 | 213 | 1.70 s / 85 MiB | `.go` |
| `withastro/astro` (`8ea79a2`) | 15,322 | 21,386 | 18.53 s / 201 MiB | `packages/astro/src/` `.ts,.js,.astro` |

Precision@5 / mean recall@5 / coverage@5 (cases with ≥ 1 hit). Precision counts
empty slots against the predictor; "answered" is cases with any prediction.

| Sample | Cases | nib | cochange (unfiltered) | frequency (global) | nib answered |
|---|---:|---|---|---|---:|
| blog 400/3 | 27 | 8.9% / 6.3% / 18.5% | **18.5% / 15.7% / 55.6%** | 11.1% / 5.1% / 29.6% | 8 |
| fzf 400/3 | 46 | 23.0% / 40.7% / 63.0% | **23.9% / 45.1% / 67.4%** | 14.8% / 22.2% / 52.2% | 43 |
| fzf 2000/5 | 131 | 31.6% / 60.6% / 75.6% | **32.1% / 61.1% / 77.1%** | 19.2% / 39.4% / 66.4% | 130 |
| Astro 400/3 | 42 | 6.2% / 11.2% / 21.4% | **8.6% / 18.2% / 28.6%** | 2.4% / 4.3% / 11.9% | 33 |
| Astro 2000/5 | 127 | 4.7% / 7.0% / 18.9% | **6.3% / 8.5% / 23.6%** | 1.9% / 2.2% / 8.7% | 97 |

Evaluation took 0.09–1.93 s and at most 58 MiB peak RSS per run. k = 1, 3, 10
and the per-case predictions are in each run's `--json` report.

### Findings

- **Conditioning on the source helps.** Source-conditioned co-change beat
  global frequency at k = 5 in every sample, by 1.5–3.9× precision on fzf and
  Astro. On the blog Nib's filtered ranking did not (8.9% vs 11.1%).
- **Nib's default filters cost recall without buying precision.** Unfiltered
  co-change matched or beat `nib` on every sample and every k. The support
  threshold explains most of it: Nib answered 8 of 27 blog cases and 97 of
  127 Astro cases, versus all of them unfiltered.
- Absolute usefulness depends strongly on the repository: fzf (213 paths)
  reached 60% recall@5; Astro (21k paths) stayed below 12%.

These are five samples from three repositories, not a general claim. Next:
compare support thresholds and the commit-size cap separately under this
protocol before changing defaults, and add more repositories.

```sh
nib evaluate --prefix packages/astro/src/ --ext .ts,.js,.astro --window 2000 --stride 5 --json
```

## Tuning support, size cap and ranking — ADR-0010 — 2026-10-21

Protocol fixed before running: tune on older first-parent windows (fzf, Astro
`--offset 400 --window 1600 --stride 3`; blog `--offset 78 --window 78
--stride 1`), validate on newer ones (`--window 400 --stride 3`; blog
`--window 78 --stride 1`). Grid: support {1, 2, 3} × max-files {20, 50, 100} ×
rank {shared, jaccard, recent 90 d, recent 365 d}. Selection: highest mean
precision@5 over the three tuning sets. Same HEADs, sources and `.nibignore`
as the previous section.

Tuning (precision@5, 35 / 170 / 178 cases), selected rows of 36:

| Configuration | Mean | blog | fzf | Astro |
|---|---:|---:|---:|---:|
| **support 1 · ≤ 50 · recent 90 d** (selected) | **24.0%** | 22.3% | 39.1% | 10.6% |
| support 1 · ≤ 50 · recent 365 d | 23.4% | 22.3% | 39.1% | 8.8% |
| support 1 · ≤ 50 · shared | 21.9% | 20.0% | 37.1% | 8.7% |
| support 1 · ≤ 50 · jaccard | 18.0% | 18.9% | 28.4% | 6.9% |
| support 3 · ≤ 50 · shared (previous default) | 17.6% | 9.7% | 36.1% | 7.0% |

Validation (never used for selection; 43 / 46 / 42 cases), precision@5 and
mean recall@5:

| Predictor | blog | fzf | Astro |
|---|---|---|---|
| **selected** | **22.3% / 20.3%** | **27.0% / 54.8%** | **11.9% / 27.7%** |
| previous default | 15.3% / 15.9% | 23.0% / 40.7% | 6.2% / 11.2% |
| unfiltered co-change | 23.3% | 23.9% | 8.6% |
| global frequency | 11.6% | 14.8% | 2.4% |

### Findings

- The selected configuration beat the previous default on all three
  validation sets, so it became the default.
- It beat unfiltered co-change on fzf and Astro, and lost narrowly on the blog.
- Recency mattered most on Astro (validation recall@5 27.7% vs 11.2%); Jaccard
  hurt everywhere, so demoting frequently changed files is not what helps.
- 90 days is the edge of the grid; a shorter half-life was not tested. Explore
  it on tuning windows only, and add repositories before trusting the choice.

Reproduce with `scripts/tune-coupling.py WORKDIR adr-0010 tune` (the full
grid) and `scripts/tune-coupling.py WORKDIR adr-0010 validate s1/f50/recent90
s3/f50/shared`.

## Replication on four new repositories — ADR-0011 — 2026-10-22

Protocol fixed in ADR-0011 before any run. New full clones, same `.nibignore`
plus `CHANGES.rst`, `CHANGES.md`, `CHANGELOG.rst`:

| Repository (HEAD) | Commits | Paths | Index | Sources |
|---|---:|---:|---:|---|
| `pallets/flask` (`d086db8`) | 5,562 | 643 | 1.36 s / 38 MiB | `src/` `.py` |
| `BurntSushi/ripgrep` (`3fce3b5`) | 2,287 | 467 | 0.91 s / 58 MiB | `.rs` |
| `square/okhttp` (`e71e328`) | 6,489 | 3,640 | 3.14 s / 75 MiB | `.kt,.java` |
| `django/django` (`0b4aaa7`) | 35,036 | 11,768 | 24.06 s / 180 MiB | `django/` `.py` |

### Step 1 — replication (validation windows, precision@5)

| Repository | Cases | current default | previous default | unfiltered co-change | global frequency |
|---|---:|---:|---:|---:|---:|
| flask | 2 | 0.0% | 0.0% | 0.0% | 0.0% |
| ripgrep | 24 | **13.3%** | 11.7% | 13.3% | 3.3% |
| okhttp | 31 | **20.0%** | 5.8% | 21.9% | 1.9% |
| django | 59 | **21.0%** | 18.0% | 18.3% | 4.1% |

The current defaults beat the previous ones on three of four new repositories
(Flask is uninformative), so ADR-0010's choice **replicates**. Against
unfiltered co-change they tie on ripgrep, lose on OkHttp and win on Django.
Source conditioning beats global frequency on every informative repository.

### Step 2 — extended grid (45 configurations × 7 repositories)

The rule selected support 1, ≤ 100 paths, recent 90 d (tuning mean 20.7% vs
17.8% for the defaults). On validation it scored a 16.7% mean against 16.5%,
won on two repositories, tied on four and lost on Django. It needed four wins,
so **the defaults stay**. Tuning rows at ≤ 50 paths:

| Rank | support 1 | support 2 | support 3 |
|---|---:|---:|---:|
| shared | 16.7% | 14.5% | 12.9% |
| recent 14 d | 17.2% | 15.0% | 12.9% |
| recent 30 d | 17.4% | 15.2% | 13.1% |
| recent 90 d | 17.8% | 15.6% | 13.5% |
| recent 365 d | 17.9% | 15.5% | 13.5% |

### Findings

- Support 1 is best for every ranking; each extra unit of support costs about
  two points of mean precision@5.
- Recency beats `shared` on six of seven repositories. Half-lives under 90 days
  are worse; 90 and 365 days are equivalent on average. Django, whose code is
  long-lived, prefers 365 days or no decay.
- **Protocol flaw:** `--max-files` also decides which commits become cases, so
  comparing caps compares different populations. The selected ≤ 100 cap's
  tuning advantage came almost entirely from Flask moving from one case to two.
- **Protocol flaw:** sampling the first-parent chain skips merge-workflow
  histories. 105 of Flask's 134 sampled validation commits were merges, leaving
  two cases.

Next: fix both flaws before further tuning. Fix the case population
independently of the predictor's commit-size cap, and sample non-merge commits
from the whole DAG rather than the first-parent chain.

Reproduce with `scripts/tune-coupling.py WORKDIR adr-0011 tune` and
`scripts/tune-coupling.py WORKDIR adr-0011 validate s1/f100/recent90 s1/f50/recent90 s3/f50/shared`.

## Re-running the replication under ADR-0012 — ADR-0013 — 2026-10-24

Protocol fixed in ADR-0013 before any run: the seven repositories, HEADs,
sources and `.nibignore` of ADR-0011; ADR-0012 sampling (non-merge commits of
the whole DAG by author time) with `--case-max-files 50` for every
configuration, so each window scores one set of cases. These numbers are not
comparable with the earlier sections.

### Step 1 — replication (validation windows, precision@5)

| Repository | Cases | current default | previous default | unfiltered co-change | global frequency |
|---|---:|---:|---:|---:|---:|
| blog | 43 | **22.3%** | 15.3% | 23.3% | 11.6% |
| fzf | 51 | **30.6%** | 23.1% | 23.9% | 18.0% |
| astro | 48 | **7.9%** | 7.5% | 7.9% | 1.7% |
| flask | 29 | **18.6%** | 17.2% | 19.3% | 2.8% |
| ripgrep | 24 | **13.3%** | 11.7% | 13.3% | 4.2% |
| okhttp | 31 | **20.0%** | 5.8% | 21.9% | 1.9% |
| django | 52 | 13.1% | **13.5%** | 11.5% | 2.3% |
| mean | | **18.0%** | 13.5% | 17.3% | 6.1% |

The current defaults beat the previous ones on six of seven repositories (five
required), so they **replicate**. Flask now has 29 cases instead of 2. Against
unfiltered co-change the validation margin is small: wins on fzf and Django,
ties on Astro and ripgrep, losses on the blog, Flask and OkHttp.

### Step 2 — grid (45 configurations × 7 repositories, tuning windows)

Tuning cases: blog 35, fzf 170, Astro 175, Flask 49, ripgrep 132, OkHttp 182,
Django 225. Mean precision@5 at ≤ 50 paths:

| Rank | support 1 | support 2 | support 3 |
|---|---:|---:|---:|
| shared | 19.5% | 17.2% | 15.0% |
| recent 14 d | 20.4% | 17.6% | 15.1% |
| recent 30 d | 20.8% | 18.0% | 15.5% |
| recent 90 d | **21.1%** | 18.3% | 15.7% |
| recent 365 d | 20.8% | 18.1% | 15.5% |

With support 1 and recent 90 d, the cap scored 20.3% (≤ 20), 21.1% (≤ 50) and
21.2% (≤ 100). The rule selected ≤ 100. On validation it scored an 18.1% mean
against 18.0%, won on Astro and OkHttp, tied on four and lost on Django. It
needed four wins, so **the defaults stay**.

### Findings

- With a fixed population the cap barely matters between 50 and 100 paths;
  ≤ 20 costs about a point.
- Support 1 and a 90-day half-life remain best. Recency beats `shared` on all
  seven tuning windows.
- On the larger tuning windows the defaults beat unfiltered co-change on all
  seven repositories (21.1% vs 18.5% mean); on the small validation windows
  the margin shrinks to 0.7 points. Source conditioning beats global frequency
  everywhere.
- Validation windows hold 24–52 cases, so single-repository differences of a
  point or two are within noise.

Reproduce with `scripts/tune-coupling.py WORKDIR adr-0013 tune` and
`scripts/tune-coupling.py WORKDIR adr-0013 validate s1/f100/recent90 s1/f50/recent90 s3/f50/shared`.
