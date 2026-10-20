# History analysis — handoff

Date: 2026-10-20.

## Delivered

The first analytical slice is complete: `nib index [--rebuild] [--json]` and
`nib coupled <repository-relative-file> [options]`. This is the beginning of the
repository-intelligence product, not completion of the entire long-term vision.

- `internal/history`: immutable Git snapshot, streaming raw/numstat parser,
  metadata, rename source and binary-aware line counts.
- `internal/index`: bbolt persistence, numeric IDs, file postings, atomic
  incremental updates/rebuilds, directional coupling and bounded evidence.
- `internal/cli`: flags, snapshot freshness checks, text/JSON output.
- `main.go`: analysis dispatch; no arguments retains the existing commit TUI.

ADR-0008 defines the observation population, path semantics and storage trade-offs.
The TUI was not refactored and no symbols, risk scores or remote integrations
were introduced.

## Verified

- `make ci` passed: formatting, vet, lint (zero issues), unit tests, domain
  coverage gate and real Git integration. `internal/index` coverage: 83.4%.
- Real Git integration covers initial/incremental/rebuild equivalence,
  merged side branches without duplicate observations, divergent histories,
  renames with whitespace/Unicode/newlines, binary files, isolated worktrees
  and explicit shallow-clone rejection.
- Deterministic generated histories compare indexed counts and denominators
  against an independent brute-force oracle. Separate tests cover rollback,
  reopening, cancellation, idempotency and evidence-order stability.
- `make build` and `make bench` passed on Go 1.26.4 / macOS arm64.
  [Benchmark results and limitations](research/file-coupling-benchmarks.md).
- The compiled binary indexed this repository at
  `df5ed0c7df2f9917421aa5be55de670f1e67108e`: 16 commits and 43 paths.
  `internal/ui/model.go` had 12 eligible observations; its test file shared
  9 of them (75%). This is a small smoke check, not evidence of predictive value.

## Resume here

```sh
make ci
make build
./nib index
./nib coupled internal/ui/model.go --json
```

Committing changes advances HEAD, so the local derived index must be updated
again before queries. The database lives in the current worktree's Git directory
and is intentionally not versioned.

Next, measure ingestion and queries on explicitly selected real repositories,
recording initial/incremental time, index size, peak memory and latency
distributions. Profile before optimizing the storage layout. Then evaluate
historical usefulness with strict chronological holdouts and frequency baselines.
Only after those results should symbol identity, extraction and impact semantics
be designed.

## Known boundaries

- Reachable HEAD history only; no all-ref or time-window query yet.
- Merge metadata is stored, but conflict-resolution-only changes are omitted.
- Renames do not join identities; delete/recreate at one path shares history.
- One transaction per update gives rollback safety but its large-history memory
  and latency costs are unmeasured.
- The query decodes every participating commit record. Frequent paths cost more.
- Shallow repositories, non-UTF-8 paths and tokens above 16 MiB fail explicitly.
- Go 1.22 compatibility is declared, but this session executed Go 1.26.4.
  Reassess the Go/bbolt baseline before a release.
