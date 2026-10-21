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
