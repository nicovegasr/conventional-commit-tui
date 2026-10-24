// Package ui holds the Bubble Tea model for the minimal commit flow:
// mark files → pick type → type message → enter. It depends on a narrow Repo
// port (ADR-0005) so the state transitions are testable without real git.
//
// Phase 2 adds three focus zones (Files → Type → Message), an openable type
// dropdown, and mouse clicks. Picking a type auto-advances to the message so
// the keyboard hot path stays a single `tab` + letter. The type shortcut only
// fires while the Type zone is focused, keeping it predictable.
//
// The right column shows what usually changes with the marked files above the
// git log (ADR-0014), so a forgotten file is visible before committing.
//
// `/` on an empty message opens the command palette; /settings switches the
// heavy parts off live and persists them (ADR-0015). /changes reviews the
// diffs and hands a file to the user's editor (ADR-0017).
package ui

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nicovegasr/nib-git/internal/audit"
	"github.com/nicovegasr/nib-git/internal/commit"
	"github.com/nicovegasr/nib-git/internal/git"
	"github.com/nicovegasr/nib-git/internal/settings"
)

// Repo is the narrow slice of git the commit flow needs. main.go injects a real
// adapter; tests inject a fake.
type Repo interface {
	Status() ([]git.FileChange, error)
	Stage(files []string) error
	Commit(subject string) error
}

// Logger is the narrow slice of git the log panel needs. Optional and read-only
// (ADR-0005): keeping it separate from Repo lets /settings disable it.
type Logger interface {
	Log(scope string, limit int) (string, error)
}

// History is the narrow slice of the history index the related-files panel
// needs (ADR-0014). Refresh brings the index up to HEAD; Related ranks what
// usually changes with files and fails with ErrStaleHistory when HEAD moved.
type History interface {
	Refresh() error
	Related(files []string, limit int) ([]Related, error)
}

// Store persists the /settings options (ADR-0015).
type Store interface {
	Load() (settings.Settings, error)
	Save(settings.Settings) error
}

// Related is a path that historically changed together with a marked file.
type Related struct {
	Path  string
	Ratio float64 // share of that file's commits that also touched Path
}

// ErrStaleHistory reports an index behind HEAD, e.g. after a commit made
// outside nib.
var ErrStaleHistory = errors.New("history index is behind HEAD")

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

// pollMsg asks for a background reload of the working tree and the log. Ticks
// of a superseded interval (gen) are dropped.
type pollMsg struct{ gen int }

// settingsSavedMsg reports the outcome of persisting the settings.
type settingsSavedMsg struct{ err error }

// statusLoadedMsg carries a background working-tree reload.
type statusLoadedMsg struct {
	files []git.FileChange
	err   error
}

// historyRefreshedMsg reports that the index caught up with HEAD, or why not.
type historyRefreshedMsg struct{ err error }

// relatedLoadedMsg answers the query numbered gen.
type relatedLoadedMsg struct {
	gen     int
	related []Related
	err     error
}

// Model is the root application state.
type Model struct {
	repo     Repo
	files    []git.FileChange
	staged   map[string]bool // path -> staged in this session
	cursor   int
	fileTop  int    // first file row shown; the list scrolls to keep the cursor visible
	typeIdx  int    // index into commit.Types
	message  string // commit message being typed
	focus    zone   // which zone owns the keyboard
	dropOpen bool   // type dropdown expanded
	dropIdx  int    // highlighted item while the dropdown is open
	flash    string // transient feedback (success, hint or failure), cleared on the next keystroke
	err      error

	width  int
	height int

	store        Store
	prefs        settings.Settings
	pollGen      int  // numbers reload loops; a changed interval starts a new one
	palette      bool // command palette open; its query is typed in the message field
	paletteQuery string
	paletteIdx   int
	settingsOpen bool // /settings screen replaces the commit view
	settingsIdx  int  // highlighted settings row

	auditor      Auditor
	auditOpen    bool // /audit screen replaces the commit view
	auditDays    int  // window shown; starts at the /settings default
	auditGen     int  // numbers audits; answers for an older window are dropped
	auditLoading bool
	auditReport  audit.Report
	auditErr     error
	auditTop     int // first hotspot row shown (scroll)

	differ        Differ
	editor        Editor
	changesOpen   bool   // /changes screen replaces the commit view
	changesPath   string // file whose diff is shown
	changesQuery  string // search box filter
	changesSearch bool   // typing in the search box
	diffFocus     bool   // j/k scroll the diff instead of moving the file
	diff          git.Diff
	diffErr       error
	diffLoading   bool
	diffGen       int // numbers diffs; answers for another file are dropped
	diffTop       int // first diff line shown (scroll)
	diffDragging  bool

	logger     Logger
	logContent string
	logErr     error
	logLoading bool

	history      History
	historyBusy  bool // index refresh in flight; queries wait for it
	historyErr   error
	related      []Related
	relatedErr   error
	relatedGen   int  // numbers queries; answers to older ones are dropped
	staleRetried bool // one refresh per stale answer, never a loop

	shortcuts map[string]int // type shortcut key -> index into commit.Types
}

