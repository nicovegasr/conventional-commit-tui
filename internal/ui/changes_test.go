package ui

import (
	"errors"
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/conventional-commit-tui/internal/git"
)

// fakeDiffer answers every file with diffs[path] and records the last one.
type fakeDiffer struct {
	diffs map[string]git.Diff
	err   error

	pathArg string
	calls   int
}

func (f *fakeDiffer) Diff(c git.FileChange) (git.Diff, error) {
	f.pathArg = c.Path
	f.calls++
	return f.diffs[c.Path], f.err
}

// fakeEditor records where it was asked to open and runs nothing.
type fakeEditor struct {
	pathArg string
	lineArg int
}

func (f *fakeEditor) Command(path string, line int) *exec.Cmd {
	f.pathArg, f.lineArg = path, line
	return exec.Command("true")
}

// apply feeds msg to the model and keeps it typed.
func apply(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// longDiff is n context lines with an added line at each index of changes.
func longDiff(n int, changes ...int) git.Diff {
	d := git.Diff{}
	added := map[int]bool{}
	for _, c := range changes {
		added[c] = true
	}
	for i := range n {
		kind := git.Context
		if added[i] {
			kind = git.Added
		}
		d.Lines = append(d.Lines, git.DiffLine{Kind: kind, Text: "x", Old: i + 1, New: i + 1})
	}
	return d
}

func changesModel(d *fakeDiffer, e *fakeEditor) Model {
	m, _ := sampleModel()
	m.differ, m.editor = d, e
	m.width, m.height = 100, 17 // 10 diff rows
	return m
}

// openChangesScreen walks the palette into /changes and feeds back the diff.
func openChangesScreen(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = send(m, "/")
	for _, r := range "ch" {
		m, _ = send(m, string(r))
	}
	m, cmd := send(m, "enter")
	if !m.changesOpen || !m.diffLoading {
		t.Fatal("/changes should open and start loading the diff")
	}
	return run(t, m, cmd)
}

func TestChangesOpensOnTheCursorFileAtItsFirstChange(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"b.go": longDiff(40, 20)}}
	m, _ := send(changesModel(d, &fakeEditor{}), "j") // cursor on b.go
	m = openChangesScreen(t, m)
	if d.pathArg != "b.go" || m.changesPath != "b.go" {
		t.Errorf("diffed %q, selected %q, want b.go", d.pathArg, m.changesPath)
	}
	if m.diffLoading || m.diffTop != 20-changeMargin {
		t.Errorf("diffTop = %d, want the first change minus the margin", m.diffTop)
	}
}

func TestChangesMovingSelectsAndDropsTheOldAnswer(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"b.go": longDiff(3, 1)}}
	m := openChangesScreen(t, changesModel(d, &fakeEditor{}))
	old := m.diffGen
	m, cmd := send(m, "j")
	if m.changesPath != "b.go" || !m.diffLoading {
		t.Fatalf("j: selected %q loading %v", m.changesPath, m.diffLoading)
	}
	next, _ := m.Update(diffLoadedMsg{gen: old, err: errors.New("late")})
	if next.(Model).diffErr != nil {
		t.Error("an answer for the old file must be dropped")
	}
	if m = run(t, m, cmd); len(m.diff.Lines) != 3 {
		t.Errorf("diff not stored: %+v", m.diff)
	}
}

func TestChangesSearchFiltersTheFiles(t *testing.T) {
	m := openChangesScreen(t, changesModel(&fakeDiffer{}, &fakeEditor{}))
	m, _ = send(m, "/")
	if !m.changesSearch {
		t.Fatal("/ should focus the search box")
	}
	m, cmd := send(m, "C") // case-insensitive
	if got := m.changesMatches(); len(got) != 1 || got[0].Path != "c.go" {
		t.Fatalf("matches = %+v, want c.go", got)
	}
	if m.changesPath != "c.go" || cmd == nil {
		t.Errorf("the selection should follow the filter, got %q", m.changesPath)
	}
	m, _ = send(m, "enter")
	if m.changesSearch || m.changesQuery != "C" {
		t.Errorf("enter leaves the box and keeps the query: %v %q", m.changesSearch, m.changesQuery)
	}
}

func TestChangesNextAndPreviousJumpBetweenChanges(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"a.go": longDiff(100, 10, 11, 50, 80)}}
	m := openChangesScreen(t, changesModel(d, &fakeEditor{}))
	for _, want := range []int{50, 80} {
		if m, _ = send(m, "n"); m.diffTop != want-changeMargin {
			t.Errorf("n: diffTop = %d, want %d", m.diffTop, want-changeMargin)
		}
	}
	if m, _ = send(m, "p"); m.diffTop != 50-changeMargin {
		t.Errorf("p: diffTop = %d", m.diffTop)
	}
}

