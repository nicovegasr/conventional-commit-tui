# ADR-0012 — A case population independent of the predictor and the merge workflow

- **Status:** accepted
- **Date:** 2026-10-23

## Context

ADR-0011 found two flaws in the ADR-0009 sample:

- `--max-files` filtered both the predictor's observations and which commits
  became cases, so comparing caps compared different populations.
- Sampling the first-parent chain skips every commit that arrived through a
  merge. On Flask 105 of 134 sampled validation commits were merges, leaving
  two cases.

## Decision

- **Case size is its own option:** `--case-max-files` (default 50) keeps held-out
  commits with 2..N paths. `--max-files` only filters what predictors learn
  from. A grid over `--max-files` now scores one fixed set of cases.
- **Sample from the whole DAG:** every non-merge commit reachable from the
  indexed HEAD, ordered by author time, ties by OID. `--offset` and `--window`
  count these commits from the latest; `--stride` is unchanged. Merges are no
  longer sampled, so they are no longer a skip reason.
- The order uses commit data, not the index's insertion IDs: a rebuilt and an
  incrementally updated index of one repository yield the same sample.
- **History stays strict ancestors** of each held-out commit. Samples on
  different branches no longer have nested ancestor sets: the training set is
  extended when a sample descends from the previous one and rebuilt otherwise.
  Held-out commits are processed in index order (parents before children), so
  author dates older than the topology (Django applies patches with their
  original dates) do not force a rebuild per case. Each case's prediction
  depends only on its own ancestors, so the processing order changes no score.

## Consequences

- Results are not comparable with ADR-0009..0011 numbers; the earlier reports
  stay as they were and new runs are reported separately.
- Author time can disagree with topology (rebases, clock skew). It only decides
  which commits are sampled; training never sees anything but real ancestors.
- Merge-workflow repositories rebuild the training set at each branch switch.
  Measured with a 400-commit window: Django 2.9 s, as fast as the first-parent
  sampler; Flask 1.3 s with 67 cases instead of 2.