// New builds the initial model, loading the settings through store and the
// working tree through repo. Unreadable settings fall back to the defaults.
func New(repo Repo, logger Logger, history History, store Store, auditor Auditor, differ Differ, editor Editor) Model {
	prefs, prefsErr := store.Load()
	m := Model{
		repo:        repo,
		logger:      logger,
		store:       store,
		auditor:     auditor,
		differ:      differ,
		editor:      editor,
		prefs:       prefs,
		logLoading:  prefs.ShowLog, // Init fetches it
		history:     history,
		historyBusy: true, // Init refreshes it
		shortcuts:   typeShortcuts(),
	}
	if prefsErr != nil {
		m.flash = "✗ ajustes ilegibles, usando los de serie: " + lastLine(prefsErr.Error())
	}
	files, err := repo.Status()
	if err != nil {
		m.err = err
		return m
	}
	return m.withFiles(files)
}

// withFiles replaces the file list. Marks survive a reload: git's staged
// state only decides a mark for a new file or when it changed (an external
// `git add`, a failed commit), so polling never undoes the user's own marks.
// The cursor follows its file when it is still there.
func (m Model) withFiles(files []git.FileChange) Model {
	previous := map[string]git.FileChange{}
	for _, f := range m.files {
		previous[f.Path] = f
	}
	var cursorPath string
	if m.cursor < len(m.files) {
		cursorPath = m.files[m.cursor].Path
	}
	staged := map[string]bool{}
	m.cursor = 0
	for i, f := range files {
		staged[f.Path] = f.Staged
		if p, known := previous[f.Path]; known && p.Staged == f.Staged {
			staged[f.Path] = m.staged[f.Path]
		}
		if f.Path == cursorPath {
			m.cursor = i
		}
	}
	m.files, m.staged = files, staged
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

// loadLog fetches the git log off the UI goroutine; nothing while its panel is
// off in /settings.
func (m Model) loadLog() tea.Cmd {
	if !m.prefs.ShowLog {
		return nil
	}
	logger, scope := m.logger, m.prefs.LogScope
	return func() tea.Msg {
		out, err := logger.Log(scope, logFetchLimit)
		return logLoadedMsg{content: out, err: err}
	}
}

// poll schedules the next background reload, so edits, `git add` and commits
// made outside nib show up without restarting it. Auto reload off: no tick.
func (m Model) poll() tea.Cmd {
	if m.prefs.ReloadSeconds == 0 {
		return nil
	}
	gen := m.pollGen
	return tea.Tick(time.Duration(m.prefs.ReloadSeconds)*time.Second, func(time.Time) tea.Msg { return pollMsg{gen: gen} })
}

// polled reloads the working tree, the log and an open diff, then schedules
// the next tick. Ticks of a superseded interval are dropped.
func (m Model) polled(msg pollMsg) (Model, tea.Cmd) {
	if msg.gen != m.pollGen {
		return m, nil
	}
	m, diff := m.reloadDiff()
	return m, tea.Batch(m.loadStatus(), m.loadLog(), m.poll(), diff)
}

func (m Model) loadStatus() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		files, err := repo.Status()
		return statusLoadedMsg{files: files, err: err}
	}
}

