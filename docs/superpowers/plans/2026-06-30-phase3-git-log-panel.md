# Phase 3 — Git-Log Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the optional right-side git-log panel to the Bubble Tea UI and turn `nib` into a persistent session that stays open across commits.

**Architecture:** Two prep commits make the app persistent (ADR-0007: a successful commit refreshes state instead of quitting; `esc`/`q`/`ctrl+c` exit). Then the log panel arrives behind a new narrow injected `Logger` port (ADR-0005), loaded async on `Init` and refreshed after each commit. Layout splits into two columns (files ‖ log) with the commit line full-width below; the log auto-hides under 80 cols. The log is read-only in Phase 3.

**Tech Stack:** Go 1.22, Bubble Tea (`tea`), Lipgloss (dimmed text), shell-out to `git`.

---

## Background the engineer must read first

- `nib` is a terminal commit tool. The UI lives in `internal/ui/model.go` (Bubble Tea: `Init`/`Update`/`View`). Read it before starting.
- **Ports & adapters (ADR-0005):** the model never calls `internal/git` directly; it receives narrow interfaces. `main.go` injects real adapters; `internal/ui/model_test.go` injects fakes. We add a `Logger` port the same way `Repo` already exists.
- **Pure logic split:** `internal/git/git.go` keeps a `var run = func(...)` shell-out seam. `git.Log(scope string, limit int) (string, error)` already exists (supports `"current"` and `"all"`; ignores anything else → plain `git log`).
- **Testing rule (ADR-0002):** state transitions in `Update` are written test-first. `View` (rendering) and the real shell-out are NOT tested. `internal/ui` requires 0% coverage, but we still test transitions.
- **The gate:** `make check` (fmt + vet + lint + test) must pass before every commit. Run it. There is a `gofmt` hook on edits.
- **Commits:** Conventional Commits in English, trailing period in the subject (e.g. `feat: add window size handling.`). Branch first; never commit to `main`.

### Current behavior being changed

`Update` on `commitDoneMsg` with no error currently does `m.done = true; return m, tea.Quit`. We remove the quit. The `done` field and the `if m.done {...}` branch in `View` go away. The test `TestCommitDoneMsgDrivesQuitOrError` is replaced.

---

## File structure

- **Modify** `internal/ui/model.go` — the whole feature: new fields, `Logger` port, `Init`, new message + size handling, two-column `layout`/`hitTest`/`View`, persistent commit handling, exit keys.
- **Modify** `internal/ui/model_test.go` — replace the quit test; add transition tests for exit keys, persistence, window size, log loading, two-column hit-testing. Update the `fakeRepo`/`sampleModel` helpers to also provide a `fakeLogger`.
- **Modify** `main.go` — add the `gitLogger` adapter and pass it to `ui.New`.
- **Modify** `go.mod` / `go.sum` — Lipgloss becomes a direct dependency (`go mod tidy`).
- **Modify** `docs/ROADMAP.md`, `docs/architecture.md`, `CHANGELOG.md` — document Phase 3.

---

## Task 0: Branch

- [ ] **Step 1: Create the feature branch**

Run:
```bash
git checkout main && git pull --ff-only
git checkout -b feat/ui-log-panel
```
Expected: on a new branch `feat/ui-log-panel`.

---

## Task 1 (prep): Exit keys — `esc` walks back then quits, `q` only in Files

**Files:**
- Modify: `internal/ui/model.go` (`updateFiles`, `updateType`)
- Test: `internal/ui/model_test.go`

Currently: `updateFiles` has no `esc` case; `updateType` (closed dropdown) has `case "q": return m, tea.Quit`. We make `esc` quit only at the top zone (Files), and stop `q` from quitting in the Type zone.

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go`:
```go
func TestEscQuitsFromFilesZone(t *testing.T) {
	m, _ := sampleModel() // focus = files
	if _, cmd := send(m, "esc"); cmd == nil {
		t.Error("esc in the files zone (top) should quit")
	}
}

func TestEscWalksBackThenQuits(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "tab") // -> type
	m, _ = send(m, "tab") // -> message
	m, _ = send(m, "esc") // -> type
	if m.focus != zoneType {
		t.Fatalf("esc from message should step to type, got %v", m.focus)
	}
	m, _ = send(m, "esc") // -> files
	if m.focus != zoneFiles {
		t.Fatalf("esc from type should step to files, got %v", m.focus)
	}
	if _, cmd := send(m, "esc"); cmd == nil {
		t.Error("esc at the files zone should quit")
	}
}

