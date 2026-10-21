// Package ui holds the Bubble Tea model for the minimal commit flow:
// mark files → pick type → type message → enter. It depends on a narrow Repo
// port (ADR-0005) so the state transitions are testable without real git.
//
// Phase 2 adds three focus zones (Files → Type → Message), an openable type
// dropdown, and mouse clicks. Picking a type auto-advances to the message so
// the keyboard hot path stays a single `tab` + letter. The type shortcut only
// fires while the Type zone is focused, keeping it predictable.
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nicovegasr/nib-git/internal/commit"
	"github.com/nicovegasr/nib-git/internal/git"
)

// Repo is the narrow slice of git the commit flow needs. main.go injects a real
// adapter; tests inject a fake.
type Repo interface {
	Status() ([]git.FileChange, error)
	Stage(files []string) error
	Commit(subject string) error
}

// Logger is the narrow slice of git the log panel needs. Optional and read-only
// (ADR-0005): keeping it separate from Repo lets /settings disable it later.
type Logger interface {
	Log(scope string, limit int) (string, error)
}

// zone is the part of the UI currently driving the keyboard.
type zone int

const (
	zoneFiles zone = iota
	zoneType
	zoneMessage
)

// commitDoneMsg reports the outcome of a commit attempt.
type commitDoneMsg struct{ err error }

// logLoadedMsg carries the result of an async git-log fetch.
type logLoadedMsg struct {
	content string
	err     error
}

// Model is the root application state.
type Model struct {
	repo     Repo
	files    []git.FileChange
	staged   map[string]bool // path -> staged in this session
	cursor   int
	typeIdx  int    // index into commit.Types
	message  string // commit message being typed
	focus    zone   // which zone owns the keyboard
	dropOpen bool   // type dropdown expanded
	dropIdx  int    // highlighted item while the dropdown is open
	flash    string // transient feedback (success, hint or failure), cleared on the next keystroke
	err      error

	width   int
	height  int
	showLog bool // git-log panel enabled (Phase 4 /settings will toggle it)

	logger     Logger
	logScope   string // "current" | "all"; "custom" deferred to Phase 4
	logContent string
	logErr     error
	logLoading bool

	shortcuts map[string]int // type shortcut key -> index into commit.Types
}

// New builds the initial model and loads the working tree through repo.
func New(repo Repo, logger Logger) Model {
	m := Model{
		repo:       repo,
		logger:     logger,
		showLog:    true,
		logScope:   "current",
		logLoading: true, // Init fetches it
		shortcuts:  typeShortcuts(),
	}
	files, err := repo.Status()
	if err != nil {
		m.err = err
		return m
	}
	return m.withFiles(files)
}

// withFiles replaces the file list, marking what git already has staged and
// keeping the cursor inside the new list.
func (m Model) withFiles(files []git.FileChange) Model {
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
	return m
}

// typeShortcuts maps each commit type's shortcut rune to its index.
func typeShortcuts() map[string]int {
	m := make(map[string]int, len(commit.Types))
	for i, t := range commit.Types {
		m[string(t.Shortcut)] = i
	}
	return m
}

const logFetchLimit = 100 // fetch a generous window; View crops to the panel height

// loadLog fetches the git log off the UI goroutine.
func (m Model) loadLog() tea.Cmd {
	logger, scope := m.logger, m.logScope
	return func() tea.Msg {
		out, err := logger.Log(scope, logFetchLimit)
		return logLoadedMsg{content: out, err: err}
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return m.loadLog() }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case commitDoneMsg:
		return m.afterCommit(msg.err)
	case logLoadedMsg:
		m.logLoading = false
		m.logContent = msg.content
		m.logErr = msg.err
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

func (m Model) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = "" // any keystroke dismisses the success flash
	if key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.focus {
	case zoneFiles:
		return m.updateFiles(key)
	case zoneType:
		return m.updateType(key)
	default:
		return m.updateMessage(key)
	}
}

func (m Model) updateFiles(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case " ": // toggle stage on the file under the cursor
		if len(m.files) > 0 {
			p := m.files[m.cursor].Path
			m.staged[p] = !m.staged[p]
		}
	case "tab":
		m.focus = zoneType
	case "shift+tab":
		m.focus = zoneMessage
	case "esc":
		return m, tea.Quit
	case "enter":
		return m.attemptCommit()
	}
	return m, nil
}