// statusLoaded applies a background reload. A failed one keeps the list; the
// related files are asked again only when the marked set changed.
func (m Model) statusLoaded(msg statusLoadedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	before := m.stagedFiles()
	m = m.withFiles(msg.files)
	if slices.Equal(before, m.stagedFiles()) {
		return m, nil
	}
	return m.queryRelated()
}

// logLoaded stores the log. A log that changed while nib was not committing
// means a commit, checkout or rebase happened outside: the index catches up.
func (m Model) logLoaded(msg logLoadedMsg) (Model, tea.Cmd) {
	moved := msg.err == nil && m.logContent != "" && msg.content != m.logContent
	m.logLoading, m.logContent, m.logErr = false, msg.content, msg.err
	if !moved || m.historyBusy {
		return m, nil
	}
	m.historyBusy, m.staleRetried = true, false
	return m, m.refreshHistory()
}

const relatedLimit = 5

// refreshHistory brings the index up to HEAD off the UI goroutine. Callers set
// historyBusy so that no query races the writer.
func (m Model) refreshHistory() tea.Cmd {
	history := m.history
	return func() tea.Msg { return historyRefreshedMsg{err: history.Refresh()} }
}

// queryRelated asks what usually changes with the marked files. With nothing
// marked it clears the panel; while the index refreshes it waits, and the
// refresh queries when done.
func (m Model) queryRelated() (Model, tea.Cmd) {
	marked := m.stagedFiles()
	m.relatedGen++
	if len(marked) == 0 {
		m.related, m.relatedErr = nil, nil
		return m, nil
	}
	if !m.prefs.ShowRelated || m.historyBusy || m.historyErr != nil {
		return m, nil
	}
	history, gen := m.history, m.relatedGen
	return m, func() tea.Msg {
		related, err := history.Related(marked, relatedLimit)
		return relatedLoadedMsg{gen: gen, related: related, err: err}
	}
}

// relatedLoaded stores the answer to the latest query. A stale index (a commit
// made outside nib) is refreshed once and queried again.
func (m Model) relatedLoaded(msg relatedLoadedMsg) (Model, tea.Cmd) {
	if msg.gen != m.relatedGen {
		return m, nil
	}
	if errors.Is(msg.err, ErrStaleHistory) && !m.staleRetried {
		m.staleRetried, m.historyBusy = true, true
		return m, m.refreshHistory()
	}
	m.related, m.relatedErr = msg.related, msg.err
	if msg.err == nil {
		m.staleRetried = false
	}
	return m, nil
}

// Init implements tea.Model. The mouse is enabled here, from the settings.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadLog(), m.refreshHistory(), m.poll()}
	if m.prefs.MouseCapture {
		cmds = append(cmds, tea.EnableMouseCellMotion)
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model. Whatever moved the cursor, resized the
// terminal or reloaded the files, the list then scrolls to keep the cursor in
// view.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if model, ok := next.(Model); ok {
		next = model.followCursor()
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case commitDoneMsg:
		return m.afterCommit(msg.err)
	case logLoadedMsg:
		return m.logLoaded(msg)
	case pollMsg:
		return m.polled(msg)
	case settingsSavedMsg:
		if msg.err != nil {
			m.flash = "✗ no se pudo guardar: " + lastLine(msg.err.Error())
		}
		return m, nil
	case statusLoadedMsg:
		return m.statusLoaded(msg)
	case historyRefreshedMsg:
		m.historyBusy, m.historyErr = false, msg.err
		return m.queryRelated()
	case relatedLoadedMsg:
		return m.relatedLoaded(msg)
	case auditLoadedMsg:
		return m.auditLoaded(msg), nil
	case diffLoadedMsg:
		return m.diffLoaded(msg), nil
	case editorClosedMsg:
		return m.editorClosed(msg)
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
	switch {
	case m.changesOpen:
		return m.updateChanges(key)
	case m.auditOpen:
		return m.updateAudit(key)
	case m.settingsOpen:
		return m.updateSettings(key)
	case m.palette:
		return m.updatePalette(key)
	case key.String() == "/" && m.message == "" && !m.dropOpen:
		return m.openPalette(), nil
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
			return m.queryRelated()
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
		m.logLoading, m.historyBusy = true, true
		cmd = tea.Batch(m.loadLog(), m.refreshHistory())
	}
	files, statusErr := m.repo.Status()
	if statusErr != nil {
		m.flash += " · no se pudo refrescar la lista"
		return m, cmd
	}
	m, query := m.withFiles(files).queryRelated()
	return m, tea.Batch(cmd, query)
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
// scrolls the open dropdown. With mouse capture off in /settings no mouse
// message arrives at all.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.changesOpen:
		return m.clickChanges(msg)
	case m.auditOpen:
		return m.clickAudit(msg)
	case m.settingsOpen:
		return m.clickSettings(msg)
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		return m.scroll(msg.Button == tea.MouseButtonWheelUp), nil
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
		return m.queryRelated()
	case hitType:
		m.focus = zoneType
		m.dropOpen = !m.dropOpen
		m.dropIdx = m.typeIdx
	case hitDropItem:
		return m.pickType(h.idx), nil
	case hitMessage:
		m.focus = zoneMessage
	case hitPaletteItem:
		m.paletteIdx = h.idx
		return m.runCommand()
	case hitCommands:
		return m.openPalette(), nil
	case hitNone:
		m.dropOpen, m.palette = false, false
	}
	return m, nil
}

