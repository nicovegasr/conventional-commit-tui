# ADR-0015 — `/settings` and the command palette

- **Status:** accepted
- **Date:** 2026-10-24

## Context

The commit TUI draws a git-log panel, a related-files panel (ADR-0014),
captures the mouse and reloads every 2 s, and none of it can be switched off.
That breaks "rich by default, everything heavy switches off". `/audit` and
`/stats` will also need an entry point inside the running TUI.

## Decision

- **Palette:** `/` opens a filterable command palette, Claude-Code style. It
  opens from any zone while the message is empty; once the message has text,
  `/` is text (a commit subject never starts with `/`). The query is typed in
  the message field and the matching commands list below it. A clickable
  `/ comandos` entry in the footer opens it with the mouse. Only `/settings`
  exists now; `/audit` and `/stats` join the same list later instead of each
  claiming a key.
- **Options in this phase**, all applied live:
  - git-log panel on/off;
  - related-files panel on/off;
  - mouse capture on/off (off restores native terminal selection and scroll);
  - git-log scope: current branch / all branches;
  - auto reload: off · 1 s · 2 s · 5 s · 10 s (default 2 s).
- **No custom log command.** Running a user-typed git command needs
  validation and escaping for little gain; `LogCustomCmd` is dropped.
- **`/audit` rows wait for Phase 5**, when `/audit` exists to consume them.
- **Persistence:** TOML at `$XDG_CONFIG_HOME/nib/settings.toml`, falling back
  to `~/.config/nib/settings.toml` (not macOS's `Application Support`, so the
  file is where terminal users look). A missing file or missing keys mean
  `Default()`; out-of-range values fall back to their default. Every change
  is saved immediately; a failed save keeps the change for the session and
  says so. Parsing uses `github.com/BurntSushi/toml` rather than a hand-rolled
  subset.
- **Port:** `Store{ Load(); Save() }`, injected like `Repo` (ADR-0005). The
  model loads once in `New` and enables the mouse from its settings, so
  `main.go` no longer turns it on unconditionally.

## Consequences

- Turning the log off also stops `git log` in the background reload; an
  external commit is then caught by the related panel's stale-index retry.
- Changing the reload interval drops the pending tick (numbered, like related
  queries) so two loops never run at once.
- One new dependency (BurntSushi/toml, no transitive deps).
