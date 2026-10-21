# ADR-0010 — Coupling ranking modes and how their defaults are chosen

- **Status:** accepted
- **Date:** 2026-10-21

## Context

The first `nib evaluate` run (ADR-0009) showed that Nib's default filters
(support ≥ 3, ≤ 50 paths per commit) lose to unfiltered co-change. Those
defaults were chosen by judgement. Changing them by looking at the same sample
would overfit, and ranking by `shared / eligible` cannot help: for one source
every candidate shares that denominator, so it orders exactly like `shared`.

## Decision

- Add `--rank` to `coupled` and `evaluate`. Every mode keeps `--min-support`
  on shared commits and reports exact shared counts and evidence:
  - `shared` — most shared eligible commits (ADR-0008).
  - `jaccard` — shared / eligible commits touching either path. Demotes files
    that change with everything (manifests, registries).
  - `recent` — shared commits weighted by `0.5^(age / --half-life days)`,
    age measured from the cutoff commit (or HEAD) by author timestamp.
- Ties fall back to shared count, then path.
- Add `evaluate --offset N`: sample below the latest N first-parent commits.
- **Tuning protocol, fixed before running it:**
  - Tune on an older window; validate on a newer one never used to choose.
    fzf and Astro: tune `--offset 400 --window 1600 --stride 3`, validate
    `--window 400 --stride 3`. Blog (156 commits): tune `--offset 78
    --window 78 --stride 1`, validate `--window 78 --stride 1`.
  - Grid: `--min-support` {1, 2, 3} × `--max-files` {20, 50, 100} ×
    `--rank` {shared, jaccard, recent 90 days, recent 365 days}.
  - Choose one configuration for all repositories: highest mean precision@5
    across the three tuning sets; on a tie, the simpler one (lower support,
    `shared` before other modes).
  - Change the defaults only if the choice beats the current defaults on the
    validation sets. Report the validation numbers, including losses.

## Consequences

- Defaults become a measured choice that can be re-run as repositories are
  added; the protocol is the reproducible artifact, not the numbers.
- `jaccard` needs every candidate's eligible-commit count. `coupled` reads the
  candidates' postings for it, so it costs more than `shared`.
- Three repositories remain a small basis; the choice is provisional.
