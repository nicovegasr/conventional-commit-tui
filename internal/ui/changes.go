package ui

// The /changes screen: the working tree on the left, the selected file's diff
// on the right, editing delegated to the user's editor (ADR-0017).

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nicovegasr/conventional-commit-tui/internal/git"
)

// Differ is the narrow slice of git the diff pane needs.
type Differ interface {
	Diff(f git.FileChange) (git.Diff, error)
}

// Editor builds the command that opens a file at a line in the user's editor.
type Editor interface {
	Command(path string, line int) *exec.Cmd
}

// diffLoadedMsg answers the diff numbered gen; reset scrolls to its first change.
type diffLoadedMsg struct {
	gen   int
	diff  git.Diff
	err   error
	reset bool
}

// editorClosedMsg reports that the editor gave the terminal back.
type editorClosedMsg struct{ err error }

// changeMargin is how many lines of context stay above a change jumped to.
const changeMargin = 3

// Rows of the screen: title, blank, search box and diff header, rule, body.
const (
	changesSearchRow = 2
	changesBodyRow   = 4
)

// openChanges shows /changes on the file under the commit view's cursor.
func (m Model) openChanges() (Model, tea.Cmd) {
	m.changesOpen, m.changesSearch, m.changesQuery, m.diffFocus = true, false, "", false
	m.changesPath = ""
	if m.cursor < len(m.files) {
		m.changesPath = m.files[m.cursor].Path
	}
	return m.loadDiff(true)
}

// changesMatches are the files whose path contains the query, ignoring case.
func (m Model) changesMatches() []git.FileChange {
	query := strings.ToLower(m.changesQuery)
	var matches []git.FileChange
	for _, f := range m.files {
		if strings.Contains(strings.ToLower(f.Path), query) {
			matches = append(matches, f)
		}
	}
	return matches
}

// changesIdx is the selected file's position among the matches, or -1.
func (m Model) changesIdx() int {
	for i, f := range m.changesMatches() {
		if f.Path == m.changesPath {
			return i
		}
	}
	return -1
}

func (m Model) selectChange(i int) (Model, tea.Cmd) {
	matches := m.changesMatches()
	if len(matches) == 0 {
		m.changesPath, m.diff, m.diffErr = "", git.Diff{}, nil
		return m, nil
	}
	i = min(max(i, 0), len(matches)-1)
	if matches[i].Path == m.changesPath {
		return m, nil
	}
	m.changesPath = matches[i].Path
	return m.loadDiff(true)
}

// keepSelection selects the first match when the filter hid the selected file.
func (m Model) keepSelection() (Model, tea.Cmd) {
	if m.changesIdx() >= 0 {
		return m, nil
	}
	return m.selectChange(0)
}

// loadDiff reads the selected file's diff off the UI goroutine; answers for a
// file no longer selected are dropped.
func (m Model) loadDiff(reset bool) (Model, tea.Cmd) {
	m.diffGen++
	var file git.FileChange
	for _, f := range m.files {
		if f.Path == m.changesPath {
			file = f
		}
	}
	if file.Path == "" || isDir(file.Path) {
		m.diffLoading, m.diff, m.diffErr = false, git.Diff{}, nil
		return m, nil
	}
	m.diffLoading = true
	differ, gen := m.differ, m.diffGen
	return m, func() tea.Msg {
		d, err := differ.Diff(file)
		return diffLoadedMsg{gen: gen, diff: d, err: err, reset: reset}
	}
}

func (m Model) diffLoaded(msg diffLoadedMsg) Model {
	if msg.gen != m.diffGen {
		return m
	}
	m.diffLoading, m.diff, m.diffErr = false, msg.diff, msg.err
	if msg.reset {
		m.diffTop = 0
		if starts := m.changeStarts(); len(starts) > 0 {
			m.diffTop = starts[0] - changeMargin
		}
	}
	return m.scrollDiff(0)
}

// reloadDiff follows edits made outside nib; a load in flight already will.
func (m Model) reloadDiff() (Model, tea.Cmd) {
	if !m.changesOpen || m.diffLoading {
		return m, nil
	}
	return m.loadDiff(false)
}

