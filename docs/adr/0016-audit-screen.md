# ADR-0016 — The `/audit` screen

- **Status:** accepted
- **Date:** 2026-10-24

## Context

`internal/audit` ranks files by the number of commits touching them over a
window (Tornhill hotspots), honoring `.auditignore`, but nothing shows it.
ADR-0015 left a `/audit` entry in the command palette and its `/settings`
rows for this phase. The mockup shows a table with commits, ±lines and a
signal column, and `--days`/`--exclude` flags.

## Decision

- **Entry:** `/audit` in the `/` palette opens a full screen, like `/settings`.
  No flags: the window starts at `/settings`' default and is cycled inside the
  screen (`d` or a click on the window): 7 · 30 · 90 · 365 days. Typing flags
  is slower than a click and the mouse is first-class.
- **Columns:** file, commits, ±lines, signal. Rows stay sorted by commits;
  lines are context only (CLAUDE.md). They come from the same
  `git log --numstat --no-renames`, one numstat line per commit and file;
  binary files count commits with no lines.
- **Signal** against the `/settings` threshold T (default 20): ≥ T red
  `refactor?`, ≥ ¾·T yellow `vigilar`, else `ok`. Dots (1–5) scale with the
  top file's count, so the table reads at a glance at any window.
- **`/settings` rows:** default window (7 · 30 · 90 · 365) and threshold
  (5 · 10 · 20 · 30 · 50), cycled like the reload interval. The excluded
  patterns are listed under the `/audit` table instead of in `/settings`:
  that is where they explain the result. Editing `.auditignore` stays in the
  editor; nib does not embed one.
- **No `--exclude`:** `.auditignore` already covers it and is versioned.
- **Port:** `Auditor{ Audit(days) (audit.Report, error) }`, injected like
  the others (ADR-0005); the query runs off the UI goroutine and an answer
  for a superseded window is dropped.

## Consequences

- The audit reads git each time it opens or the window changes; there is no
  cache. It reuses no history index state (ADR-0008), which counts a
  different population (eligible commits, renames).
- Renames split a file's history at the rename (`--no-renames`), the same
  limitation the mockup's flat count had.