func TestChangesDiffScrollClamps(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"a.go": longDiff(15)}}
	m := openChangesScreen(t, changesModel(d, &fakeEditor{}))
	m, _ = send(m, "tab")
	for range 20 {
		m, _ = send(m, "j")
	}
	if m.diffTop != 15-m.changesRows() {
		t.Errorf("diffTop = %d, want the last page", m.diffTop)
	}
	if m.changesPath != "a.go" {
		t.Error("with the diff focused, j scrolls instead of moving the file")
	}
}

func TestChangesEditOpensTheEditorAtTheChangeOnScreen(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"a.go": longDiff(40, 20)}}
	e := &fakeEditor{}
	m := openChangesScreen(t, changesModel(d, e))
	m, cmd := send(m, "e")
	if cmd == nil || e.pathArg != "a.go" || e.lineArg != 21 {
		t.Fatalf("editor at %s:%d, want a.go:21", e.pathArg, e.lineArg)
	}
	calls := d.calls
	m, cmd = apply(m, editorClosedMsg{})
	if cmd == nil {
		t.Fatal("closing the editor reloads the files and the diff")
	}
	for _, msg := range cmd().(tea.BatchMsg) {
		if msg != nil {
			m, _ = apply(m, msg())
		}
	}
	if d.calls != calls+1 {
		t.Error("the diff should be read again after editing")
	}
	_ = m
}

func TestChangesEditRefusesADeletedFile(t *testing.T) {
	e := &fakeEditor{}
	m := openChangesScreen(t, changesModel(&fakeDiffer{}, e))
	m.files[0].Status = 'D'
	if m, _ = send(m, "e"); e.pathArg != "" || m.flash == "" {
		t.Errorf("a deleted file has nothing to open; flash %q", m.flash)
	}
}

func TestChangesAnUntrackedFolderIsNotDiffed(t *testing.T) {
	d, e := &fakeDiffer{}, &fakeEditor{}
	m := openChangesScreen(t, changesModel(d, e))
	m.files = append(m.files, git.FileChange{Path: "new/", Status: '?'})
	calls := d.calls
	m, _ = send(m, "/")
	for _, r := range "new" {
		m, _ = send(m, string(r))
	}
	if m.changesPath != "new/" || m.diffLoading || d.calls != calls {
		t.Errorf("selected %q, diffed %d times", m.changesPath, d.calls-calls)
	}
	m, _ = send(m, "enter")
	if m, _ = send(m, "e"); e.pathArg != "" || m.flash == "" {
		t.Error("a folder is not handed to the editor")
	}
}

func TestChangesSpaceMarksTheSelectedFile(t *testing.T) {
	m := openChangesScreen(t, changesModel(&fakeDiffer{}, &fakeEditor{}))
	if m, _ = send(m, "space"); !m.staged["a.go"] {
		t.Error("space should mark a.go for the commit")
	}
}

func TestChangesClicksSelectFilesAndDragTheScrollbar(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"a.go": longDiff(110), "c.go": longDiff(2)}}
	m := openChangesScreen(t, changesModel(d, &fakeEditor{}))
	m, cmd := apply(m, tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: 2, Y: changesBodyRow + 2})
	if m.changesPath != "c.go" || cmd == nil {
		t.Fatalf("click on the third row selected %q", m.changesPath)
	}
	m = run(t, m, cmd)
	m.diff = d.diffs["a.go"]
	bar := m.width - 1
	m, _ = apply(m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: bar, Y: changesBodyRow})
	m, _ = apply(m, tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: bar - 3, Y: changesBodyRow + m.changesRows() - 1})
	if m.diffTop != 100 {
		t.Errorf("dragging the bar to the bottom: diffTop = %d, want 100", m.diffTop)
	}
	m, _ = apply(m, tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, X: bar, Y: changesBodyRow})
	if m.diffDragging {
		t.Error("release ends the drag")
	}
}

func TestChangesEscGoesBackToTheCommitView(t *testing.T) {
	m := openChangesScreen(t, changesModel(&fakeDiffer{}, &fakeEditor{}))
	if m, _ = send(m, "esc"); m.changesOpen {
		t.Error("esc should close /changes")
	}
}

func TestChangesViewRendersWithoutPanicking(t *testing.T) {
	d := &fakeDiffer{diffs: map[string]git.Diff{"a.go": longDiff(30, 5)}}
	m := openChangesScreen(t, changesModel(d, &fakeEditor{}))
	if v := m.View(); v == "" {
		t.Error("empty view")
	}
	m.diffErr = errors.New("no HEAD")
	_ = m.View()
}