// editorClosed reloads what the editor may have changed. tea.ExecProcess does
// not restore mouse capture, so it is enabled again here.
func (m Model) editorClosed(msg editorClosedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.flash = "✗ editor: " + lastLine(msg.err.Error())
	}
	m, diff := m.loadDiff(false)
	cmds := []tea.Cmd{m.loadStatus(), diff}
	if m.prefs.MouseCapture {
		cmds = append(cmds, tea.EnableMouseCellMotion)
	}
	return m, tea.Batch(cmds...)
}

// changesRows is how many diff lines and files fit under the header rows,
// leaving the blank line, the footer and a safety row.
func (m Model) changesRows() int {
	if m.height == 0 {
		return 20 // size not known yet
	}
	return max(m.height-changesBodyRow-3, 1)
}

func (m Model) scrollDiff(delta int) Model {
	last := max(len(m.diff.Lines)-m.changesRows(), 0)
	m.diffTop = min(max(m.diffTop+delta, 0), last)
	return m
}

// changeStarts are the indexes where a run of added or removed lines begins.
func (m Model) changeStarts() []int {
	var starts []int
	for i, l := range m.diff.Lines {
		if l.Kind != git.Context && (i == 0 || m.diff.Lines[i-1].Kind == git.Context) {
			starts = append(starts, i)
		}
	}
	return starts
}

// jumpChange scrolls to the next (or previous) change below (or above) the
// one at the top of the view.
func (m Model) jumpChange(next bool) Model {
	anchor := m.diffTop + changeMargin
	starts := m.changeStarts()
	target := -1
	for _, s := range starts {
		if next && s > anchor {
			target = s
			break
		}
		if !next && s < anchor {
			target = s
		}
	}
	if target < 0 {
		return m
	}
	m.diffTop = target - changeMargin
	return m.scrollDiff(0)
}

// isDir tells an untracked folder, which git status lists as one entry.
func isDir(path string) bool { return strings.HasSuffix(path, "/") }

// editLine is where the editor opens: the first change on screen, else the
// top line shown.
func (m Model) editLine() int {
	lines := m.diff.Lines
	if len(lines) == 0 {
		return 1
	}
	shown := lines[m.diffTop:min(m.diffTop+m.changesRows(), len(lines))]
	for _, l := range shown {
		if l.Kind != git.Context {
			return l.New
		}
	}
	return shown[0].New
}

func (m Model) openEditor() (Model, tea.Cmd) {
	idx := m.changesIdx()
	if idx < 0 {
		return m, nil
	}
	switch {
	case m.changesMatches()[idx].Status == 'D':
		m.flash = "✗ el archivo está borrado, no hay nada que abrir"
		return m, nil
	case isDir(m.changesPath):
		m.flash = "✗ es una carpeta nueva, abre sus archivos desde el editor"
		return m, nil
	}
	cmd := m.editor.Command(m.changesPath, m.editLine())
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return editorClosedMsg{err: err} })
}

func (m Model) updateChanges(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.changesSearch {
		return m.updateChangesSearch(key)
	}
	if next, ok := m.navigateDiff(key.String()); ok {
		return next, nil
	}
	switch key.String() {
	case "/":
		m.changesSearch = true
	case "j", "down":
		return m.moveChanges(1)
	case "k", "up":
		return m.moveChanges(-1)
	case "tab", "shift+tab":
		m.diffFocus = !m.diffFocus
	case "e":
		return m.openEditor()
	case " ":
		if m.changesIdx() >= 0 {
			m.staged[m.changesPath] = !m.staged[m.changesPath]
			return m.queryRelated()
		}
	case "esc", "q":
		m.changesOpen = false
	}
	return m, nil
}

// navigateDiff handles the keys that move through the diff whatever the focus.
func (m Model) navigateDiff(key string) (Model, bool) {
	page := m.changesRows() / 2
	switch key {
	case "ctrl+d", "pgdown":
		return m.scrollDiff(page), true
	case "ctrl+u", "pgup":
		return m.scrollDiff(-page), true
	case "n":
		return m.jumpChange(true), true
	case "p":
		return m.jumpChange(false), true
	}
	return m, false
}

// moveChanges scrolls the diff when it has the focus, else moves the file.
func (m Model) moveChanges(step int) (Model, tea.Cmd) {
	if m.diffFocus {
		return m.scrollDiff(step), nil
	}
	return m.selectChange(m.changesIdx() + step)
}

