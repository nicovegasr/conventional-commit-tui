package ui

// The command palette and the /settings screen (ADR-0015).

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/conventional-commit-tui/internal/settings"
)

const commandsButton = "/ comandos"

var scopeNames = map[string]string{settings.ScopeCurrent: "rama actual", settings.ScopeAll: "todas las ramas"}

type command struct{ name, desc string }

// commands is the palette's list.
var commands = []command{
	{"audit", "archivos que más cambian (hotspots)"},
	{"settings", "paneles, ratón, alcance del log, recarga, audit"},
	{"changes", "diff de cada archivo, y abrirlo en tu editor"},
	{"stats", "commits por tipo y actividad"},
}

func (m Model) openPalette() Model {
	m.palette, m.paletteQuery, m.paletteIdx, m.dropOpen = true, "", 0, false
	return m
}

// paletteMatches are the commands whose name contains the typed query.
func (m Model) paletteMatches() []command {
	var matches []command
	for _, c := range commands {
		if strings.Contains(c.name, m.paletteQuery) {
			matches = append(matches, c)
		}
	}
	return matches
}

// updatePalette types the query and picks a command. Backspace on an empty
// query closes it, like deleting the `/`.
func (m Model) updatePalette(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.palette = false
	case "enter":
		return m.runCommand()
	case "up":
		m.paletteIdx = max(m.paletteIdx-1, 0)
	case "down":
		m.paletteIdx = min(m.paletteIdx+1, max(len(m.paletteMatches())-1, 0))
	case "backspace":
		if r := []rune(m.paletteQuery); len(r) > 0 {
			m.paletteQuery, m.paletteIdx = string(r[:len(r)-1]), 0
		} else {
			m.palette = false
		}
	default:
		if text := typedText(key); text != "" {
			m.paletteQuery, m.paletteIdx = m.paletteQuery+strings.TrimSpace(text), 0
		}
	}
	return m, nil
}

// runCommand runs the highlighted match; with no match the palette stays open.
func (m Model) runCommand() (Model, tea.Cmd) {
	matches := m.paletteMatches()
	if m.paletteIdx >= len(matches) {
		return m, nil
	}
	m.palette = false
	switch matches[m.paletteIdx].name {
	case "stats":
		return m.openStats()
	case "changes":
		return m.openChanges()
	case "audit":
		return m.openAudit()
	case "settings":
		m.settingsOpen, m.settingsIdx = true, 0
	}
	return m, nil
}

func (m Model) paletteView() string {
	matches := m.paletteMatches()
	if len(matches) == 0 {
		return dim("  sin comandos") + "\n"
	}
	var b strings.Builder
	for i, c := range matches {
		name := fmt.Sprintf("/%-9s", c.name)
		if i == m.paletteIdx {
			fmt.Fprintf(&b, "%s %s %s\n", paint(green, "▸"), paint(cyan, bold.Render(name)), c.desc)
			continue
		}
		fmt.Fprintf(&b, "  %s %s\n", paint(cyan, name), dim(c.desc))
	}
	return b.String()
}

type rowID int

const (
	rowLog rowID = iota
	rowScope
	rowRelated
	rowMouse
	rowReload
	rowAuditDays
	rowThreshold
)

// settingRow is one line of /settings. A row with a parent belongs to it: it
// is drawn under it and only changes while the parent is on.
type settingRow struct {
	id          rowID
	label, hint string
	value       func(settings.Settings) string
	parent      func(settings.Settings) bool
}

func (r settingRow) enabled(s settings.Settings) bool { return r.parent == nil || r.parent(s) }

func onOff(on bool) string {
	if on {
		return "ON ●"
	}
	return "OFF ○"
}

