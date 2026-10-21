# nib

Local repository intelligence from Git history, for humans and developer tools.
Nib's analytical core indexes the history reachable from HEAD and explains which
files have changed together, with counts and commit evidence. The existing
conventional-commit TUI remains available when invoked without arguments.

## Build and use

Requires Go 1.22+ and Git.

```sh
make build
./nib index
./nib coupled internal/inventory/service.go
./nib coupled internal/inventory/service.go --min-support 2 --json
```

Run inside the repository to analyze. File arguments are relative to its root,
including when invoked from a subdirectory. Use `nib --help` for all options.
After building, the binary can also be invoked by absolute path from another
repository; no repository contents are uploaded.

Create a `.nibignore` at the repository root to omit noisy paths from related
file results. It uses one basename glob or directory name per line; blank lines
and `#` comments are ignored. For example:

```gitignore
*.lock
*-lock.yaml
package-lock.json
dist/
packages/generated/
```

Directory names ending in `/` match that directory at any depth. Ignore rules
filter candidate relationships without changing the source-file denominator;
the source path itself cannot be ignored. Changes take effect immediately and
do not require rebuilding the history index.

## What the first analytical slice does

- Persists commit metadata and file changes in the current worktree's Git
  directory (`nib/index.db`), outside the tracked working tree.
- Streams initial history and incrementally imports new reachable commits.
- Refuses stale queries and divergent histories; `nib index --rebuild` replaces
  the snapshot atomically. Failed imports preserve the previous index.
- Computes directional file co-change: shared eligible commits divided by
  eligible commits touching the source file.
- Returns exact counts, filters, the indexed HEAD and bounded commit evidence,
  as readable text or JSON.

Defaults require at least **3 shared observations** and exclude commits touching
more than **50 paths** from both numerator and denominator. A ratio of 100% over
3 observations is not statistical certainty. Historical co-change does not prove
dependency or predict a regression.

## Explicit boundaries

The snapshot covers the DAG reachable from captured HEAD, not all refs. Merge
metadata is retained, but merge diffs are excluded to avoid counting branch work
twice; this omits conflict-resolution-only changes. Root commits are eligible,
subject to the same size filter. There is no time-window filter yet.

A file means a **path**. Renames preserve their source path as context but do not
join identities across paths. Delete/recreate cycles at one path share history.
Binary changes retain a binary marker instead of claiming known line counts.
Shallow repositories and non-UTF-8 paths fail explicitly. Empty repositories
need an initial commit. Each message/path token is limited to 16 MiB.

This version does not implement symbol analysis, risk scoring, historical
prediction evaluation or GitHub integration. Million-commit scale is unverified.
See [the analysis design](docs/adr/0008-history-index-and-file-coupling.md).

## Validation

```sh
make ci      # format, vet, lint, units, coverage, real Git integration
make bench   # synthetic storage/query microbenchmarks; excludes Git ingestion
```

Tests compare indexed results with a brute-force oracle over deterministic
generated histories. Real Git tests cover incremental/rebuild equivalence,
merges, rewrites, rename paths, binaries, worktrees and shallow history.
See [benchmark methodology](docs/research/file-coupling-benchmarks.md).

## Docs

- [Architecture](docs/architecture.md)
- [Decisions](docs/adr/)
- [Roadmap](docs/ROADMAP.md)
- [Contributing](docs/contributing.md)
- [Legacy TUI mockups](docs/ux-mockups.html)
