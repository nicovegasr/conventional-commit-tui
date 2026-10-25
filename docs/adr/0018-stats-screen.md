# ADR-0018 — The `/stats` screen

- **Status:** accepted
- **Date:** 2026-10-25

## Context

`internal/stats` counted commits per conventional type but nothing showed it,
and the roadmap left open whether a timeline should group by day or by week.
There is no mockup for `/stats`; `/audit` (ADR-0016) set the shape of a
palette screen with an in-screen window.

## Decision

- **Entry:** `/stats` in the `/` palette opens a full screen, like `/audit`.
  The window starts at 30 days and `d` (or a click on the title) cycles
  7 · 30 · 90 · 365, the `/audit` choices. No `/settings` row: the window is
  picked where it is read.
- **By type:** one bar per type, sorted by count, with count and share.
  Subjects without a `type:` prefix (e.g. GitHub's default squash title
  `Feat/audit (#13)`) are counted as `sin tipo`: how often the convention is
  broken is part of the answer.
- **Activity:** commits over the window as a one-row bar chart. The
  granularity follows the window, so the row always fits: per day up to 30
  days, per week beyond. Buckets are calendar days in local time, counted
  back from today; the newest is on the right.
- **Population:** `git log` of the current branch since the window start,
  committer date, merges included (a merge is a commit the branch received).
- **Port:** `StatsReader{ Stats(days) (stats.Report, error) }`, injected like
  the others (ADR-0005); `main.go` passes the clock. An answer for a
  superseded window is dropped.

## Consequences

- No per-author breakdown and no scope breakdown yet; both fit the same
  report later.
- Types are not checked against `internal/commit`: a typo like `faet:` shows
  up as its own type, which is useful feedback rather than noise.
