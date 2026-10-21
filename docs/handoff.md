# History analysis — handoff

Date: 2026-10-21.

## Delivered

The first analytical slice is complete: `nib index [--rebuild] [--json]` and
`nib coupled <repository-relative-file> [options]`, including `--before <oid>`
for strict-ancestor historical queries. This is the beginning of the
repository-intelligence product, not completion of the entire long-term vision.

- `internal/history`: immutable Git snapshot, streaming raw/numstat parser,
  metadata, rename source, binary-aware line counts and bounded Git diff batches.
- `internal/index`: bbolt persistence, numeric IDs, file postings, atomic
  incremental updates/rebuilds, directional coupling and bounded evidence.
- `.nibignore`: query-time candidate filtering without rebuilding the index.
- `internal/cli`: flags, snapshot freshness checks, text/JSON output.
- `main.go`: analysis dispatch; no arguments retains the existing commit TUI.

ADR-0008 defines the observation population, path semantics and storage trade-offs.
The TUI was not refactored and no symbols, risk scores or remote integrations
were introduced.

## Verified

- `make ci` passed: formatting, vet, lint (zero issues), unit tests, domain
  coverage gate and real Git integration. `internal/index` coverage: 84.3%.
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
- Full-history Astro smoke: 15,322 commits and 21,386 paths indexed in 18.58 s;
  peak Git-process RSS fell from 434 MiB to 201 MiB with 1,000-commit batches.
  A relationship query took 0.08 s. See the benchmark report for measurement
  limits and the index-file high-water observation.

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

Exploratory chronological holdouts showed mixed results: Nib beat global file
frequency on Astro (13 vs. 2 hits across 44 cases) but lost on the blog (8 vs.
13 across 31 cases); coverage was low in both. Do not claim predictive
usefulness yet. Expand the evaluation and add a source-conditioned baseline.
`--before <oid>` excludes the held-out commit and commits outside its ancestry.
Then repeat real-repository measurements under controlled cache conditions and
on an intermediate-sized repository; capture incremental updates and query
latency distributions. Only after those results should symbol identity,
extraction and impact semantics be designed.

## Known boundaries

- Reachable HEAD history only; no all-ref or time-window query yet.
- Merge metadata is stored, but conflict-resolution-only changes are omitted.
- Renames do not join identities; delete/recreate at one path shares history.
- One transaction per update gives rollback safety. Astro measured 18.58 s and
  201 MiB peak Git-process RSS for a full rebuild; the bbolt file retained an
  88 MiB allocated high-water size after subsequent rebuilds.
- The query decodes every participating commit record. Frequent paths cost more.
- Shallow repositories, non-UTF-8 paths and tokens above 16 MiB fail explicitly.
- Go 1.22 compatibility is declared, but this session executed Go 1.26.4.
  Reassess the Go/bbolt baseline before a release.
