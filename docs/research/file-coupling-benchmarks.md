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
| First rebuild in place | 18.26 s | 429 MiB | index file 104 MiB (88 MiB allocated) |
| Second rebuild in place | 17.32 s | 435 MiB | unchanged at 104 MiB (88 MiB allocated) |
| `coupled` on `packages/astro/src/core/build/index.ts` | 0.08 s | 15 MiB | 145 eligible, 34 excluded |
| `coupled` on `packages/astro/src/actions/handler.ts` | 0.07 s | 7 MiB | 2 eligible, 2 excluded |

The first rebuild was run against the existing index file; allocated storage
increased from 44 MiB to 88 MiB. A second rebuild remained at that size, so this
sample shows a higher file high-water mark rather than repeated linear growth.
The large-history rebuild's memory and storage costs still need profiling before
recommending frequent rebuilds. Measurements are single runs from
`/usr/bin/time -l`, without cache control or latency percentiles. They are a
real-repository smoke test, not a capacity claim.

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