// scroll moves one item with the wheel: the open dropdown's highlight, else
// the file cursor.
func (m Model) scroll(up bool) Model {
	step := 1
	if up {
		step = -1
	}
	if m.dropOpen {
		m.dropIdx = min(max(m.dropIdx+step, 0), len(commit.Types)-1)
		return m
	}
	m.cursor = max(min(m.cursor+step, len(m.files)-1), 0)
	return m
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

// logVisible reports whether the right column (related files + git log) should
// be drawn: enabled and the terminal is wide enough that the minimal flow is
// not squeezed.
func (m Model) logVisible() bool {
	return m.prefs.ShowLog && m.width >= minWidthForLog
}

// leftColWidth splits the two-column band in half: files left, log right.
func (m Model) leftColWidth() int {
	return m.width / 2
}

const reservedBelow = 5 // blank + commit line + preview + blank + help line under the band

// belowRows is how many rows the open dropdown or palette adds under the
// preview line.
func (m Model) belowRows() int {
	switch {
	case m.dropOpen:
		return len(commit.Types)
	case m.palette:
		return max(len(m.paletteMatches()), 1) // "sin comandos" when nothing matches
	}
	return 0
}

// logRows is how many rows the log column may fill. It is derived from the
// terminal height and MUST leave room for everything drawn below the band —
// including the open dropdown — plus a one-row safety margin. If the whole frame
// ever exceeds the terminal height, Bubble Tea cannot clear it cleanly: the
// dropdown ghosts and mouse clicks land on the wrong rows.
func (m Model) logRows() int {
	r := m.height - headerRows - reservedBelow - 1 // -1: never fill the last row
	r -= m.belowRows()                             // an open dropdown or palette needs its own rows below the commit line
	if r < 0 {
		return 0
	}
	return r
}

// fileRows is how many files fit in the band; all of them while the terminal
// size is not known yet.
func (m Model) fileRows() int {
	if m.height == 0 {
		return max(len(m.files), 1)
	}
	return max(m.logRows(), 1)
}

// followCursor scrolls the file list just enough to show the cursor.
func (m Model) followCursor() Model {
	rows := m.fileRows()
	if m.cursor < m.fileTop {
		m.fileTop = m.cursor
	}
	if m.cursor >= m.fileTop+rows {
		m.fileTop = m.cursor - rows + 1
	}
	m.fileTop = min(max(m.fileTop, 0), max(len(m.files)-rows, 0))
	return m
}

type layout struct {
	filesStart int // row of the first visible file
	fileCount  int // visible files
	bandHeight int // rows the two-column (files ‖ log) band occupies
	commitRow  int // the "[ type ▾ ] message" line (type and message share it)
	dropStart  int // first dropdown or palette item row (only meaningful when open)
	footerRow  int // flash + help line, led by the "/ comandos" button
}

func (m Model) layout() layout {
	n := min(len(m.files)-m.fileTop, m.fileRows())
	band := max(n, 1) // a clean tree still gets its one-line notice
	if m.logVisible() {
		band = max(band, m.logRows())
	}
	commitRow := headerRows + band + 1 // one blank line between the band and the commit line
	return layout{
		filesStart: headerRows,
		fileCount:  n,
		bandHeight: band,
		commitRow:  commitRow,
		dropStart:  commitRow + 2, // below the preview line
		footerRow:  commitRow + 2 + m.belowRows() + 1,
	}
}

type hitKind int

const (
	hitNone hitKind = iota
	hitFile
	hitType
	hitMessage
	hitDropItem
	hitPaletteItem
	hitCommands
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
			return hit{kind: hitNone} // clicked the read-only right column
		}
		return hit{hitFile, m.fileTop + y - l.filesStart}
	}
	if m.dropOpen && y >= l.dropStart && y < l.dropStart+len(commit.Types) {
		return hit{hitDropItem, y - l.dropStart}
	}
	if m.palette && y >= l.dropStart && y < l.dropStart+len(m.paletteMatches()) {
		return hit{hitPaletteItem, y - l.dropStart}
	}
	if y == l.footerRow && x <= lipgloss.Width(commandsButton) {
		return hit{kind: hitCommands}
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

// --- rendering --------------------------------------------------------------
//
// Colors follow docs/ux-mockups.html. Text is truncated while still plain and
// styled afterwards, so widths are always measured on what the user sees.

var (
	green   = lipgloss.Color("#3fb950")
	yellow  = lipgloss.Color("#d29922")
	red     = lipgloss.Color("#f85149")
	cyan    = lipgloss.Color("#39c5cf")
	magenta = lipgloss.Color("#bc8cff")
	gray    = lipgloss.Color("240")
	faint   = lipgloss.Color("237")

	dimStyle = lipgloss.NewStyle().Foreground(gray)
	bold     = lipgloss.NewStyle().Bold(true)
)

func dim(s string) string { return dimStyle.Render(s) }
func paint(c lipgloss.TerminalColor, s string) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}