// updateType drives the type zone: the dropdown is a discoverable cheat-sheet,
// while the letter shortcut stays the expert path. Selecting a type (letter or
// dropdown) auto-advances to the message field.
func (m Model) updateType(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dropOpen {
		switch key.String() {
		case "j", "down":
			if m.dropIdx < len(commit.Types)-1 {
				m.dropIdx++
			}
		case "k", "up":
			if m.dropIdx > 0 {
				m.dropIdx--
			}
		case "enter", " ":
			return m.pickType(m.dropIdx), nil
		case "esc":
			m.dropOpen = false
		default:
			if idx, ok := m.shortcuts[key.String()]; ok {
				return m.pickType(idx), nil
			}
		}
		return m, nil
	}

	switch key.String() {
	case "down", "enter", " ":
		m.dropOpen = true
		m.dropIdx = m.typeIdx
	case "tab":
		m.focus = zoneMessage
	case "shift+tab":
		m.focus = zoneFiles
	case "esc":
		m.focus = zoneFiles
	default:
		if idx, ok := m.shortcuts[key.String()]; ok {
			return m.pickType(idx), nil
		}
	}
	return m, nil
}

// pickType selects a commit type, closes the dropdown, and advances focus to
// the message field so the hand never leaves the keyboard.
func (m Model) pickType(idx int) Model {
	m.typeIdx = idx
	m.dropOpen = false
	m.focus = zoneMessage
	return m
}

func (m Model) updateMessage(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.focus = zoneType
	case "tab":
		m.focus = zoneFiles
	case "shift+tab":
		m.focus = zoneType
	case "enter":
		return m.attemptCommit()
	case "backspace":
		if r := []rune(m.message); len(r) > 0 {
			m.message = string(r[:len(r)-1])
		}
	default:
		m.message += typedText(key)
	}
	return m, nil
}

// afterCommit keeps the persistent session alive (ADR-0007) whatever the
// outcome. Both paths re-read the working tree: a failed commit may already have
// staged the files. A failure keeps the message and shows git's reason (e.g. a
// rejecting hook) so the user can fix and retry.
func (m Model) afterCommit(err error) (Model, tea.Cmd) {
	var cmd tea.Cmd
	if err != nil {
		m.flash = "✗ " + lastLine(err.Error())
	} else {
		m.message = ""
		m.flash = "✓ commit creado"
		m.logLoading = true
		cmd = m.loadLog()
	}
	files, statusErr := m.repo.Status()
	if statusErr != nil {
		m.flash += " · no se pudo refrescar la lista"
		return m, cmd
	}
	return m.withFiles(files), cmd
}

// lastLine is the most specific line of a multi-line git error: hooks and git
// itself print their verdict last.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// typedText is the printable text a key contributes to the message: any rune
// (ñ, á, emoji, a paste), never control keys. Newlines from a paste would break
// the one-line subject, so they become spaces.
func typedText(key tea.KeyMsg) string {
	switch key.Type {
	case tea.KeySpace:
		return " "
	case tea.KeyRunes:
		return strings.ReplaceAll(string(key.Runes), "\n", " ")
	}
	return ""
}

