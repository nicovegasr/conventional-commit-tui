# ADR-0013 — Re-running the coupling replication under ADR-0012

- **Status:** accepted
- **Date:** 2026-10-24

## Context

ADR-0010 and ADR-0011 chose and confirmed support 1, ≤ 50 paths and `recent`
ranking with a 90-day half-life on samples ADR-0012 found flawed: `--max-files`
also changed the cases, and merge-workflow histories were barely sampled. The
defaults must be judged again on the fixed population before further tuning.

## Decision

Fixed before any run under ADR-0012:

- **Repositories, HEADs, sources and `.nibignore`:** the seven of ADR-0011,
  unchanged.
- **Population:** `--case-max-files 50` for every configuration, so all
  configurations of one window score the same cases.
- **Windows:** as in ADR-0011, now counted in non-merge commits. Tune on
  `--offset 400 --window 1600 --stride 3` (blog `--offset 78 --window 78
  --stride 1`); validate on `--window 400 --stride 3` (blog `--window 78
  --stride 1`).
- **Step 1, replication:** on the validation windows, the current defaults
  (`s1/f50/recent90`) hold if they beat the previous defaults (`s3/f50/shared`)
  on precision@5 in at least five of the seven repositories.
- **Step 2, grid:** the ADR-0011 grid (support {1, 2, 3} × max-files {20, 50,
  100} × rank {shared, recent 14/30/90/365 d}) on the tuning windows. Select
  the highest mean precision@5; ties by lower support, smaller cap, longer
  half-life.
- **Changing defaults** requires the selection to beat the current defaults on
  mean validation precision@5 and on at least four of the seven repositories.
- Both baselines (`cochange`, `frequency`) are reported for every window.

## Consequences

- Every window was seen before under the old sampler, though mostly with
  different cases; none is fully out of sample. A change of defaults here is
  weaker evidence than ADR-0010's and must be confirmed on new repositories.
- If Step 1 fails, the failure is reported and the defaults are revisited, not
  hidden.
