package ui

import (
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/nib-git/internal/commit"
	"github.com/nicovegasr/nib-git/internal/git"
)

// fakeRepo is a trivial in-memory Repo for testing model transitions.
type fakeRepo struct {
	status    []git.FileChange
	statusErr error
	stageErr  error
	commitErr error

	stagedArg []string
	commitArg string
}

func (f *fakeRepo) Status() ([]git.FileChange, error) { return f.status, f.statusErr }
func (f *fakeRepo) Stage(files []string) error        { f.stagedArg = files; return f.stageErr }
func (f *fakeRepo) Commit(subject string) error       { f.commitArg = subject; return f.commitErr }

// key builds a tea.KeyMsg whose String() matches the model's switch arms.
func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// send applies a key and returns the resulting Model and command.
func send(m Model, s string) (Model, tea.Cmd) {
	next, cmd := m.Update(key(s))
	return next.(Model), cmd
}

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

func sampleModel() (Model, *fakeRepo) {
	m, repo, _ := sampleModelWithLogger()
	return m, repo
}

func TestNewLoadsFilesAndPreMarksStaged(t *testing.T) {
	m, _ := sampleModel()

	if len(m.files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(m.files))
	}
	if !m.staged["c.go"] {
		t.Error("c.go was staged in git and should be pre-marked")
	}
	if m.staged["a.go"] {
		t.Error("a.go should not be pre-marked")
	}
}

func TestNewPropagatesStatusError(t *testing.T) {
	m := New(&fakeRepo{statusErr: errors.New("boom")}, &fakeLogger{})
	if m.err == nil {
		t.Error("expected err set when Status fails")
	}
}

func TestCursorMovesWithinBounds(t *testing.T) {
	m, _ := sampleModel()

	m, _ = send(m, "k") // already at top, stays
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped at top)", m.cursor)
	}
	m, _ = send(m, "j")
	m, _ = send(m, "j")
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
	m, _ = send(m, "j") // at bottom, stays
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped at bottom)", m.cursor)
	}
}

func TestSpaceTogglesStage(t *testing.T) {
	m, _ := sampleModel() // cursor at a.go

	m, _ = send(m, "space")
	if !m.staged["a.go"] {
		t.Error("space should stage a.go")
	}
	m, _ = send(m, "space")
	if m.staged["a.go"] {
		t.Error("space again should unstage a.go")
	}
}

func fixIndex(t *testing.T) int {
	t.Helper()
	for i, ty := range commit.Types {
		if ty.Shortcut == 'x' { // fix
			return i
		}
	}
	t.Fatal("no type uses shortcut 'x'")
	return -1
}

func TestTypeShortcutSelectsTypeOnlyInTypeZone(t *testing.T) {
	want := fixIndex(t)

	// In the file zone the letter is inert (predictable: no stray jumps).
	m, _ := sampleModel()
	m, _ = send(m, "x")
	if m.typeIdx == want {
		t.Error("letter must not select a type while focused on files")
	}

	// In the type zone it selects and auto-advances to the message.
	m, _ = send(m, "tab")
	m, _ = send(m, "x")
	if m.typeIdx != want {
		t.Errorf("typeIdx = %d, want %d (fix)", m.typeIdx, want)
	}
	if m.focus != zoneMessage {
		t.Errorf("picking a type should advance focus to the message, got %v", m.focus)
	}
}

func TestTabCyclesFocusZones(t *testing.T) {
	m, _ := sampleModel()
	if m.focus != zoneFiles {
		t.Fatalf("initial focus = %v, want files", m.focus)
	}
	m, _ = send(m, "tab")
	if m.focus != zoneType {
		t.Errorf("after 1 tab focus = %v, want type", m.focus)
	}
	m, _ = send(m, "tab")
	if m.focus != zoneMessage {
		t.Errorf("after 2 tabs focus = %v, want message", m.focus)
	}
	m, _ = send(m, "tab")
	if m.focus != zoneFiles {
		t.Errorf("tab should wrap back to files, got %v", m.focus)
	}
}

func TestDropdownOpenNavigateSelect(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "tab") // focus type

	m, _ = send(m, "down") // open
	if !m.dropOpen {
		t.Fatal("down should open the dropdown on the type zone")
	}
	if m.dropIdx != m.typeIdx {
		t.Errorf("dropdown should start highlighting the current type")
	}
	m, _ = send(m, "j")
	m, _ = send(m, "j")
	if m.dropIdx != 2 {
		t.Errorf("dropIdx = %d, want 2 after two j", m.dropIdx)
	}
	m, _ = send(m, "enter") // select highlighted
	if m.typeIdx != 2 {
		t.Errorf("typeIdx = %d, want 2 (selected item)", m.typeIdx)
	}
	if m.dropOpen {
		t.Error("selecting should close the dropdown")
	}
	if m.focus != zoneMessage {
		t.Error("selecting should advance to the message")
	}
}

func TestDropdownEscClosesWithoutLeavingType(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "tab")
	m, _ = send(m, "down") // open
	m, _ = send(m, "esc")
	if m.dropOpen {
		t.Error("esc should close the dropdown")
	}
	if m.focus != zoneType {
		t.Error("esc on an open dropdown should keep focus on the type zone")
	}
}

