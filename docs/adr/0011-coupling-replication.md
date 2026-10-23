# ADR-0011 — Replicating the coupling defaults on new repositories

- **Status:** accepted
- **Date:** 2026-10-21

## Context

ADR-0010 chose support 1, ≤ 50 paths per commit and `recent` ranking with a
90-day half-life on three repositories. A 90-day half-life sat at the edge of
the grid, and three repositories are a small basis. The choice must survive
repositories that took no part in choosing it.

## Decision

Fixed before any run on the new repositories:

- **New repositories:** `pallets/flask` (Python, `src/` `.py`),
  `BurntSushi/ripgrep` (Rust, `.rs`), `square/okhttp` (Kotlin/Java,
  `.kt,.java`) and `django/django` (Python, `django/` `.py`). The shared
  `.nibignore` adds `CHANGES.rst`, `CHANGES.md` and `CHANGELOG.rst`.
- **Windows:** as in ADR-0010 for fzf and Astro. Tune on `--offset 400
  --window 1600 --stride 3`; validate on `--window 400 --stride 3`.
- **Step 1, replication:** evaluate the current defaults and the previous
  defaults (support 3, `shared`) on the new validation windows, with both
  baselines. The defaults replicate if they beat the previous defaults on
  precision@5 in at least three of the four new repositories.
- **Step 2, extended grid:** support {1, 2, 3} × max-files {20, 50, 100} ×
  rank {shared, recent 14 d, recent 30 d, recent 90 d, recent 365 d} on the
  tuning windows of all seven repositories. Jaccard is dropped: it lost on
  every repository in ADR-0010. Select the highest mean precision@5; on a
  tie, the lower support, then the smaller cap, then the longer half-life.
- **Changing defaults again** requires the selection to beat the current
  defaults on mean validation precision@5 over the seven repositories and on
  at least four of them.

## Consequences

- The three original validation windows were already used to confirm
  ADR-0010; reusing them here weakens them as evidence. The four new
  repositories carry the out-of-sample weight.
- A failed replication is reported and the defaults are revisited, not hidden.
