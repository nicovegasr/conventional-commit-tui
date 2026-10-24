# Changelog

All notable changes to nib-git are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versioning is
[SemVer](https://semver.org/). (Auto-derivation from conventional commits is a
later goal — ADR-0004.)

## [Unreleased]

### Added

- The TUI shows what usually changes with the marked files (`RELACIONADOS`,
  above the git log). Modified files that are related but not marked get a
  `+` in the panel and a `↔` in the file list. The history index is refreshed
  in the background on start and after each commit (ADR-0014).
- The file list and the git log reload every 2 s, so edits, `git add` and
  commits made outside nib appear without restarting it. Marks are kept.
- Colors and layout follow the UX mockups: status letters, marks, git-log
  hashes and refs, a commit preview under the message line and a key-styled
  footer.

### Fixed

- A failed commit now shows git's reason (e.g. a rejecting pre-commit hook),
  keeps the message for a retry and refreshes what git already staged.
- The commit message accepts non-ASCII input (ñ, accents, emoji, pastes) and
  backspace removes a whole character.
- `git status` paths with spaces, quotes or renames are staged verbatim
  (`--porcelain -z`).
- Git warnings on stderr can no longer corrupt indexed OIDs or repository paths.
- The log panel shows "cargando…" until its first fetch completes.

### Changed

- Coupling defaults are now support ≥ 1 and recency ranking with a 90-day
  half-life, chosen on tuning windows and confirmed on validation windows
  (ADR-0010). `--min-support 3 --rank shared` restores the previous ranking.

- `nib coupled` accepts flags before or after the file, and `--before` any
  revision (abbreviated OID, tag, `HEAD~3`).
- `/audit` reuses the `.nibignore` matcher and reports a malformed
  `.auditignore` instead of ignoring it. `/audit` and `/stats` break ties
  deterministically.

### Added

- `--rank shared|jaccard|recent` and `--half-life` for `coupled` and
  `evaluate`, plus `evaluate --offset` for tuning/validation splits (ADR-0010).
- `nib evaluate`: reproducible held-out evaluation of file coupling against
  unfiltered co-change and global-frequency baselines, with precision, recall,
  coverage and false positives at k = 1, 3, 5, 10 (ADR-0009).

- Persistent session: a successful commit refreshes the working tree (no
  auto-quit) instead of closing the app (ADR-0007).
- Git-log panel (Phase 3): optional read-only right-side log, auto-hidden under
  80 columns, behind an injected `Logger` port (ADR-0005); refreshes after each
  commit.
- `esc` walk-back-then-quit: steps Message → Type → Files, then quits from the
  files zone; `ctrl+c` quits anytime.

- Minimal commit flow (Phase 1): file list + inline commit line wired to git
  through an injected `Repo` port (ADR-0005). Keyboard path (`j/k`, `space`,
  type shortcuts, `tab`, `enter`), commit result reported, transitions tested.

- Quality harness: `Makefile` gate, `golangci-lint` config, `lefthook` git
  hooks, GitHub Actions CI, and a Claude Code `gofmt` hook (ADR-0001, 0003).
- Per-package coverage gate at 80% for domain packages (`scripts/coverage.sh`,
  ADR-0002).
- Test suites for `commit`, `git`, `audit`, `stats`, `settings`; injectable
  shell-out seam in the domain packages.
- Integration test tier: real git on happy paths behind the `integration`
  build tag (`make test-integration`, ADR-0006).
- Docs: `docs/architecture.md`, `docs/contributing.md`, `docs/adr/`.

## [0.1.0] — 2026-06-25

### Added

- Project scaffold: Go + Bubble Tea TUI skeleton, domain packages
  (`commit`, `git`, `audit`, `stats`, `settings`), `CLAUDE.md`, `ROADMAP`.
