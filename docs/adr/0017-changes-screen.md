# ADR-0017 — The `/changes` screen

- **Status:** accepted
- **Date:** 2026-10-24

## Context

nib lists what changed but not how. Reviewing a file meant leaving nib for an
IDE, and nib is meant to run in a bare terminal next to other sessions, with
no IDE open. Editing a file from the review is the natural next step, but an
editor embedded in the TUI would be a large amount of code and still a worse
editor than any real one.

## Decision

- **Entry:** `/changes` in the `/` palette opens a full screen, like `/audit`.
  It costs the minimal flow nothing, so it has no `/settings` switch.
- **Left pane:** the working-tree files from `git status`, filtered by a
  search box at the top (substring of the path, case-insensitive).
- **Right pane:** the whole selected file against `HEAD` (staged and unstaged
  together), added lines on green, removed on red, with a scrollbar that also
  marks where the changes are. It opens on the first change; `n`/`p` jump
  between changes. Untracked files diff against `/dev/null` (`--no-index`);
  binary files only say so.
- **Editing is delegated** to the user's editor through `tea.ExecProcess`:
  nib suspends, the editor takes the terminal and nib reloads on exit. The
  editor is `$VISUAL`, `$EDITOR`, else the first of `micro`, `nano`, `vi`
  found, opened at the line of the change on screen.
- **Ports:** `Differ{ Diff(git.FileChange) (git.Diff, error) }` and
  `Editor{ Command(path, line) *exec.Cmd }`, injected like the others
  (ADR-0005). Answers for a file no longer selected are dropped. While the
  screen is open, the poll also reloads the diff, so edits made by other
  tools show up.

## Consequences

- One `git diff` per selection, with no cache; it is fast enough on a single
  file and keeps the view honest.
- Whole-file context means a large file renders many lines; only the visible
  ones are styled.
- A repository with no commits yet has no `HEAD` to diff against: the pane
  shows git's error.
- Staging hunks and discarding changes are out of scope for now.