func TestMessageFieldFocusAndEditing(t *testing.T) {
	m, _ := sampleModel()

	m, _ = send(m, "tab") // -> type
	m, _ = send(m, "tab") // -> message
	if m.focus != zoneMessage {
		t.Fatal("two tabs should reach the message field")
	}
	// In the message zone, "j" is text, not navigation.
	m, _ = send(m, "h")
	m, _ = send(m, "i")
	m, _ = send(m, "j")
	if m.message != "hij" {
		t.Errorf("message = %q, want %q", m.message, "hij")
	}
	if m.cursor != 0 {
		t.Errorf("cursor moved during typing: %d", m.cursor)
	}
	m, _ = send(m, "backspace")
	if m.message != "hi" {
		t.Errorf("message = %q, want %q after backspace", m.message, "hi")
	}
	m, _ = send(m, "esc")
	if m.focus != zoneType {
		t.Error("esc should step focus back to the type zone")
	}
}

func TestCommitStagesSelectedAndFormatsSubject(t *testing.T) {
	m, repo := sampleModel()

	// Stage a.go (cursor there), pick fix in the type zone (auto-advances to
	// the message), then type. c.go is pre-staged.
	m, _ = send(m, "space") // stage a.go (cursor at a.go)
	m, _ = send(m, "tab")   // -> type
	m, _ = send(m, "x")     // fix, auto-advance to message
	for _, ch := range "algo roto" {
		m, _ = send(m, string(ch))
	}

	_, cmd := send(m, "enter")
	if cmd == nil {
		t.Fatal("enter should return a commit command")
	}
	msg := cmd()
	done, ok := msg.(commitDoneMsg)
	if !ok {
		t.Fatalf("expected commitDoneMsg, got %T", msg)
	}
	if done.err != nil {
		t.Fatalf("unexpected commit error: %v", done.err)
	}

	// a.go (just staged) and c.go (pre-staged), in file order.
	want := []string{"a.go", "c.go"}
	if !reflect.DeepEqual(repo.stagedArg, want) {
		t.Errorf("staged = %v, want %v", repo.stagedArg, want)
	}
	if repo.commitArg != "fix: algo roto." {
		t.Errorf("commit subject = %q, want %q", repo.commitArg, "fix: algo roto.")
	}
}

func TestCommitStageErrorIsReported(t *testing.T) {
	m, repo := sampleModel()
	repo.stageErr = errors.New("stage failed")
	m.message = "algo" // c.go is pre-staged; give it a message so the guard passes

	_, cmd := send(m, "enter")
	done := cmd().(commitDoneMsg)
	if done.err == nil {
		t.Error("expected stage error to be reported")
	}
	if repo.commitArg != "" {
		t.Error("commit must not run if staging failed")
	}
}

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

func TestCommitFailureKeepsOpenWithFlash(t *testing.T) {
	m, _ := sampleModel()
	next, cmd := m.Update(commitDoneMsg{err: errors.New("nope")})
	em := next.(Model)
	if em.err != nil {
		t.Error("a failed commit must NOT set the fatal err (it would tear down the UI)")
	}
	if em.flash == "" {
		t.Error("failed commit should surface a flash")
	}
	if cmd != nil {
		t.Error("failed commit should not reload the unchanged log")
	}
}

func TestEnterWithoutStagedFilesShowsHint(t *testing.T) {
	m, _ := sampleModel()
	m.staged = map[string]bool{} // unmark the pre-staged file: nothing to commit
	m.message = "algo"
	next, cmd := send(m, "enter")
	if cmd != nil {
		t.Error("enter with nothing marked must not run a commit")
	}
	if next.flash == "" {
		t.Error("enter with nothing marked should show a hint flash")
	}
}

func TestEnterWithEmptyMessageShowsHint(t *testing.T) {
	m, _ := sampleModel() // c.go is pre-staged, message empty
	next, cmd := send(m, "enter")
	if cmd != nil {
		t.Error("enter with an empty message must not run a commit")
	}
	if next.flash == "" {
		t.Error("enter with an empty message should show a hint flash")
	}
}

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

func TestQuitKeys(t *testing.T) {
	m, _ := sampleModel()
	if _, cmd := send(m, "q"); cmd == nil {
		t.Error("q should quit")
	}
}

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

// click builds a left-button release at (x, y) and applies it.
func click(m Model, x, y int) Model {
	next, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	})
	return next.(Model)
}

func TestHitTestMapsRows(t *testing.T) {
	m, _ := sampleModel() // 3 files: rows 2,3,4; commit line row 6
	l := m.layout()

	if got := m.hitTest(0, l.filesStart+1); got.kind != hitFile || got.idx != 1 {
		t.Errorf("file row -> %+v, want hitFile idx 1", got)
	}
	if got := m.hitTest(0, l.commitRow); got.kind != hitType {
		t.Errorf("commit line col 0 -> %+v, want hitType", got)
	}
	if got := m.hitTest(40, l.commitRow); got.kind != hitMessage {
		t.Errorf("commit line far right -> %+v, want hitMessage", got)
	}
	if got := m.hitTest(0, 0); got.kind != hitNone {
		t.Errorf("title row -> %+v, want hitNone", got)
	}
}

