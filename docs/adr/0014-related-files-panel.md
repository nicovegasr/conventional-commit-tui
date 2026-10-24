# ADR-0014 — Related files in the commit TUI

- **Status:** accepted
- **Date:** 2026-10-24

## Context

`nib coupled` answers "what usually changes with this file?" (ADR-0008), and
ADR-0010..0013 showed its defaults beat global frequency on every benchmark
repository. It only lives in the CLI. The commit flow is where the answer is
useful: while choosing what goes into a commit, a file that usually changes
with the marked ones and is not marked may be forgotten.

## Decision

- **Question:** what usually changes with the files **marked** for this
  commit. Marking is the intent signal; the cursor would refresh on every
  keystroke.
- **Panel:** a `RELACIONADOS` section at the top of the right column, above the
  git log, with up to five paths. Hidden while nothing is marked; it shares the
  log column's width rule (≥ 80 columns). Each row shows the co-change ratio
  with the marked file it relates to most strongly. A path modified in the
  working tree but not marked is highlighted (`+`): it is the likely omission.
  Marked paths never appear.
- **Several marked files:** each candidate keeps its highest `recent` score
  over the marked sources. That score counts weighted co-changes, so it is in
  the same unit for every source and keeps the ranking ADR-0010 validated.
  Marked files without history (new files) are skipped.
- **Freshness:** every 2 s the TUI reloads the working tree and the log in the
  background; marks survive a reload unless git's staged state for that file
  changed. The index catches up with HEAD on start, after every commit and
  whenever the log changes outside nib (incremental, ADR-0008); a rewritten
  history (checkout, amend, rebase) rebuilds it. The first index of a
  large repository takes seconds; meanwhile the panel says so. Queries run off
  the UI goroutine; a result for a superseded marked set is dropped.
- **Failure never blocks committing:** an index or query error collapses the
  panel to one dim line.
- **Port:** `History{ Refresh() error; Related(files, limit) }`, injected like
  `Repo` and `Logger` (ADR-0005). The adapter reuses the CLI's index handling.
- **Off switch:** a model flag now, the `/settings` toggle when settings exist
  ("rich by default, everything heavy switches off").

## Consequences

- Opening `nib` creates `.git/nib` index state without an explicit
  `nib index`. It is derived, unversioned and incremental.
- Related paths that are not modified are shown but cannot be marked: only
  working-tree files can be committed. Clicking a highlighted row to mark it
  is left for a later iteration.
- The ratio shown belongs to one source; across sources it is context, not a
  probability that the commit needs the file.
