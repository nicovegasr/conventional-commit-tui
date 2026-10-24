package ui

// The command palette and the /settings screen (ADR-0015).

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/nib-git/internal/settings"
)

const commandsButton = "/ comandos"

var scopeNames = map[string]string{settings.ScopeCurrent: "rama actual", settings.ScopeAll: "todas las ramas"}

type command struct{ name, desc string }

// commands is the palette's list; /audit and /stats join it in later phases.
var commands = []command{
	{"settings", "paneles, ratón, alcance del log, recarga"},
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

// settingRow is one line of /settings; rows are changed by index in change.
type settingRow struct {
	label, hint string
	value       func(settings.Settings) string
}

func onOff(on bool) string {
	if on {
		return "ON ●"
	}
	return "OFF ○"
}

var settingRows = []settingRow{
	{"Panel git log", "apágalo si va lento", func(s settings.Settings) string { return onOff(s.ShowLog) }},
	{"Archivos relacionados", "lo que suele cambiar con lo marcado", func(s settings.Settings) string { return onOff(s.ShowRelated) }},
	{"Mouse capture", "off = selección y scroll nativos del terminal", func(s settings.Settings) string { return onOff(s.MouseCapture) }},
	{"git log: alcance", "rama actual / todas las ramas", func(s settings.Settings) string { return scopeNames[s.LogScope] + " ▾" }},
	{"Recarga automática", "cada cuánto se relee el árbol", func(s settings.Settings) string {
		if s.ReloadSeconds == 0 {
			return "off ▾"
		}
		return fmt.Sprintf("%d s ▾", s.ReloadSeconds)
	}},
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

// change toggles or cycles row, applies it live and saves it.
func (m Model) change(row int) (Model, tea.Cmd) {
	var apply tea.Cmd
	switch row {
	case 0:
		m.prefs.ShowLog = !m.prefs.ShowLog
		m.logLoading = m.prefs.ShowLog
		apply = m.loadLog()
	case 1:
		m.prefs.ShowRelated = !m.prefs.ShowRelated
		m.related, m.relatedErr = nil, nil
		m, apply = m.queryRelated()
	case 2:
		m.prefs.MouseCapture = !m.prefs.MouseCapture
		apply = tea.DisableMouse
		if m.prefs.MouseCapture {
			apply = tea.EnableMouseCellMotion
		}
	case 3:
		if m.prefs.LogScope == settings.ScopeAll {
			m.prefs.LogScope = settings.ScopeCurrent
		} else {
			m.prefs.LogScope = settings.ScopeAll
		}
		m.logLoading = m.prefs.ShowLog
		apply = m.loadLog()
	case 4:
		m.prefs.ReloadSeconds = settings.NextReload(m.prefs.ReloadSeconds)
		m.pollGen++
		apply = m.poll()
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
		if strings.HasPrefix(value, "ON") {
			color = green
		} else if strings.HasPrefix(value, "OFF") {
			color = gray
		}
		fmt.Fprintf(&b, "%s %s %s  %s\n", marker, padRight(label, 24), padRight(paint(color, value), 18), dim(r.hint))
	}
	fmt.Fprintf(&b, "\n %s\n", m.footer())
	return b.String()
}