// updateMouse handles left clicks (mapped through hitTest) and the wheel, which
// scrolls the open dropdown. The mouse-capture toggle lives in /settings
// (Phase 4); here it is always on (main.go enables it).
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.dropOpen && msg.Button == tea.MouseButtonWheelUp {
		if m.dropIdx > 0 {
			m.dropIdx--
		}
		return m, nil
	}
	if m.dropOpen && msg.Button == tea.MouseButtonWheelDown {
		if m.dropIdx < len(commit.Types)-1 {
			m.dropIdx++
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	h := m.hitTest(msg.X, msg.Y)
	switch h.kind {
	case hitFile:
		m.cursor = h.idx
		m.focus = zoneFiles
		p := m.files[h.idx].Path
		m.staged[p] = !m.staged[p]
	case hitType:
		m.focus = zoneType
		m.dropOpen = !m.dropOpen
		m.dropIdx = m.typeIdx
	case hitDropItem:
		return m.pickType(h.idx), nil
	case hitMessage:
		m.focus = zoneMessage
	case hitNone:
		m.dropOpen = false
	}
	return m, nil
}

// stagedFiles returns the paths currently marked for commit, in file order so
// the command is deterministic (and testable).
func (m Model) stagedFiles() []string {
	var files []string
	for _, f := range m.files {
		if m.staged[f.Path] {
			files = append(files, f.Path)
		}
	}
	return files
}

// attemptCommit guards the enter key: it refuses to commit with nothing marked
// or an empty message, surfacing a hint instead of failing. This keeps `enter`
// safe and never lets a no-op collapse the flow.
func (m Model) attemptCommit() (Model, tea.Cmd) {
	if len(m.stagedFiles()) == 0 {
		m.flash = "marca al menos un archivo (espacio)"
		return m, nil
	}
	if strings.TrimSpace(m.message) == "" {
		m.flash = "escribe un mensaje antes de commitear"
		return m, nil
	}
	return m, m.commit()
}

// commit stages the selected files and creates the commit, reporting the
// outcome as a commitDoneMsg.
func (m Model) commit() tea.Cmd {
	files := m.stagedFiles()
	subject := commit.Format(commit.Types[m.typeIdx].Prefix, m.message)
	repo := m.repo
	return func() tea.Msg {
		if err := repo.Stage(files); err != nil {
			return commitDoneMsg{err: err}
		}
		return commitDoneMsg{err: repo.Commit(subject)}
	}
}

// --- layout & hit-testing -------------------------------------------------
//
// layout is the SINGLE source of the row geometry: View() draws with it and
// hitTest() reads it, so a click can never drift from what is rendered.

const headerRows = 2 // title + blank line above the file list

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

const reservedBelow = 4 // blank + commit line + blank + help line under the band

// logRows is how many rows the log column may fill. It is derived from the
// terminal height and MUST leave room for everything drawn below the band —
// including the open dropdown — plus a one-row safety margin. If the whole frame
// ever exceeds the terminal height, Bubble Tea cannot clear it cleanly: the
// dropdown ghosts and mouse clicks land on the wrong rows.
func (m Model) logRows() int {
	r := m.height - headerRows - reservedBelow - 1 // -1: never fill the last row
	if m.dropOpen {
		r -= len(commit.Types) // the open dropdown needs its own rows below the commit line
	}
	if r < 0 {
		return 0
	}
	return r
}

type layout struct {
	filesStart int // row of the first file
	fileCount  int
	bandHeight int // rows the two-column (files ‖ log) band occupies
	commitRow  int // the "[ type ▾ ] message" line (type and message share it)
	dropStart  int // first dropdown item row (only meaningful when open)
}

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

type hitKind int

const (
	hitNone hitKind = iota
	hitFile
	hitType
	hitMessage
	hitDropItem
)

type hit struct {
	kind hitKind
	idx  int // file or dropdown-item index, when relevant
}

// hitTest maps a click to a target. Y selects the row; only on the commit line
// does X split the type box from the message field.
func (m Model) hitTest(x, y int) hit {
	l := m.layout()
	if y >= l.filesStart && y < l.filesStart+l.fileCount {
		if m.logVisible() && x >= m.leftColWidth() {
			return hit{kind: hitNone} // clicked the read-only log column
		}
		return hit{hitFile, y - l.filesStart}
	}
	if m.dropOpen && y >= l.dropStart && y < l.dropStart+len(commit.Types) {
		return hit{hitDropItem, y - l.dropStart}
	}
	if y == l.commitRow {
		if x < typeBoxWidth(commit.Types[m.typeIdx].Name) {
			return hit{kind: hitType}
		}
		return hit{kind: hitMessage}
	}
	return hit{kind: hitNone}
}

// typeBoxWidth is the display width of the leading "▸[ name ▾ ] " on the commit
// line, in columns. Used by both View() and hitTest() to agree on where the
// type box ends and the message begins.
func typeBoxWidth(name string) int {
	// "▸" + "[ " + name + " " + "▾" + " ] " = 1 + 2 + len + 1 + 1 + 3
	return len([]rune(name)) + 8
}

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

// View implements tea.Model. See docs/ux-mockups.html for the target layout.
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

func (m Model) commitLine() string {
	t := commit.Types[m.typeIdx]
	marker := " "
	if m.focus == zoneType {
		marker = "▸"
	}
	arrow := "▾"
	if m.dropOpen {
		arrow = "▴"
	}
	msg := m.message
	if msg == "" {
		msg = "(escribe el mensaje)"
	}
	if m.focus == zoneMessage {
		msg = m.message + "_"
	}
	return fmt.Sprintf("%s[ %s %s ] %s", marker, t.Name, arrow, msg)
}

func (m Model) dropdownView() string {
	var b strings.Builder
	for i, t := range commit.Types {
		sel := " "
		if i == m.dropIdx {
			sel = "▸"
		}
		fmt.Fprintf(&b, "%s %-9s %s\n", sel, t.Name, t.Desc)
	}
	return b.String()
}

func (m Model) helpLine() string {
	switch {
	case m.focus == zoneType && m.dropOpen:
		return "j/k mover · enter elige · esc cierra"
	case m.focus == zoneType:
		return "letra elige tipo · ↓ abre lista · tab mensaje · esc atrás"
	case m.focus == zoneMessage:
		return "escribe · enter commit · esc atrás · tab archivos"
	default:
		return "j/k mover · espacio stage · tab tipo · enter commit · q salir"
	}
}
