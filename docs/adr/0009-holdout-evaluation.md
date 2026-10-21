# ADR-0009 — Reproducible held-out evaluation of file coupling

- **Status:** accepted
- **Date:** 2026-10-21

## Context

The exploratory holdouts in `docs/research/file-coupling-benchmarks.md` were
ad-hoc scripts: Nib beat global frequency on Astro and lost on a blog. Before
claiming analytical usefulness the ROADMAP requires a preregistered sample,
several cutoffs, and both a global-frequency and a source-conditioned baseline.
The experiment must be reproducible by anyone from the index alone.

## Decision

- Add `nib evaluate`. It reads the current index only; it never trains on the
  commit being predicted.
- **Sample, fixed by flags before any result is seen:** walk the first-parent
  chain from the indexed HEAD, keep the latest `--window` commits (default 400),
  hold out every `--stride`-th one (default 3), oldest first. Skip merges,
  commits outside 2..`--max-files` paths, and commits without a source.
- **Source:** among the held-out commit's modified (`M`) paths that are not
  ignored and match the optional `--prefix` and `--ext` filters, pick the
  sorted path at index `SHA-256(OID) mod n`. Deterministic, not chosen by the
  analyst, and the source existed before the commit.
- **Targets:** the commit's other paths, minus `.nibignore` matches.
- **History:** strict ancestors of the held-out commit through the stored DAG,
  as `nib coupled --before`. Merges contribute no observations (ADR-0008).
- **Predictors**, each ranking up to 10 paths, ties by path:
  - `nib` — `nib coupled` with the effective filters.
  - `cochange` — source-conditioned co-change with no filters (support 1, no
    commit-size cap). It isolates what Nib's filters add.
  - `frequency` — global file frequency over eligible ancestor commits, with
    the same support threshold, excluding the source. It isolates what
    conditioning on the source adds.
- **Metrics at k = 1, 3, 5, 10:** hits; precision = hits / (k × cases), so an
  empty slot counts against the predictor; mean per-case recall; coverage =
  cases with at least one hit; false positives = predicted − hits. Also report
  how many cases each predictor answered at all.
- Report the per-case sample (OID, source, targets, predictions) in `--json`
  so any number can be audited with `git show`.

## Consequences

- Held-out commits along one first-parent chain have nested ancestor sets, so
  ancestry and global frequencies are extended incrementally: one history
  traversal per evaluation instead of one per case.
- Results describe this sample of this repository. They are evidence to report,
  not a general claim; cross-repository conclusions need several repositories.
- Sources are restricted to modified files, which excludes "what else should I
  touch when adding a file" — a different question for later.
