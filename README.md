# nib

**English** · [Español](README.es.md)

![CI](https://github.com/nicovegasr/nib-git/actions/workflows/ci.yml/badge.svg)

A terminal tool to write [Conventional Commits](https://www.conventionalcommits.org/) in one line and **not forget anything**: while you mark files, nib tells you which other files usually change with them, according to the repository's git history.

Built to stay open in the VSCode terminal and commit on the fly.

![nib: mark a file and its related files show up](assets/tui.gif)

---

## What you get

1. **One-line commits**: mark files → pick the type → type the message → `enter`. Mouse or keyboard.
2. **Related files**: what usually changes with the files you marked, and which of those are modified but not marked (the classic forgotten test).
3. **Analysis from the CLI**: `nib coupled <file>` explains what changes with a file, with percentages and the commits that back them.
4. **Measured, not promised**: `nib evaluate` checks on real commits whether the suggestions hit. Results below.

Everything is local: nib only reads your repository through `git` and keeps its index inside `.git/`.

---

## Install

Requires Go 1.22+ and git.

```bash
git clone https://github.com/nicovegasr/nib-git
cd nib-git
make build                     # builds ./nib
cp nib ~/.local/bin/           # or any folder on your PATH
```

Then, inside any repository:

```bash
nib
```

---

## The TUI — commit in one line

| Action | Keyboard | Mouse |
|---|---|---|
| Mark / unmark a file | `space` | click the file |
| Move through the list | `j` / `k` or arrows | — |
| Pick the type (`feat`, `fix`, …) | `tab` + its initial | click `[ feat ▾ ]` |
| Type the message and commit | type + `enter` | click the message |
| Go back / quit | `esc` · `q` · `ctrl+c` | — |

Below the commit line you see the exact subject that will be created (`→ feat: … .`), trailing period included. nib runs `git add` **only on the marked files**, never `git add .`.

The session stays open after committing: the file list and the git log reload every 2 s, so editor changes, `git add` or commits made from another terminal show up on their own. Your marks are kept.

### Related files

When you mark files, the right column shows the five that most often change with them:

- yellow `+`: **modified and not marked**. You are probably forgetting it.
- grey `·`: usually changes with the marked files, but you have not touched it.
- `↔` in the file list: the same hint, next to the file.

The percentage is the share of the marked file's commits that also touched the other one. The index is created on first use (a few seconds in the background on a large repository) and kept up to date after every commit, checkout or rebase. If anything fails, the panel collapses to one grey line and you can still commit.

Below 80 columns the right column hides, so it never gets in the way of the minimal flow.

---

## What usually changes with this file? — `nib coupled`

![nib coupled in the terminal](assets/coupled.png)

```bash
nib index                                      # create or update the index
nib coupled internal/ui/model.go               # related files, with evidence
nib coupled internal/ui/model.go --json        # output for other tools
nib coupled internal/ui/model.go --before v1.0 # only history before a commit
```

Each row says how many commits both files shared (`6/7`) and cites concrete commits you can open with `git show`. By default every co-change counts, mass commits (more than 50 files) are ignored and recent history weighs more (90-day half-life).

To cut noise (lockfiles, changelogs, generated code), add a `.nibignore` at the root with one glob or directory per line. No reindexing needed:

```gitignore
*.lock
package-lock.json
CHANGELOG.md
dist/
```

---

## Does it actually work? — `nib evaluate`

`nib evaluate` hides past commits and asks nib, using **only the history before** each one, which other files changed. It compares against two baselines: plain co-change counting with no filters (`co-change`) and suggesting the repository's most-touched files (`frequency`).

Precision of the top 5 suggestions on each repository's most recent commits (parameters were tuned on older commits):

| Repository | nib | co-change | frequency |
|---|---:|---:|---:|
| fzf (Go) | **30.6%** | 23.9% | 18.0% |
| Django (Python) | **13.1%** | 11.5% | 2.3% |
| OkHttp (Kotlin/Java) | 20.0% | **21.9%** | 1.9% |
| Flask (Python) | 18.6% | **19.3%** | 2.8% |
| ripgrep (Rust) | 13.3% | 13.3% | 4.2% |
| Astro (TypeScript) | 7.9% | 7.9% | 1.7% |
| Personal blog (Astro) | 22.3% | **23.3%** | 11.6% |
| mean of 7 repositories | **18.0%** | 17.3% | 6.1% |

The honest reading: looking at **what changes with your file** hits about 3 times more often on average than looking at what changes most overall (from 1.7× on fzf to 10× on OkHttp). nib's filters add little over plain co-change on these small samples (24–52 cases per repository); on larger samples the advantage is about 2.5 points. Methodology and full numbers in [docs/research/file-coupling-benchmarks.md](docs/research/file-coupling-benchmarks.md).

```bash
nib evaluate --prefix src/ --ext .ts,.tsx     # measure nib on your own repository
```

---

## Limits

- A file is a **path**: renames do not join their history.
- Only history reachable from `HEAD`; shallow clones (`--depth`) are rejected.
- Historical co-change is not dependency nor a bug prediction: it is a hint.

---

## Documentation

| Document | Contents |
|---|---|
| [docs/architecture.md](docs/architecture.md) | Packages, flow and design rules |
| [docs/adr/](docs/adr/) | Decisions, one per file (index, evaluation, related-files panel…) |
| [docs/research/file-coupling-benchmarks.md](docs/research/file-coupling-benchmarks.md) | Benchmarks and evaluations on real repositories |
| [docs/ROADMAP.md](docs/ROADMAP.md) | What comes next |
| [docs/contributing.md](docs/contributing.md) | How to work on the repository |

---

## Development

```bash
make check   # format, vet, lint and tests (pre-commit)
make ci      # all of the above + ≥ 80% coverage + real git integration
make bench   # index microbenchmarks
```

The screenshots in this README are recorded with [vhs](https://github.com/charmbracelet/vhs) on a demo clone: `vhs assets/tapes/tui.tape` and `vhs assets/tapes/coupled.tape`.