// updateChangesSearch types the filter; enter, esc or ↓ go back to the list.
func (m Model) updateChangesSearch(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "enter", "esc", "down", "tab":
		m.changesSearch = false
		return m, nil
	case "backspace":
		if r := []rune(m.changesQuery); len(r) > 0 {
			m.changesQuery = string(r[:len(r)-1])
		}
	default:
		m.changesQuery += strings.TrimSpace(typedText(key))
	}
	return m.keepSelection()
}

// clickChanges: a click picks a file or the search box, the wheel scrolls the
// pane under the pointer, and the scrollbar can be clicked or dragged.
func (m Model) clickChanges(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	row := msg.Y - changesBodyRow
	switch {
	case msg.Button == tea.MouseButtonWheelDown:
		return m.wheelChanges(msg.X, 1)
	case msg.Button == tea.MouseButtonWheelUp:
		return m.wheelChanges(msg.X, -1)
	case msg.Action == tea.MouseActionRelease:
		return m.releaseChanges(msg.X, row)
	case msg.Button != tea.MouseButtonLeft:
	case msg.Action == tea.MouseActionPress && row >= 0 && row < m.changesRows() && msg.X == m.changesWidth()-1:
		m.diffDragging = true
		return m.dragBar(row), nil
	case msg.Action == tea.MouseActionMotion && m.diffDragging:
		return m.dragBar(min(max(row, 0), m.changesRows()-1)), nil
	}
	return m, nil
}

// wheelChanges moves the file over the list, else scrolls the diff.
func (m Model) wheelChanges(x, step int) (Model, tea.Cmd) {
	if x < m.changesLeftWidth() {
		return m.selectChange(m.changesIdx() + step)
	}
	return m.scrollDiff(3 * step), nil
}

// releaseChanges ends a drag, or clicks the search box, a file or the diff.
func (m Model) releaseChanges(x, row int) (Model, tea.Cmd) {
	dragged, left := m.diffDragging, x < m.changesLeftWidth()
	m.diffDragging = false
	inBody := row >= 0 && row < m.changesRows()
	switch {
	case dragged:
	case row == changesSearchRow-changesBodyRow && left:
		m.changesSearch = true
	case inBody && left:
		m.changesSearch, m.diffFocus = false, false
		return m.selectChange(m.changesFileTop() + row)
	case inBody:
		m.changesSearch, m.diffFocus = false, true
	}
	return m, nil
}

// dragBar scrolls so the thumb sits at the given body row, top to bottom.
func (m Model) dragBar(row int) Model {
	rows := m.changesRows()
	last := max(len(m.diff.Lines)-rows, 0)
	if rows > 1 {
		m.diffTop = row * last / (rows - 1)
	}
	return m.scrollDiff(0)
}

// --- rendering ----------------------------------------------------------------

var (
	addedBg   = lipgloss.Color("#12361f")
	removedBg = lipgloss.Color("#3d161b")
)

func (m Model) changesWidth() int {
	if m.width == 0 {
		return 80
	}
	return m.width
}

func (m Model) changesLeftWidth() int {
	return min(max(m.changesWidth()/3, 20), 40)
}

// changesFileTop is the first file shown: the list scrolls only as far as the
// selection needs.
func (m Model) changesFileTop() int {
	rows, n := m.changesRows(), len(m.changesMatches())
	return min(max(m.changesIdx()-rows+1, 0), max(n-rows, 0))
}

func (m Model) changesView() string {
	w, leftW := m.changesWidth(), m.changesLeftWidth()
	rightW := w - leftW - 3 // " │ "
	rows := m.changesRows()

	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s %s\n\n", paint(green, bold.Render("nib")), paint(cyan, bold.Render("/changes")),
		dim(count(len(m.files), "archivo cambiado", "archivos cambiados")))
	fmt.Fprintf(&b, "%s %s %s\n", padRight(m.searchBox(leftW), leftW), paint(faint, "│"), m.diffHeader(rightW))
	b.WriteString(paint(faint, strings.Repeat("─", leftW+1)+"┼"+strings.Repeat("─", max(rightW+1, 0))) + "\n")

	left, right := m.changesFileLines(leftW), m.diffLines(rightW)
	for i := range rows {
		lc, rc := "", ""
		if i < len(left) {
			lc = left[i]
		}
		if i < len(right) {
			rc = right[i]
		}
		fmt.Fprintf(&b, "%s %s %s\n", padRight(lc, leftW), paint(faint, "│"), rc)
	}
	fmt.Fprintf(&b, "\n %s\n", m.footer())
	return b.String()
}