// statusColor colors the porcelain status letter like the mockups.
func statusColor(status rune) lipgloss.TerminalColor {
	switch status {
	case 'M':
		return yellow
	case 'A':
		return green
	case 'D':
		return red
	case 'R', 'C':
		return magenta
	default:
		return cyan
	}
}

// truncate hard-caps a plain string to w display columns with an ellipsis.
// A non-positive w means unbounded (the size is not known yet).
func truncate(s string, w int) string {
	r := []rune(s)
	if w <= 0 || len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// padRight fills a styled string with spaces up to w visible columns.
func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// sectionTitle is a panel header: a dim upper-case label and a fainter hint.
func sectionTitle(label, hint string, w int) string {
	hint = truncate(hint, w-len([]rune(label))-3)
	return dimStyle.Bold(true).Render(label) + paint(faint, " · "+hint)
}

// header counts the marks and, when the list scrolls, which rows are shown.
func (m Model) header() string {
	marked := len(m.stagedFiles())
	counts := fmt.Sprintf("%s de %s", count(marked, "marcado", "marcados"), count(len(m.files), "archivo", "archivos"))
	if shown := m.layout().fileCount; shown < len(m.files) {
		counts += fmt.Sprintf(" · viendo %d–%d", m.fileTop+1, m.fileTop+shown)
	}
	return fmt.Sprintf("  %s  %s %s", paint(green, bold.Render("nib")), dimStyle.Bold(true).Render("CAMBIOS"), dim(counts))
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// fileLines renders the working tree, w columns wide. Unmarked files that
// usually change with the marked ones carry a yellow ↔.
func (m Model) fileLines(w int) []string {
	if len(m.files) == 0 {
		return []string{dim("  ✓ árbol limpio, nada que commitear")}
	}
	related := map[string]bool{}
	for _, r := range m.related {
		related[r.Path] = true
	}
	lines := make([]string, 0, len(m.files))
	for i := m.fileTop; i < m.fileTop+m.layout().fileCount; i++ {
		f := m.files[i]
		cursor := m.focus == zoneFiles && i == m.cursor
		marker, mark := " ", paint(faint, "○")
		if cursor {
			marker = paint(green, "▸")
		}
		if m.staged[f.Path] {
			mark = paint(green, "✓")
		}
		hint := ""
		if related[f.Path] && !m.staged[f.Path] {
			hint = paint(yellow, " ↔")
		}
		path := truncate(f.Path, w-6-lipgloss.Width(hint))
		if cursor {
			path = bold.Render(path)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %s%s", marker, mark, paint(statusColor(f.Status), string(f.Status)), path, hint))
	}
	return lines
}

// relatedLines is the RELACIONADOS section, w columns wide, or nil while
// nothing is marked. "+" flags a modified file that is not marked yet: the
// likely omission.
func (m Model) relatedLines(w int) []string {
	if !m.prefs.ShowRelated || len(m.stagedFiles()) == 0 {
		return nil
	}
	title := sectionTitle("RELACIONADOS", "suelen cambiar con lo marcado", w)
	switch {
	case m.historyErr != nil || m.relatedErr != nil:
		return []string{title, dim("historial no disponible")}
	case m.historyBusy:
		return []string{title, dim("indexando historial…")}
	case len(m.related) == 0:
		return []string{title, dim("sin cambios compartidos en el historial")}
	}
	unmarked := map[string]bool{}
	for _, f := range m.files {
		unmarked[f.Path] = !m.staged[f.Path]
	}
	lines := []string{title}
	for _, r := range m.related {
		pct := fmt.Sprintf("%3.0f%%", r.Ratio*100)
		path := truncate(r.Path, w-3-len(pct))
		row := paint(faint, "·") + " " + dim(path)
		if unmarked[r.Path] {
			row = paint(yellow, "+") + " " + path
		}
		lines = append(lines, padRight(row, w-len(pct)-1)+" "+dim(pct))
	}
	return lines
}

// logLine colors one `git log --oneline --decorate` line: graph, hash, refs.
var logLine = regexp.MustCompile(`^([*|\\/_ ]*)([0-9a-f]{7,40})( \([^)]*\))?(.*)$`)

func colorLog(line string) string {
	g := logLine.FindStringSubmatch(line)
	if g == nil {
		return line
	}
	return paint(red, g[1]) + paint(yellow, g[2]) + paint(magenta, g[3]) + g[4]
}

// logBody returns the log lines or a single dimmed status line.
func (m Model) logBody(w int) []string {
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
	var lines []string
	for _, line := range strings.Split(trimmed, "\n") {
		lines = append(lines, colorLog(truncate(line, w)))
	}
	return lines
}

// rightLines stacks RELACIONADOS over the git log, cropped to the band.
func (m Model) rightLines(band, w int) []string {
	scope := scopeNames[m.prefs.LogScope]
	var lines []string
	if related := m.relatedLines(w); related != nil {
		lines = append(related, "")
	}
	lines = append(lines, sectionTitle("GIT LOG", scope, w))
	lines = append(lines, m.logBody(w)...)
	return lines[:min(len(lines), band)]
}

// View implements tea.Model. See docs/ux-mockups.html for the target layout.
func (m Model) View() string {
	if m.err != nil {
		return "nib: " + m.err.Error() + "\n"
	}
	switch {
	case m.changesOpen:
		return m.changesView()
	case m.auditOpen:
		return m.auditView()
	case m.settingsOpen:
		return m.settingsView()
	}

	var b strings.Builder
	b.WriteString(m.header() + "\n\n")

	l := m.layout()
	if m.logVisible() {
		leftW := m.leftColWidth()
		rightW := m.width - leftW - 3 // " │ "
		left, right := m.fileLines(leftW), m.rightLines(l.bandHeight, rightW)
		for i := 0; i < l.bandHeight; i++ {
			lc, rc := "", ""
			if i < len(left) {
				lc = left[i]
			}
			if i < len(right) {
				rc = right[i]
			}
			fmt.Fprintf(&b, "%s %s %s\n", padRight(lc, leftW), paint(faint, "│"), rc)
		}
	} else {
		for _, line := range m.fileLines(m.width) {
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n" + m.commitLine() + "\n" + m.previewLine() + "\n")
	if m.dropOpen {
		b.WriteString(m.dropdownView())
	}
	if m.palette {
		b.WriteString(m.paletteView())
	}
	fmt.Fprintf(&b, "\n %s\n", m.footer())
	return b.String()
}

func (m Model) commitLine() string {
	t := commit.Types[m.typeIdx]
	marker, frame := " ", faint
	if m.focus == zoneType {
		marker, frame = paint(green, "▸"), green
	}
	arrow := "▾"
	if m.dropOpen {
		arrow = "▴"
	}
	msg := paint(faint, "escribe el mensaje…")
	switch {
	case m.palette:
		msg = paint(cyan, "/"+m.paletteQuery) + paint(green, "▌")
	case m.focus == zoneMessage:
		msg = m.message + paint(green, "▌")
	case m.message != "":
		msg = m.message
	}
	box := paint(frame, "[ ") + paint(green, bold.Render(t.Name)) + " " + dim(arrow) + paint(frame, " ]")
	return marker + box + " " + msg
}

// previewLine shows the exact subject enter will commit, or the next step.
func (m Model) previewLine() string {
	marked := len(m.stagedFiles())
	switch {
	case marked == 0:
		return dim("  marca archivos con espacio o click")
	case strings.TrimSpace(m.message) == "":
		return dim("  " + count(marked, "archivo listo", "archivos listos") + " · escribe el mensaje")
	}
	subject := commit.Format(commit.Types[m.typeIdx].Prefix, m.message)
	return dim("  → ") + subject + dim("  · "+count(marked, "archivo", "archivos"))
}

func (m Model) dropdownView() string {
	var b strings.Builder
	for i, t := range commit.Types {
		name := fmt.Sprintf("%-9s", t.Name)
		if i == m.dropIdx {
			fmt.Fprintf(&b, "%s %s %s\n", paint(green, "▸"), paint(green, bold.Render(name)), t.Desc)
			continue
		}
		fmt.Fprintf(&b, "  %s %s\n", name, dim(t.Desc))
	}
	return b.String()
}

// footer is the "/ comandos" button, the transient flash colored by outcome,
// and the keys of the focused zone. The button leads so hitTest finds it at a
// fixed column.
func (m Model) footer() string {
	var keys []string
	for _, pair := range m.helpKeys() {
		keys = append(keys, bold.Render(pair[0])+" "+dim(pair[1]))
	}
	line := ""
	if !m.changesOpen && !m.auditOpen && !m.settingsOpen { // the palette opens from the commit view only
		line = paint(cyan, commandsButton) + "  "
	}
	switch {
	case m.flash == "":
	case strings.HasPrefix(m.flash, "✓"):
		line += paint(green, m.flash) + "   "
	case strings.HasPrefix(m.flash, "✗"):
		line += paint(red, m.flash) + "   "
	default:
		line += paint(yellow, m.flash) + "   "
	}
	return line + strings.Join(keys, dim(" · "))
}

func (m Model) helpKeys() [][2]string {
	switch {
	case m.changesOpen && m.changesSearch:
		return [][2]string{{"escribe", "filtra"}, {"enter", "listo"}}
	case m.changesOpen:
		return [][2]string{{"j/k", "mover"}, {"tab", "panel"}, {"n/p", "cambio"}, {"espacio", "marca"}, {"e", "editar"}, {"/", "buscar"}, {"esc", "vuelve"}}
	case m.auditOpen:
		return [][2]string{{"d", "ventana"}, {"j/k", "mover"}, {"esc", "vuelve"}}
	case m.settingsOpen:
		return [][2]string{{"j/k", "mover"}, {"espacio", "cambia"}, {"esc", "vuelve"}}
	case m.palette:
		return [][2]string{{"↑/↓", "mover"}, {"enter", "abre"}, {"esc", "cierra"}}
	case m.focus == zoneType && m.dropOpen:
		return [][2]string{{"j/k", "mover"}, {"enter", "elige"}, {"esc", "cierra"}}
	case m.focus == zoneType:
		return [][2]string{{"letra", "elige tipo"}, {"↓", "abre lista"}, {"tab", "mensaje"}, {"esc", "atrás"}}
	case m.focus == zoneMessage:
		return [][2]string{{"enter", "commit"}, {"esc", "atrás"}, {"tab", "archivos"}}
	default:
		return [][2]string{{"j/k", "mover"}, {"espacio", "marca"}, {"tab", "tipo"}, {"enter", "commit"}, {"q", "salir"}}
	}
}