func TestHitTestDropItemsOnlyWhenOpen(t *testing.T) {
	m, _ := sampleModel()
	l := m.layout()
	if got := m.hitTest(0, l.dropStart); got.kind == hitDropItem {
		t.Error("closed dropdown should not register item hits")
	}
	m.dropOpen = true
	if got := m.hitTest(0, l.dropStart+1); got.kind != hitDropItem || got.idx != 1 {
		t.Errorf("open dropdown row -> %+v, want hitDropItem idx 1", got)
	}
}

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

func TestMouseClickFileTogglesStageAndFocus(t *testing.T) {
	m, _ := sampleModel()
	l := m.layout()
	m = click(m, 0, l.filesStart) // a.go
	if !m.staged["a.go"] {
		t.Error("clicking a file should stage it")
	}
	if m.focus != zoneFiles || m.cursor != 0 {
		t.Error("clicking a file should focus it and move the cursor there")
	}
	m = click(m, 0, l.filesStart)
	if m.staged["a.go"] {
		t.Error("clicking again should unstage")
	}
}

func TestMouseClickTypeOpensAndItemSelects(t *testing.T) {
	m, _ := sampleModel()
	l := m.layout()
	m = click(m, 0, l.commitRow) // the type box
	if !m.dropOpen || m.focus != zoneType {
		t.Fatal("clicking the type box should open the dropdown and focus type")
	}
	m = click(m, 0, l.dropStart+2) // third item
	if m.typeIdx != 2 || m.dropOpen || m.focus != zoneMessage {
		t.Errorf("clicking an item should select it, close, and advance to message; got idx=%d open=%v focus=%v", m.typeIdx, m.dropOpen, m.focus)
	}
}

func TestMouseWheelNavigatesOpenDropdown(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "tab")
	m, _ = send(m, "down") // open, dropIdx = 0
	wheel := func(btn tea.MouseButton) {
		next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: btn})
		m = next.(Model)
	}
	wheel(tea.MouseButtonWheelDown)
	wheel(tea.MouseButtonWheelDown)
	if m.dropIdx != 2 {
		t.Errorf("wheel down -> dropIdx %d, want 2", m.dropIdx)
	}
	wheel(tea.MouseButtonWheelUp)
	if m.dropIdx != 1 {
		t.Errorf("wheel up -> dropIdx %d, want 1", m.dropIdx)
	}
}

func TestCommitFailureShowsGitReasonAndKeepsMessage(t *testing.T) {
	m, repo := sampleModel()
	m.message = "algo"
	repo.status = []git.FileChange{{Path: "a.go", Status: 'M', Staged: true}}

	next, _ := m.Update(commitDoneMsg{err: errors.New("git commit: exit status 1: running hooks\nlint failed")})
	em := next.(Model)

	if em.flash != "✗ lint failed" {
		t.Errorf("flash = %q, want the hook's verdict", em.flash)
	}
	if em.message != "algo" {
		t.Errorf("message = %q, a failure must keep it for the retry", em.message)
	}
	if !em.staged["a.go"] || len(em.files) != 1 {
		t.Errorf("a failure must refresh what git already staged, got %+v", em.files)
	}
}

func TestCommitRefreshFailureIsReported(t *testing.T) {
	m, repo := sampleModel()
	repo.statusErr = errors.New("boom")
	next, _ := m.Update(commitDoneMsg{})
	if got := next.(Model).flash; got != "✓ commit creado · no se pudo refrescar la lista" {
		t.Errorf("flash = %q", got)
	}
}

func TestRefreshKeepsCursorInsideShorterList(t *testing.T) {
	m, repo := sampleModel()
	m.cursor = 2
	repo.status = []git.FileChange{{Path: "left.go", Status: 'M'}}
	next, _ := m.Update(commitDoneMsg{})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d, want 0", got)
	}
}

func TestMessageAcceptsNonASCIIAndBackspacesByRune(t *testing.T) {
	m, _ := sampleModel()
	m.focus = zoneMessage
	for _, k := range []string{"a", "ñ", "space", "á"} {
		m, _ = send(m, k)
	}
	if m.message != "añ á" {
		t.Fatalf("message = %q, want %q", m.message, "añ á")
	}
	m, _ = send(m, "backspace")
	if m.message != "añ " {
		t.Errorf("backspace left %q, want %q", m.message, "añ ")
	}
}

func TestPasteBecomesOneLine(t *testing.T) {
	m, _ := sampleModel()
	m.focus = zoneMessage
	m, _ = send(m, "line one\nline two")
	if m.message != "line one line two" {
		t.Errorf("message = %q", m.message)
	}
}

func TestNewStartsWithLogLoading(t *testing.T) {
	m, _ := sampleModel()
	if !m.logLoading {
		t.Error("the log is fetched by Init, so the panel must start as loading")
	}
}
