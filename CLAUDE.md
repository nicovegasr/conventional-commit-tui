# nib-git — Claude Code project guide

`nib` is a fast, **terminal-first** commit tool for conventional commits — a
lighter "git IDE" you keep open in the VSCode terminal to commit on the fly.
Sibling of `conventional-stats` (zsh toolkit), **independent — no submodule**.

Read `docs/ROADMAP.md` for the design decisions and phase plan, and
`docs/ux-mockups.html` for the target UX (all 6 states).

## Build & run

Go is the runtime (Bubble Tea TUI). If `go` is missing: `brew install go`.

```sh
go mod tidy            # populate go.sum + transitive deps (needs network)
make tools             # install golangci-lint + lefthook, wire git hooks (once)
make build             # build ./nib
make check             # fmt + vet + lint + test  (the pre-commit gate)
make cover             # per-package coverage gate ≥80% domain  (pre-push & CI)
```

The Makefile is the single source of truth: lefthook and CI call the same
targets (see `docs/contributing.md`, ADR-0003). Do not invent ad-hoc commands.

## Architecture

```
main.go               entrypoint (tea.NewProgram, AltScreen + mouse)
internal/commit       conventional-commit types + Format() (auto trailing period)
internal/git          git CLI wrappers: Status, Stage([]files), Commit, Log
internal/audit        /audit hotspots (by commit frequency) + .auditignore
internal/stats        /stats commits-by-type breakdown
internal/settings     toggleable options (Default() = everything on)
internal/ui/model.go  Bubble Tea model (currently a SKELETON with TODOs)
```

The UI layer (`internal/ui`) is the unfinished part. Wire the model to git/
audit/stats/commit through **narrow injected interfaces** (ports & adapters,
ADR-0005), not direct package calls — `main.go` injects real adapters, tests
inject fakes. Do not re-shell-out from the UI.

Domain packages split **pure logic** (tested) from a **thin shell-out seam**
(`var run = func(...)`, stubbed in tests). See `docs/architecture.md`.

## Quality rules (harness)

- **TDD, pragmatic** (ADR-0002): logic written test-first (parsing, ranking,
  `Format`, model `Update→state`); raw render + git shell-out are not.
- **Coverage ≥80% per domain package** (`commit, audit, stats, git, settings, index, pathignore`);
  `internal/ui` requires 0%. Gate: `make cover`.
- **Branch + PR**, never commit to `main`; CI (`make ci`) is the gate (ADR-0004).
- **Non-obvious design decision → write an ADR first** (`docs/adr/`, copy
  `0000-template.md`).

## Non-negotiable design rules

- **Minimal flow is one line**: type dropdown + message field, over the file
  list. Mark files → pick type → type message → enter. This must stay fast.
- **Mouse-first**, keyboard always available as an alternative.
- **"Rich by default, everything heavy switches off in `/settings`."** Never
  make the minimal flow pay for a panel the user can't disable.
- **`git add` takes an explicit file list**, never `git add .`.
- **`/audit` sorts by NUMBER OF COMMITS** touching a file (Tornhill hotspots).
  Lines changed are context only — never the sort key.
- **Commit types are duplicated** from conventional-stats on purpose. Do not
  build a sync mechanism. If they drift, update `internal/commit/types.go` by
  hand. The `tests` label maps to the `test:` prefix.
- **SemVer**, currently `0.1.0` (see `VERSION`).

## Conventions

- Commits: Conventional Commits in English (`feat:`, `fix:`, `chore:` …),
  trailing period in the subject (matches the tool's own `Format`).
- Prose to the user in Spanish; code/identifiers/commits in English. (This is
  why `misspell` is off in `.golangci.yml` — it only understands English.)
- Keep packages small; shell out to `git` rather than pulling a git library.
- **Good naming over comments.** Names carry intent.
- **Rule of three:** do not abstract until ~3 real repetitions justify it. No
  speculative interfaces or layers.
- **CQS:** a function either changes state (command) or returns data (query),
  not both.
- **Less code is better.** Prefer the smallest clear solution; cut boilerplate.

## Slash-commands (planned)

`/audit` · `/stats` · `/settings` — a command palette inside the running TUI
(Claude-Code-terminal style). See ROADMAP phases.