var settingRows = []settingRow{
	{rowLog, "Panel git log", "apágalo si va lento", func(s settings.Settings) string { return onOff(s.ShowLog) }, nil},
	{rowScope, "└ alcance", "rama actual / todas las ramas", func(s settings.Settings) string { return scopeNames[s.LogScope] + " ▾" },
		func(s settings.Settings) bool { return s.ShowLog }},
	{rowRelated, "Archivos relacionados", "lo que suele cambiar con lo marcado", func(s settings.Settings) string { return onOff(s.ShowRelated) }, nil},
	{rowMouse, "Mouse capture", "off = selección y scroll nativos del terminal", func(s settings.Settings) string { return onOff(s.MouseCapture) }, nil},
	{rowReload, "Recarga automática", "cada cuánto se relee el árbol", func(s settings.Settings) string {
		if s.ReloadSeconds == 0 {
			return "off ▾"
		}
		return fmt.Sprintf("%d s ▾", s.ReloadSeconds)
	}, nil},
	{rowAuditDays, "/audit · ventana", "días que mira al abrirse", func(s settings.Settings) string { return fmt.Sprintf("%d días ▾", s.AuditDays) }, nil},
	{rowThreshold, "/audit · umbral", "commits que marcan refactor?", func(s settings.Settings) string { return fmt.Sprintf("≥ %d ▾", s.AuditThreshold) }, nil},
}

func (m Model) updateSettings(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "j", "down":
		m.settingsIdx = min(m.settingsIdx+1, len(settingRows)-1)
	case "k", "up":
		m.settingsIdx = max(m.settingsIdx-1, 0)
	case " ", "enter":
		return m.change(m.settingsIdx)
	case "esc", "q":
		m.settingsOpen = false
	}
	return m, nil
}

// change toggles or cycles row, applies it live and saves it. A row whose
// parent is off does not change.
func (m Model) change(row int) (Model, tea.Cmd) {
	r := settingRows[row]
	if !r.enabled(m.prefs) {
		return m, nil
	}
	var apply tea.Cmd
	switch r.id {
	case rowLog:
		m.prefs.ShowLog = !m.prefs.ShowLog
		m.logLoading = m.prefs.ShowLog
		apply = m.loadLog()
	case rowRelated:
		m.prefs.ShowRelated = !m.prefs.ShowRelated
		m.related, m.relatedErr = nil, nil
		m, apply = m.queryRelated()
	case rowMouse:
		m.prefs.MouseCapture = !m.prefs.MouseCapture
		apply = tea.DisableMouse
		if m.prefs.MouseCapture {
			apply = tea.EnableMouseCellMotion
		}
	case rowScope:
		if m.prefs.LogScope == settings.ScopeAll {
			m.prefs.LogScope = settings.ScopeCurrent
		} else {
			m.prefs.LogScope = settings.ScopeAll
		}
		m.logLoading = true
		apply = m.loadLog()
	case rowReload:
		m.prefs.ReloadSeconds = settings.Next(settings.ReloadChoices, m.prefs.ReloadSeconds)
		m.pollGen++
		apply = m.poll()
	case rowAuditDays:
		m.prefs.AuditDays = settings.Next(settings.AuditDayChoices, m.prefs.AuditDays)
	case rowThreshold:
		m.prefs.AuditThreshold = settings.Next(settings.ThresholdChoices, m.prefs.AuditThreshold)
	}
	store, prefs := m.store, m.prefs
	save := func() tea.Msg { return settingsSavedMsg{err: store.Save(prefs)} }
	return m, tea.Batch(apply, save)
}

// clickSettings changes the clicked row.
func (m Model) clickSettings(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	row := msg.Y - headerRows
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft || row < 0 || row >= len(settingRows) {
		return m, nil
	}
	m.settingsIdx = row
	return m.change(row)
}

func (m Model) settingsView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s\n\n", paint(green, bold.Render("nib")), paint(cyan, bold.Render("/settings")))
	for i, r := range settingRows {
		marker, label := " ", r.label
		if i == m.settingsIdx {
			marker, label = paint(green, "▸"), bold.Render(label)
		}
		value := r.value(m.prefs)
		color := cyan
		switch {
		case !r.enabled(m.prefs), strings.HasPrefix(value, "OFF"):
			color = gray
		case strings.HasPrefix(value, "ON"):
			color = green
		}
		fmt.Fprintf(&b, "%s %s %s  %s\n", marker, padRight(label, 24), padRight(paint(color, value), 18), dim(r.hint))
	}
	fmt.Fprintf(&b, "\n %s\n", m.footer())
	return b.String()
}