func TestQDoesNotQuitWhileTyping(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "tab")
	m, _ = send(m, "tab") // message zone
	next, cmd := send(m, "q")
	if cmd != nil {
		t.Error("q in the message zone is text, must not quit")
	}
	if next.message != "q" {
		t.Errorf("q should be typed, message = %q", next.message)
	}
}

func TestQInTypeZoneDoesNotQuit(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "tab") // type zone
	if _, cmd := send(m, "q"); cmd != nil {
		t.Error("q should quit only in the files zone")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestEsc|TestQ' -v`
Expected: FAIL — `TestEscQuitsFromFilesZone` (esc returns nil cmd) and `TestQInTypeZoneDoesNotQuit` (q still quits).

- [ ] **Step 3: Implement — add `esc` to `updateFiles`**

In `internal/ui/model.go`, in `updateFiles`, add an `esc` arm (place it after the `"shift+tab"` case):
```go
	case "esc":
		return m, tea.Quit
```

- [ ] **Step 4: Implement — remove `q` quit from `updateType`**

In `internal/ui/model.go`, in `updateType`, delete these two lines from the closed-dropdown `switch` (the `case "q"` block):
```go
	case "q":
		return m, tea.Quit
```
Leave the open-dropdown `esc` (closes the dropdown) untouched.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestEsc|TestQ' -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Run the full gate**

Run: `make check`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: esc walks back then quits, q quits only in files zone."
```

---

## Task 2 (prep): Persistent session — commit refreshes instead of quitting (ADR-0007)

**Files:**
- Modify: `internal/ui/model.go` (`Model` struct, `Update`/`updateKey`, `View`)
- Test: `internal/ui/model_test.go`

Remove the `done` field; on a successful commit refresh the working tree, clear the message, and show a transient `flash`. No quit. A keystroke dismisses the flash.

- [ ] **Step 1: Write the failing tests (replace the old quit test)**

In `internal/ui/model_test.go`, DELETE `TestCommitDoneMsgDrivesQuitOrError` entirely and add:
```go
func TestCommitSuccessRefreshesAndStaysOpen(t *testing.T) {
	m, repo := sampleModel()
	m.message = "algo roto"
	// Simulate the tree after the commit landed.
	repo.status = []git.FileChange{{Path: "left.go", Status: 'M', Staged: false}}

	next, cmd := m.Update(commitDoneMsg{})
	dm := next.(Model)

	// Must not quit (persistent session). If a cmd is returned it must not be Quit.
	if cmd != nil {
		if _, isQuit := cmd().(tea.QuitMsg); isQuit {
			t.Fatal("commit success must not quit the persistent session")
		}
	}
	if dm.message != "" {
		t.Errorf("message should reset after commit, got %q", dm.message)
	}
	if dm.flash == "" {
		t.Error("commit success should set a feedback flash")
	}
	if len(dm.files) != 1 || dm.files[0].Path != "left.go" {
		t.Errorf("files should refresh from repo.Status, got %+v", dm.files)
	}
}

func TestCommitFailureKeepsOpenWithError(t *testing.T) {
	m, _ := sampleModel()
	next, cmd := m.Update(commitDoneMsg{err: errors.New("nope")})
	em := next.(Model)
	if em.err == nil {
		t.Error("failed commit should set err")
	}
	if cmd != nil {
		t.Error("failed commit should not return a command")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run TestCommit -v`
Expected: FAIL — `dm.flash` field does not exist yet (compile error). That counts as the failing state.

- [ ] **Step 3: Implement — struct fields**

In `internal/ui/model.go`, in the `Model` struct, DELETE the `done bool` field and ADD:
```go
	flash string // transient success feedback, cleared on the next keystroke
```

- [ ] **Step 4: Implement — `commitDoneMsg` handler**

In `Update`, replace the whole `case commitDoneMsg:` block with:
```go
	case commitDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Persistent session (ADR-0007): refresh state, never quit.
		m.message = ""
		m.flash = "✓ commit creado"
		files, err := m.repo.Status()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.files = files
		m.staged = map[string]bool{}
		for _, f := range files {
			if f.Staged {
				m.staged[f.Path] = true
			}
		}
		if m.cursor >= len(files) {
			m.cursor = 0
		}
		return m, nil
```

- [ ] **Step 5: Implement — dismiss flash on keystroke**

In `updateKey`, add the flash reset as the first line:
```go
func (m Model) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = "" // any keystroke dismisses the success flash
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	...
```

- [ ] **Step 6: Implement — drop the `done` branch in `View`**

In `View`, DELETE this block:
```go
	if m.done {
		return "nib: commit creado.\n"
	}
```

- [ ] **Step 7: Run tests to verify pass**

Run: `go test ./internal/ui/ -run TestCommit -v`
Expected: PASS.

- [ ] **Step 8: Run the full gate**

Run: `make check`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: keep the session open after a commit (ADR-0007)."
```

---

## Task 3: Window size + log visibility helpers

**Files:**
- Modify: `internal/ui/model.go` (`Model`, `New`, `Update`, helpers)
- Test: `internal/ui/model_test.go`

Store terminal dimensions from `tea.WindowSizeMsg`. Add `showLog` (default on) and the `logVisible()` rule (hide under 80 cols) and `leftColWidth()`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go`:
```go
func TestWindowSizeStored(t *testing.T) {
	m, _ := sampleModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	dm := next.(Model)
	if dm.width != 120 || dm.height != 40 {
		t.Errorf("size not stored: %d x %d", dm.width, dm.height)
	}
}

func TestLogHiddenOnNarrowTerminal(t *testing.T) {
	m, _ := sampleModel() // showLog default true
	m.width = 70
	if m.logVisible() {
		t.Error("log must hide under 80 cols")
	}
	m.width = 100
	if !m.logVisible() {
		t.Error("log should show at 100 cols")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run 'TestWindowSize|TestLogHidden' -v`
Expected: FAIL — `width`/`height`/`showLog`/`logVisible` do not exist (compile error).

- [ ] **Step 3: Implement — struct fields**

In the `Model` struct add:
```go
	width   int
	height  int
	showLog bool // git-log panel enabled (Phase 4 /settings will toggle it)
```

- [ ] **Step 4: Implement — default `showLog` in `New`**

In `New`, set the default right after building `m` (before the `repo.Status()` call):
```go
	m.showLog = true
```

- [ ] **Step 5: Implement — handle `WindowSizeMsg`**

In `Update`, add a case (next to the other `case` arms):
```go
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
```

- [ ] **Step 6: Implement — visibility helpers**

Add near the layout section of `model.go`:
```go
const minWidthForLog = 80 // below this the minimal flow keeps the full width

// logVisible reports whether the git-log panel should be drawn: enabled and the
// terminal is wide enough that the minimal flow is not squeezed.
func (m Model) logVisible() bool {
	return m.showLog && m.width >= minWidthForLog
}

// leftColWidth splits the two-column band in half: files left, log right.
func (m Model) leftColWidth() int {
	return m.width / 2
}
```

- [ ] **Step 7: Run tests to verify pass**

Run: `go test ./internal/ui/ -run 'TestWindowSize|TestLogHidden' -v`
Expected: PASS.

- [ ] **Step 8: Gate + commit**

```bash
make check
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: track terminal size and log-panel visibility rule."
```

---

## Task 4: `Logger` port — load the log async on Init

**Files:**
- Modify: `internal/ui/model.go` (`Logger`, `Model`, `New`, `Init`, message, command, `Update`)
- Modify: `main.go` (`gitLogger` adapter, `ui.New` call)
- Modify: `internal/ui/model_test.go` (`fakeLogger`, `sampleModel` helpers)

`git.Log` already exists. Inject it behind a narrow `Logger` port. `New` gains a second parameter. `Init` fires the async load returning `logLoadedMsg`.

- [ ] **Step 1: Update test helpers (so `New` keeps compiling) and write failing tests**

In `internal/ui/model_test.go`, add the fake and a logger-aware helper, and rewrite `sampleModel` to use it:
```go
type fakeLogger struct {
	out      string
	err      error
	scopeArg string
	limitArg int
}

func (f *fakeLogger) Log(scope string, limit int) (string, error) {
	f.scopeArg = scope
	f.limitArg = limit
	return f.out, f.err
}

func sampleModelWithLogger() (Model, *fakeRepo, *fakeLogger) {
	repo := &fakeRepo{status: []git.FileChange{
		{Path: "a.go", Status: 'M', Staged: false},
		{Path: "b.go", Status: '?', Staged: false},
		{Path: "c.go", Status: 'A', Staged: true},
	}}
	lg := &fakeLogger{out: "abc123 feat: x\n"}
	return New(repo, lg), repo, lg
}
```
Then REPLACE the existing `sampleModel` body with:
```go
func sampleModel() (Model, *fakeRepo) {
	m, repo, _ := sampleModelWithLogger()
	return m, repo
}
```
Also update the direct `New` call in `TestNewPropagatesStatusError` to pass a logger:
```go
	m := New(&fakeRepo{statusErr: errors.New("boom")}, &fakeLogger{})
```
Now add the new tests:
```go
func TestInitLoadsLog(t *testing.T) {
	m, _, lg := sampleModelWithLogger()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a log-load command")
	}
	msg := cmd()
	ll, ok := msg.(logLoadedMsg)
	if !ok {
		t.Fatalf("expected logLoadedMsg, got %T", msg)
	}
	if ll.content != lg.out {
		t.Errorf("log content = %q, want %q", ll.content, lg.out)
	}
	if lg.scopeArg != "current" {
		t.Errorf("scope = %q, want current", lg.scopeArg)
	}
}

func TestLogLoadedMsgStored(t *testing.T) {
	m, _ := sampleModel()
	next, _ := m.Update(logLoadedMsg{content: "X\n"})
	dm := next.(Model)
	if dm.logContent != "X\n" {
		t.Errorf("logContent = %q", dm.logContent)
	}
	if dm.logLoading {
		t.Error("logLoading should clear once the log arrives")
	}
}

func TestLogLoadErrorStored(t *testing.T) {
	m, _ := sampleModel()
	next, _ := m.Update(logLoadedMsg{err: errors.New("boom")})
	dm := next.(Model)
	if dm.logErr == nil {
		t.Error("logErr should be set")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run 'TestInitLoadsLog|TestLogLoaded|TestLogLoadError' -v`
Expected: FAIL — `New` takes one arg, `Logger`/`logLoadedMsg`/`logContent`/`logErr`/`logLoading` undefined (compile error).

- [ ] **Step 3: Implement — port, fields, message**

In `internal/ui/model.go`, add the port next to the `Repo` interface:
```go
// Logger is the narrow slice of git the log panel needs. Optional and read-only
// (ADR-0005): keeping it separate from Repo lets /settings disable it later.
type Logger interface {
	Log(scope string, limit int) (string, error)
}
```
Add to the `Model` struct:
```go
	logger     Logger
	logScope   string // "current" | "all"; "custom" deferred to Phase 4
	logContent string
	logErr     error
	logLoading bool
```
Add the message near `commitDoneMsg`:
```go
// logLoadedMsg carries the result of an async git-log fetch.
type logLoadedMsg struct {
	content string
	err     error
}
```

- [ ] **Step 4: Implement — `New` signature + defaults**

Change the signature and add defaults:
```go
func New(repo Repo, logger Logger) Model {
	m := Model{
		repo:      repo,
		logger:    logger,
		logScope:  "current",
		staged:    map[string]bool{},
		shortcuts: typeShortcuts(),
	}
	m.showLog = true
	...
```
(keep the rest of `New` unchanged.)

- [ ] **Step 5: Implement — load command + `Init`**

Add the command and update `Init`:
```go
const logFetchLimit = 100 // fetch a generous window; View crops to the panel height

// loadLog fetches the git log off the UI goroutine.
func (m Model) loadLog() tea.Cmd {
	logger, scope := m.logger, m.logScope
	return func() tea.Msg {
		out, err := logger.Log(scope, logFetchLimit)
		return logLoadedMsg{content: out, err: err}
	}
}

func (m Model) Init() tea.Cmd { return m.loadLog() }
```
(Replace the existing `func (m Model) Init() tea.Cmd { return nil }`.)

- [ ] **Step 6: Implement — handle `logLoadedMsg`**

In `Update`, add a case:
```go
	case logLoadedMsg:
		m.logLoading = false
		m.logContent = msg.content
		m.logErr = msg.err
		return m, nil
```

- [ ] **Step 7: Implement — `main.go` adapter**

In `main.go`, add the adapter beside `gitRepo`:
```go
type gitLogger struct{}

func (gitLogger) Log(scope string, limit int) (string, error) { return git.Log(scope, limit) }
```
And change the model construction:
```go
	model := ui.New(gitRepo{}, gitLogger{})
```

- [ ] **Step 8: Run tests + build**

Run: `go test ./internal/ui/ -v && go build ./...`
Expected: PASS and a clean build (main.go compiles with the new signature).

- [ ] **Step 9: Gate + commit**

```bash
make check
git add internal/ui/model.go internal/ui/model_test.go main.go
git commit -m "feat: inject a Logger port and load the git log on startup."
```

---

## Task 5: Refresh the log after each commit

**Files:**
- Modify: `internal/ui/model.go` (`commitDoneMsg` success path)
- Test: `internal/ui/model_test.go`

On a successful commit, mark the log reloading and return `loadLog()`.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/model_test.go`:
```go
func TestCommitSuccessReloadsLog(t *testing.T) {
	m, _, lg := sampleModelWithLogger()
	lg.out = "new log\n"

	next, cmd := m.Update(commitDoneMsg{})
	dm := next.(Model)

	if !dm.logLoading {
		t.Error("commit success should mark the log as reloading")
	}
	if cmd == nil {
		t.Fatal("commit success should return a reload command")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); isQuit {
		t.Fatal("commit success must not quit")
	}
	ll, ok := msg.(logLoadedMsg)
	if !ok || ll.content != "new log\n" {
		t.Errorf("expected reloaded log, got %T %+v", msg, msg)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run TestCommitSuccessReloadsLog -v`
Expected: FAIL — `cmd` is nil (current handler returns `m, nil`).

- [ ] **Step 3: Implement — return the reload command**

In `Update`, in the `case commitDoneMsg:` success path, change the final two lines from:
```go
		if m.cursor >= len(files) {
			m.cursor = 0
		}
		return m, nil
```
to:
```go
		if m.cursor >= len(files) {
			m.cursor = 0
		}
		m.logLoading = true
		return m, m.loadLog()
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/ui/ -run TestCommit -v`
Expected: PASS (both the refresh and reload tests).

- [ ] **Step 5: Gate + commit**

```bash
make check
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: refresh the git-log panel after each commit."
```

---

## Task 6: Two-column layout + hit-testing

**Files:**
- Modify: `internal/ui/model.go` (`layout` struct, `layout()`, `hitTest`, `logRows` helper)
- Test: `internal/ui/model_test.go`

The two-column band is `max(fileCount, logRows)` tall; the commit line sits below it. `hitTest` must ignore clicks in the right (log) column and in the padding below the file list.

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go`:
```go
func TestHitTestLogColumnIgnored(t *testing.T) {
	m, _ := sampleModel()
	m.width = 120 // logVisible: showLog true and >= 80
	l := m.layout()
	if got := m.hitTest(0, l.filesStart); got.kind != hitFile {
		t.Errorf("left column file row -> %+v, want hitFile", got)
	}
	if got := m.hitTest(80, l.filesStart); got.kind != hitNone {
		t.Errorf("log column click -> %+v, want hitNone", got)
	}
}

func TestHitTestPaddingBelowFilesNotAFile(t *testing.T) {
	m, _ := sampleModel()
	m.width = 120
	m.height = 40 // logRows tall -> band taller than the file list
	l := m.layout()
	if l.bandHeight <= l.fileCount {
		t.Fatalf("expected band taller than files, band=%d files=%d", l.bandHeight, l.fileCount)
	}
	row := l.filesStart + l.fileCount + 1 // inside the band, below the files
	if got := m.hitTest(0, row); got.kind == hitFile {
		t.Errorf("padding row should not be a file hit, got %+v", got)
	}
}
```
The existing `TestHitTestMapsRows` and `TestHitTestDropItemsOnlyWhenOpen` must keep passing: with no `WindowSizeMsg` (width 0) `logVisible()` is false, so `bandHeight == fileCount` and `commitRow` is unchanged.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run TestHitTest -v`
Expected: FAIL — `l.bandHeight` undefined (compile error).

- [ ] **Step 3: Implement — `logRows` helper**

Add near the layout section:
```go
const reservedBelow = 4 // blank + commit line + blank + help line under the band

// logRows is how many rows the log column may fill, derived from terminal height.
func (m Model) logRows() int {
	r := m.height - headerRows - reservedBelow
	if r < 0 {
		return 0
	}
	return r
}
```

- [ ] **Step 4: Implement — `layout` struct + `layout()`**

Add `bandHeight` to the `layout` struct:
```go
type layout struct {
	filesStart int // row of the first file
	fileCount  int
	bandHeight int // rows the two-column (files ‖ log) band occupies
	commitRow  int // the "[ type ▾ ] message" line
	dropStart  int // first dropdown item row (only meaningful when open)
}
```
Replace `layout()` with:
```go
func (m Model) layout() layout {
	n := len(m.files)
	band := n
	if m.logVisible() {
		if lr := m.logRows(); lr > band {
			band = lr
		}
	}
	commitRow := headerRows + band + 1 // one blank line between the band and the commit line
	return layout{
		filesStart: headerRows,
		fileCount:  n,
		bandHeight: band,
		commitRow:  commitRow,
		dropStart:  commitRow + 1,
	}
}
```

- [ ] **Step 5: Implement — `hitTest` two-column guard**

In `hitTest`, replace the file-row branch:
```go
	if y >= l.filesStart && y < l.filesStart+l.fileCount {
		return hit{hitFile, y - l.filesStart}
	}
```
with:
```go
	if y >= l.filesStart && y < l.filesStart+l.fileCount {
		if m.logVisible() && x >= m.leftColWidth() {
			return hit{kind: hitNone} // clicked the read-only log column
		}
		return hit{hitFile, y - l.filesStart}
	}
```

- [ ] **Step 6: Run tests to verify pass**

Run: `go test ./internal/ui/ -run TestHitTest -v`
Expected: PASS (all four hit-test tests).

- [ ] **Step 7: Gate + commit**

```bash
make check
git add internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: two-column layout geometry and log-aware hit-testing."
```

---

## Task 7: Render the panel (View) — NOT unit-tested (render)

**Files:**
- Modify: `internal/ui/model.go` (`View`, new render helpers, lipgloss import)
- Modify: `go.mod` / `go.sum` (lipgloss becomes direct)

Rendering is excluded from unit tests (ADR-0002). Verify by building and running.

- [ ] **Step 1: Implement — import lipgloss + dim helper**

In `internal/ui/model.go`, add to the import block:
```go
	"github.com/charmbracelet/lipgloss"
```
Add near the View helpers:
```go
var dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

func dim(s string) string { return dimStyle.Render(s) }

// truncate hard-caps a plain string to w display columns with an ellipsis.
// Used only on plain (un-styled) cells; the dim status strings are short.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}
```

- [ ] **Step 2: Implement — file/log column builders**

Add:
```go
func (m Model) fileLines() []string {
	lines := make([]string, 0, len(m.files))
	for i, f := range m.files {
		marker := " "
		if m.focus == zoneFiles && i == m.cursor {
			marker = "▸"
		}
		mark := "○"
		if m.staged[f.Path] {
			mark = "✓"
		}
		lines = append(lines, fmt.Sprintf("%s %s %c %s", marker, mark, f.Status, f.Path))
	}
	return lines
}

// logBody returns the log lines or a single dimmed status line.
func (m Model) logBody() []string {
	switch {
	case m.logLoading:
		return []string{dim("cargando…")}
	case m.logErr != nil:
		return []string{dim("log no disponible")}
	}
	trimmed := strings.TrimRight(m.logContent, "\n")
	if trimmed == "" {
		return []string{dim("(sin commits)")}
	}
	return strings.Split(trimmed, "\n")
}

func (m Model) logLines(band int) []string {
	lines := append([]string{dim("GIT LOG · " + m.logScope)}, m.logBody()...)
	if len(lines) > band {
		lines = lines[:band]
	}
	return lines
}
```

- [ ] **Step 3: Implement — replace `View`**

Replace the body of `View` (keep the `m.err` guard) with:
```go
func (m Model) View() string {
	if m.err != nil {
		return "nib: " + m.err.Error() + "\n"
	}

	var b strings.Builder
	b.WriteString("  nib — cambios\n\n")

	l := m.layout()
	left := m.fileLines()
	if m.logVisible() {
		right := m.logLines(l.bandHeight)
		leftW := m.leftColWidth()
		rightW := m.width - leftW - 3 // " │ "
		for i := 0; i < l.bandHeight; i++ {
			lc, rc := "", ""
			if i < len(left) {
				lc = left[i]
			}
			if i < len(right) {
				rc = right[i]
			}
			fmt.Fprintf(&b, "%-*s │ %s\n", leftW, truncate(lc, leftW), truncate(rc, rightW))
		}
	} else {
		for _, line := range left {
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.commitLine())
	b.WriteString("\n")
	if m.dropOpen {
		b.WriteString(m.dropdownView())
	}
	help := m.helpLine()
	if m.flash != "" {
		help = m.flash + " · " + help
	}
	fmt.Fprintf(&b, "\n %s\n", help)
	return b.String()
}
```
Note: `truncate` is applied to log cells too; the dimmed status strings (`cargando…` etc.) are short and never hit the cap, so the ANSI codes are safe.

- [ ] **Step 4: Make lipgloss a direct dependency**

Run: `go mod tidy`
Expected: `go.mod` moves `github.com/charmbracelet/lipgloss` out of the `// indirect` block into a direct `require`.

- [ ] **Step 5: Build and verify it runs**

Run:
```bash
go build ./... && go vet ./...
```
Expected: clean build.

Manual smoke check (in a repo with changes and a wide terminal ≥80 cols):
```bash
go run . 
```
Expected: file list on the left, `GIT LOG · current` panel on the right, commit line below. Make a commit and confirm the app stays open, the message clears, `✓ commit creado` shows, the file list and log refresh. Resize below 80 cols and confirm the log panel disappears. Press `esc` repeatedly to walk back and quit; `ctrl+c` quits anytime. Quit with `q` from the file list.

- [ ] **Step 6: Gate + commit**

```bash
make check
git add internal/ui/model.go go.mod go.sum
git commit -m "feat: render the two-column git-log panel with degraded states."
```

---

## Task 8: Documentation

**Files:**
- Modify: `docs/ROADMAP.md`, `docs/architecture.md`, `CHANGELOG.md`

- [ ] **Step 1: Check off Phase 3 in the roadmap**

In `docs/ROADMAP.md`, change the Phase 3 bullet from `- [ ] **Phase 3` to `- [x] **Phase 3` and append a one-line note that the panel is read-only, auto-hides under 80 cols, and the app is now a persistent session (ADR-0007). Leave `custom` scope noted as deferred to Phase 4.

- [ ] **Step 2: Update the architecture map**

In `docs/architecture.md`, note: the UI now has two injected ports (`Repo`, `Logger`); the session is persistent (commit refreshes, does not quit — ADR-0007); `layout()` drives a two-column band shared by `View()` and `hitTest()`.

- [ ] **Step 3: Add a CHANGELOG entry**

In `CHANGELOG.md`, under the Unreleased/next section, add bullets: persistent session (no auto-quit after commit), git-log panel (read-only, auto-hide <80 cols), `esc` walk-back-then-quit. Reference ADR-0007.

- [ ] **Step 4: Commit**

```bash
git add docs/ROADMAP.md docs/architecture.md CHANGELOG.md
git commit -m "docs: record Phase 3 git-log panel and persistent session."
```

---

## Task 9: Final verification + push

- [ ] **Step 1: Full gate + coverage**

Run:
```bash
make check && make cover
```
Expected: all green. `internal/ui` has no coverage requirement; domain packages stay ≥80%.

- [ ] **Step 2: Push the branch**

Run: `git push -u origin feat/ui-log-panel`
Expected: branch pushed. (The user opens the PR; `gh` is not available in this environment.)

---

## Self-review notes

- **Spec coverage:** layout/auto-hide (T3, T6, T7) · `Logger` port (T4) · load on start + refresh after commit (T4, T5) · persistent session + exit keys (T1, T2) · `current`/`all` wired, `custom` falls through `git.Log` to plain log (T4, deferred per grill) · read-only, right-column click ignored (T6) · loading/error/empty degraded states (T7) · 4 tested transitions (window size, persistence, log loaded, two-column hit-test) with render/shell-out excluded (T7 untested).
- **ADR-0007** authored separately (`docs/adr/0007-persistent-session-lifecycle.md`).
- **Deferred (out of scope, recorded in memory):** branch switcher + quick stash, the "pretty tree" cherry-pick screen, `custom` log scope (Phase 4 decision), copy-hash on click, log scrolling, `/audit` refinement vs conventional-stats.
- **Type consistency:** `Logger.Log(scope string, limit int) (string, error)` matches `git.Log` and `fakeLogger`. `logLoadedMsg{content, err}`, `layout.bandHeight`, `logVisible()`, `leftColWidth()`, `logRows()`, `loadLog()` used consistently across tasks.
