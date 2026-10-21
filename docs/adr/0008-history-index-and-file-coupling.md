# ADR-0008 — Persistent history index and evidence-backed file coupling

- **Status:** accepted
- **Date:** 2026-10-20

## Context

The product direction is local analysis of repository history.
The existing commit TUI is a consumer, not the analytical core. The first slice
must demonstrate useful file relationships before symbols or risk scores.

## Decision

- Add `nib index [--rebuild]` and `nib coupled <file> [--json]` independently
  of the TUI. No arguments retains the existing TUI.
- Use bbolt, an embedded transactional key/value database, pinned to the
  Go-1.22-compatible 1.3.11 series. Revisit the toolchain/dependency baseline
  before release. SQLite was considered; bbolt avoids CGO and its ordered
  keys match the initial posting-list access pattern without a SQL layer.
- Keep the database at the current worktree's Git directory, `nib/index.db`.
  Use private directory/file permissions and a bounded lock wait. Repository
  identity is its canonical absolute Git directory, not a display name/remote.
- Assign numeric commit and path IDs. Store commits, path dictionaries,
  OID lookups, and `(file ID, commit ID)` postings. Do not materialize all
  file pairs: a large commit would create quadratic storage.
- Stream one NUL-delimited Git log, including raw changes and numstat. Paths
  retain whitespace, tabs, newlines and Unicode. Preserve full OIDs (including
  SHA-256), parents, author identity, time, messages, rename source, change
  type and binary-aware line counts. Git's rename detection is heuristic.
- Index the complete DAG reachable from a captured HEAD. Merge commits retain
  metadata but no diff; non-merge commits (including roots) supply observations.
  This avoids counting branch changes again at a merge, but omits merge-only
  conflict resolutions. Shallow repositories are rejected explicitly.
- Fast-forward updates import `old..HEAD`; duplicate OIDs are ignored. A
  rewritten/divergent history requires `--rebuild`. Queries reject stale
  snapshots. Import and rebuild each use one transaction: failure or cancellation
  must preserve the previous snapshot. This trades large-transaction costs for
  a simple atomicity guarantee; do not claim million-commit scale yet.
- Files are paths, not inferred cross-rename identities. A rename records the
  old path as context and touches its destination; queries do not silently join
  histories across renames. Delete/recreate cycles at one path share history.
- Coupling is directional: `shared eligible commits / source eligible commits`.
  Default minimum support is 3 and maximum files per observation is 50. Exclude
  oversized commits from both numerator and denominator; report excluded
  observations and effective filters. Sort by shared count, then path.
- Return bounded commit evidence per relationship plus exact shared counts.
  Coupling describes historical co-change, not dependency or proven risk.
- Read optional root `.nibignore` rules at query time. Filter matching candidate
  paths before ranking, preserve the eligible source-commit denominator, and
  reject a query whose source path is ignored. Ignore changes need no reindex.
  The format supports basename globs and literal directory names ending in `/`;
  it intentionally does not claim full `.gitignore` syntax.

## Consequences

Queries scan the source's postings and participating commit records rather than
the full history. Dense source histories still cost time; benchmarks must expose
that cost. The index is reconstructible, schema-versioned and private to the
worktree. No cloud, symbols, risk prediction or GitHub integration is introduced.
Golden histories, incremental/rebuild equivalence, rollback and real Git edge
cases are required before treating this slice as usable.

## References

- [bbolt documentation](https://github.com/etcd-io/bbolt)
- [Git log output and diff semantics](https://git-scm.com/docs/git-log)
