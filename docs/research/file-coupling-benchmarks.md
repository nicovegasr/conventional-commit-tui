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

Before a scale claim, measure real repositories with recorded commit/path/change
counts, Git/Go versions, CPU, cache conditions, index size, peak RSS, initial and
incremental ingestion times, and query latency distributions. A query over a
rare path and one over a ubiquitous path exercise different costs.

Historical utility is a separate experiment: a correct ratio does not prove
predictive usefulness. Train strictly on earlier commits, predict held-out
changed files and compare with simple frequency baselines. Never train on the
commit being predicted.
