package ui

// The /audit screen: files ranked by how many commits touched them (ADR-0016).

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/nib-git/internal/audit"
	"github.com/nicovegasr/nib-git/internal/settings"
)

// Auditor is the narrow slice of the audit package the /audit screen needs.
type Auditor interface {
	Audit(days int) (audit.Report, error)
}

// auditLoadedMsg answers the audit numbered gen.
type auditLoadedMsg struct {
	gen    int
	report audit.Report
	err    error
}

// openAudit shows /audit over the default window from /settings.
func (m Model) openAudit() (Model, tea.Cmd) {
	m.auditOpen, m.auditDays = true, m.prefs.AuditDays
	return m.loadAudit()
}

// loadAudit reads the window off the UI goroutine; answers to an older window
// are dropped.
func (m Model) loadAudit() (Model, tea.Cmd) {
	m.auditGen++
	m.auditLoading, m.auditTop = true, 0
	auditor, days, gen := m.auditor, m.auditDays, m.auditGen
	return m, func() tea.Msg {
		report, err := auditor.Audit(days)
		return auditLoadedMsg{gen: gen, report: report, err: err}
	}
}

func (m Model) auditLoaded(msg auditLoadedMsg) Model {
	if msg.gen == m.auditGen {
		m.auditLoading, m.auditReport, m.auditErr = false, msg.report, msg.err
	}
	return m
}

func (m Model) cycleAuditDays() (Model, tea.Cmd) {
	m.auditDays = settings.Next(settings.AuditDayChoices, m.auditDays)
	return m.loadAudit()
}

func (m Model) updateAudit(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "d":
		return m.cycleAuditDays()
	case "j", "down":
		m = m.scrollAudit(1)
	case "k", "up":
		m = m.scrollAudit(-1)
	case "esc", "q":
		m.auditOpen = false
	}
	return m, nil
}

// clickAudit cycles the window from the title row; the wheel scrolls.
func (m Model) clickAudit(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelDown:
		return m.scrollAudit(1), nil
	case msg.Button == tea.MouseButtonWheelUp:
		return m.scrollAudit(-1), nil
	case msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft && msg.Y == 0:
		return m.cycleAuditDays()
	}
	return m, nil
}

func (m Model) scrollAudit(delta int) Model {
	last := max(len(m.auditReport.Hotspots)-m.auditRows(), 0)
	m.auditTop = min(max(m.auditTop+delta, 0), last)
	return m
}

// auditRows is how many hotspots fit: the terminal minus the title, column
// header, excluded line, footer and their blank lines.
func (m Model) auditRows() int {
	if m.height == 0 {
		return 20 // size not known yet
	}
	return max(m.height-8, 1)
}

const (
	commitsCol = 7
	linesCol   = 14
	signalCol  = 15
)

func (m Model) auditView() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	pathW := max(w-4-commitsCol-linesCol-signalCol, 12)

	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s %s %s\n\n", paint(green, bold.Render("nib")), paint(cyan, bold.Render("/audit")),
		paint(cyan, fmt.Sprintf("últimos %d días ▾", m.auditDays)), dim("· ordenado por commits"))
	fmt.Fprintf(&b, "  %s%s%s  %s\n", dimStyle.Bold(true).Render(padRight("ARCHIVO", pathW)),
		dimStyle.Bold(true).Render(fmt.Sprintf("%*s", commitsCol, "COMMITS")),
		dimStyle.Bold(true).Render(fmt.Sprintf("%*s", linesCol, "±LÍNEAS")), dimStyle.Bold(true).Render("SEÑAL"))

	hotspots := m.auditReport.Hotspots
	switch {
	case m.auditLoading:
		b.WriteString(dim("  leyendo historial…") + "\n")
	case m.auditErr != nil:
		b.WriteString(paint(red, "  ✗ audit no disponible: "+lastLine(m.auditErr.Error())) + "\n")
	case len(hotspots) == 0:
		b.WriteString(dim("  sin commits en esta ventana") + "\n")
	default:
		top := hotspots[0].Commits
		for _, h := range hotspots[m.auditTop:min(m.auditTop+m.auditRows(), len(hotspots))] {
			b.WriteString("  " + m.hotspotLine(h, top, pathW) + "\n")
		}
	}

	excluded := "excluidos: " + strings.Join(m.auditReport.Ignored, ", ")
	fmt.Fprintf(&b, "\n  %s\n\n %s\n", dim(truncate(excluded, w-2)), m.footer())
	return b.String()
}

// hotspotLine is one table row. Dots scale with the top file's count; the
// label rates the commits against the /settings threshold.
func (m Model) hotspotLine(h audit.Hotspot, top, pathW int) string {
	color, label := gray, "ok"
	switch audit.LevelOf(h.Commits, m.prefs.AuditThreshold) {
	case audit.Refactor:
		color, label = red, "refactor?"
	case audit.Watch:
		color, label = yellow, "vigilar"
	}
	dots := strings.Repeat("●", max((5*h.Commits+top-1)/top, 1))
	lines := fmt.Sprintf("%*s", linesCol, fmt.Sprintf("+%d −%d", h.Added, h.Deleted))
	return padRight(truncate(h.Path, pathW), pathW) +
		paint(color, bold.Render(fmt.Sprintf("%*d", commitsCol, h.Commits))) +
		dim(lines) + "  " + paint(color, padRight(dots, 6)+label)
}
