# nib — roadmap & design decisions

## Current direction — repository history analysis

The product direction is temporal repository analysis: persistent history,
historical relationships and evidence for change impact. The original TUI roadmap below is historical context, not the
current implementation priority.

The first analytical slice implements `nib index [--rebuild]` and
`nib coupled <file> [--json]`: local persistence, incremental DAG ingestion,
file co-change, explicit eligibility filters and inspectable commit evidence.
See ADR-0008 for semantics and limitations.

Next milestones:

1. ~~Reproducible held-out evaluation~~ — `nib evaluate` (ADR-0009). First
   preregistered run on three repositories: source conditioning beats global
   frequency, but Nib's default filters lost to unfiltered co-change. Tuning on
   older windows and validating on newer ones (ADR-0010) replaced the defaults
   with support 1 and recency ranking (90-day half-life). ADR-0011 replicated
   that choice on four new repositories (Flask, ripgrep, OkHttp, Django).
   ADR-0012 fixed the case population: `--case-max-files` is separate from the
   predictor's `--max-files`, and non-merge commits are sampled from the whole
   DAG (Flask: 67 cases instead of 2). Earlier numbers are not comparable.
   ADR-0013 re-ran the replication under it: the defaults hold (better than
   the previous ones on six of seven repositories), though their margin over
   unfiltered co-change on validation is under a point.
2. Measure real repositories at increasing sizes and profile ingestion/query costs.
   Initial Astro smoke completed; repeat under controlled conditions and include
   another repository size before making capacity claims.
3. Add file history/churn/ownership queries, then decide symbol identity and
   extraction semantics before implementing symbol relationships.
4. Expose validated evidence to agents and eventually PR workflows. Risk scores
   require an operational definition and historical validation first.

## Original commit-TUI roadmap

Decisions locked in a design session (2026-06-25). This file is the source of
truth so a fresh Claude session (or you, in a month) doesn't re-debate them.

## Why nib exists

A commit flow you keep open in the VSCode terminal and use 50× a day, faster
than the Source Control panel or lazygit/gitui (both tried, neither fit). It is
**not** a full git client — it's a fast committer + a few inspection commands.

## Decisions (locked)

| Topic | Decision |
|---|---|
| Repo | Separate from `conventional-stats`. No submodule. Independent. |
| Language | Go + Bubble Tea (TUI). |
| Versioning | SemVer, starts `0.1.0`. |
| Minimal flow | One line: type dropdown + message, over the file list. |
| Interaction | Mouse-first; keyboard always available. |
| Mouse caveat | Mouse capture hijacks terminal selection/scroll → toggle in `/settings`. |
| Staging | `git add` takes an explicit file list, never `git add .`. |
| Heavy features | Slash-commands: `/audit`, `/stats`, `/settings`. Not panels in the hot path. |
| Philosophy | Rich by default; everything heavy can be switched off. |
| `/audit` metric | Sort by **commit count** per file (Tornhill hotspots). Lines = context only. |
| Commit types | Duplicated from conventional-stats. No sync. `tests` → `test:`. |

## Phases

- [x] **Phase 0 — scaffold.** Structure, commit/git/audit/stats/settings
  packages, skeleton UI, mockups, CLAUDE.md. (current)
- [x] **Phase 1 — minimal commit flow (the core).** `internal/ui` wired to the
  file list + inline commit line through an injected `Repo` port (ADR-0005).
  Keyboard path: `j/k` move, `space` stage, type shortcuts, `tab` to message,
  `enter` commit, result reported. Transitions covered by tests. *MVP.*
- [x] **Phase 2 — type dropdown + mouse.** Three focus zones (Files → Type →
  Message, `tab`/`shift+tab` cycle); hybrid type dropdown (letter instant-select
  only in the Type zone, `↓`/`enter`/`space`/click opens the cheat-sheet,
  picking auto-advances to Message); `tea.MouseMsg` clicks via a pure
  `hitTest`, wheel scrolls the open dropdown. `layout()` is the single source of
  the row geometry shared by `View()` and `hitTest()`. `▸` marks the active zone
  + contextual help line. See the closed grill decisions in this file's history
  and `internal/ui/model_test.go`.
- [x] **Phase 3 — git-log panel.** Optional right-side log, `LogScope`
  current/all. Off-by-toggle. Panel is **read-only** y se auto-oculta bajo 80
  columnas; la app es ahora una **sesión persistente** (un commit refresca el
  estado en vez de cerrar — ADR-0007). El scope `custom` queda diferido a Fase 4.
- [ ] **Phase 4 — `/settings`.** Command palette + persisted TOML
  (`~/.config/nib/settings.toml`), all toggles live.
- [ ] **Phase 5 — `/audit`.** Render hotspots, `--days`/`--exclude`, threshold
  coloring from settings.
- [ ] **Phase 6 — `/stats`.** Commit-type breakdown view; later timeline/authors.
- [ ] **Phase 7 — release.** `gh repo create nib-git`, push, tag `v0.1.0`,
  build/install instructions, maybe `brew`/`go install`.

## Working style

Phase by phase, a commit at the end of each phase (visible progress in git
history). Conventional Commits in English, trailing period in the subject.

## Open questions

- `/stats` timeline granularity (per day / per week)?
- Distribution: `go install`, Homebrew tap, or prebuilt binaries?
- Does `/audit` v0.2 (commits × file size) need file LOC, or git blame lines?