func (m Model) searchBox(w int) string {
	switch {
	case m.changesSearch:
		return paint(cyan, "/ ") + truncate(m.changesQuery, w-3) + paint(green, "▌")
	case m.changesQuery != "":
		return paint(cyan, "/ ") + truncate(m.changesQuery, w-2)
	}
	return paint(cyan, "/ ") + paint(faint, "buscar…")
}

func (m Model) diffHeader(w int) string {
	if m.changesPath == "" {
		return ""
	}
	counts := fmt.Sprintf("+%d −%d", m.diff.Added, m.diff.Removed)
	path := truncate(m.changesPath, w-len([]rune(counts))-2)
	return padRight(bold.Render(path), w-len([]rune(counts))) +
		paint(green, fmt.Sprintf("+%d", m.diff.Added)) + " " + paint(red, fmt.Sprintf("−%d", m.diff.Removed))
}

func (m Model) changesFileLines(w int) []string {
	matches := m.changesMatches()
	if len(matches) == 0 {
		if len(m.files) == 0 {
			return []string{dim("✓ árbol limpio")}
		}
		return []string{dim("sin coincidencias")}
	}
	top := m.changesFileTop()
	var lines []string
	for _, f := range matches[top:min(top+m.changesRows(), len(matches))] {
		marker, mark := " ", paint(faint, "○")
		if m.staged[f.Path] {
			mark = paint(green, "✓")
		}
		path := truncate(f.Path, w-6)
		if f.Path == m.changesPath {
			marker, path = paint(gray, "▸"), bold.Render(path)
			if !m.diffFocus && !m.changesSearch {
				marker = paint(green, "▸")
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s", marker, mark, paint(statusColor(f.Status), string(f.Status)), path))
	}
	return lines
}

// diffLines renders the visible part of the diff w columns wide: line number,
// sign and text on a green or red background, and the scrollbar last.
func (m Model) diffLines(w int) []string {
	switch {
	case m.changesPath == "":
		return nil
	case isDir(m.changesPath):
		return []string{dim(truncate("carpeta nueva: git status no lista sus archivos por separado", w))}
	case m.diffErr != nil:
		return []string{paint(red, truncate("✗ diff no disponible: "+lastLine(m.diffErr.Error()), w))}
	case m.diff.Binary:
		return []string{dim("archivo binario")}
	case m.diffLoading && len(m.diff.Lines) == 0:
		return []string{dim("leyendo diff…")}
	case len(m.diff.Lines) == 0:
		return []string{dim("sin cambios respecto a HEAD")}
	}
	lines := m.diff.Lines
	rows := m.changesRows()
	last := lines[len(lines)-1]
	numW := len(strconv.Itoa(max(last.New, last.Old)))
	textW := max(w-numW-5, 1) // number, sign, spaces and the bar
	bar := m.scrollbar(rows)
	var out []string
	for i, l := range lines[m.diffTop:min(m.diffTop+rows, len(lines))] {
		num, sign, style := l.New, " ", lipgloss.NewStyle()
		switch l.Kind {
		case git.Added:
			sign, style = "+", style.Background(addedBg)
		case git.Removed:
			num, sign, style = l.Old, "−", style.Background(removedBg)
		}
		text := padRight(truncate(strings.ReplaceAll(l.Text, "\t", "    "), textW), textW)
		out = append(out, dim(fmt.Sprintf("%*d", numW, num))+" "+style.Render(sign+" "+text)+" "+bar[i])
	}
	return out
}

// scrollbar is one cell per body row: the thumb where the view is, and the
// track colored where changes are, so they can be spotted and dragged to.
func (m Model) scrollbar(rows int) []string {
	lines := m.diff.Lines
	n := len(lines)
	thumb := max(rows*rows/max(n, 1), 1)
	thumbAt := 0
	if n > rows {
		thumbAt = m.diffTop * (rows - thumb) / (n - rows)
	}
	bar := make([]string, rows)
	for r := range rows {
		color := faint
		lo := min(r*n/rows, n-1)
		for _, l := range lines[lo:min(max((r+1)*n/rows, lo+1), n)] {
			if l.Kind == git.Added {
				color = green
			} else if l.Kind == git.Removed && color != green {
				color = red
			}
		}
		cell := "│"
		if r >= thumbAt && r < thumbAt+thumb {
			cell = "┃"
			if color == faint {
				color = gray
			}
		}
		bar[r] = paint(color, cell)
	}
	return bar
}
