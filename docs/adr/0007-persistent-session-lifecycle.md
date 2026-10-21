# ADR-0007 — Persistent session lifecycle (commit does not quit)

- **Status:** accepted
- **Date:** 2026-06-30

## Context

The scaffold flow quits the program after one successful commit
(`commitDoneMsg{err: nil}` → `tea.Quit`). That models `nib` as a one-shot
command. The intended product is a **terminal-first git IDE** you keep open in
the VSCode terminal to commit on the fly (see CLAUDE.md), which implies a
**persistent** session: commit, see the result, keep going.

This also unblocks Phase 3 (the git-log panel): a panel that only ever shows the
pre-commit state, then the app exits, is nearly pointless. A persistent session
lets the log refresh after each commit and stay useful.

## Decision

`nib` stays open until the user explicitly leaves. A successful commit no longer
quits; instead the model:

1. **Does not** `tea.Quit`.
2. Refreshes the working tree (`repo.Status()` again — committed files vanish).
3. Refreshes the git-log panel (the new commit appears on top).
4. Resets the commit line (clears the message) and shows transient `✓` feedback.

Exit keys (no new global binding that breaks typing):

- `ctrl+c` — always quits, from any zone.
- `q` — quits **only** in the Files zone (it is a literal character while typing
  the message).
- `esc` — moves "back" between zones (Message → Type → Files) and, once at the
  Files zone (no further back), quits.

## Consequences

- The git-log panel becomes worthwhile: it reflects live history across commits.
- The error path is unchanged: a failed commit keeps the session open with the
  error shown — already the behavior, now also the success path stays open.
- **Breaks existing behavior + test:** the Phase 1 transition asserting
  `tea.Quit` after a successful commit must change. This lands as a dedicated
  prep commit, conceptually separate from "add the log panel".
- Slightly more state to manage (reload commands after commit), paid back by the
  product actually matching its "keep it open" pitch.
