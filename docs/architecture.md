# Architecture

## Analytical core

The primary direction is local analysis of Git history (ADR-0008):

```text
main.go → internal/cli → internal/history (Git reader + streaming parser)
                     → internal/index   (transactional storage + coupling)
```

`history` produces commit metadata and path-level changes independently of any
consumer. Its parser is pure and unit-tested; the Git process adapter is covered
by real-repository integration tests. `index` accepts a narrow history source so
its transaction and query logic can be tested without Git. bbolt persists numeric
path/commit IDs, OID lookups and file-to-commit postings. Coupling reads only the
source postings and associated changes, without storing all possible pairs.

The CLI owns flags, output and freshness checks. The index contains no terminal
rendering or command-line parsing. A transaction publishes both data and snapshot
metadata together; a failed rebuild rolls back to the previous snapshot.

The existing TUI below is retained as a separate consumer. It does not yet
display analytical results. Completing its original feature list is not a
prerequisite for developing the analytical core.

## Existing commit TUI

A human-readable map of conventional-commit-tui. For the *why* behind decisions, see
[`adr/`](adr/). For build/test commands, see [`contributing.md`](contributing.md).

## Big picture

nib is a terminal-first conventional-commit tool: a Bubble Tea TUI over a small
set of typed domain packages that shell out to `git`.

```
main.go                 entrypoint; builds the TUI and injects adapters
internal/commit         conventional-commit types + Format()
internal/git            git CLI wrappers: Status, Stage, Commit, Log, DiffFile
internal/editor         picks the user's editor for /changes (ADR-0017)
internal/audit          /audit hotspots (by commit frequency) + .auditignore
internal/stats          /stats commits-by-type breakdown
internal/settings       toggleable options (Default() = everything on)
internal/ui/model.go    Bubble Tea model (wiring in progress)
```

## Data flow

```
user (mouse/keys)
      │
      ▼
internal/ui (Bubble Tea model)         ← state machine: Update → state
      │   depends on NARROW interfaces (ADR-0005)
      ▼
internal/{git,audit,stats,commit}      ← domain logic, pure + testable
      │   thin shell-out seam (var run = func ...)
      ▼
git CLI
```

The domain packages split **pure logic** (parsing, ranking, formatting — fully
tested) from a **thin shell-out seam** (`var run`/`var gitLog…`, stubbed in
tests). This is what makes the ≥80% domain coverage real (ADR-0002).

The UI now depends on **three injected ports**: `Repo` (status/stage/commit),
`Logger` (git log read-only) y `History` (índice de historial: refresco y
archivos relacionados con lo marcado, ADR-0014). La sesión es **persistente**: un commit exitoso
refresca el estado y no cierra la app (ADR-0007). `layout()` calcula una banda
de **dos columnas** (archivos ‖ log) que comparten `View()` y `hitTest()`, de
modo que el render y la detección de clics usan la misma geometría.

## Non-negotiable rules

- **Minimal flow is one line:** mark files → pick type → type message → enter.
  Never make it pay for a panel that `/settings` can disable.
- **Mouse-first**, keyboard always available.
- `git add` takes an **explicit file list**, never `git add .`.
- `/audit` sorts by **number of commits** touching a file (Tornhill hotspots);
  lines changed are context, never the sort key.
- Commit types are **duplicated on purpose** from conventional-stats; no sync
  mechanism. Update `internal/commit/types.go` by hand.

## Where to start reading

`internal/commit/types.go` (smallest, fully tested) → `internal/git/git.go`
(the shell-out seam pattern) → `internal/ui/model.go` (the work in progress).
