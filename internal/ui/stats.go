package ui

// The /stats screen: commits by conventional type and their activity over a
// window (ADR-0018).

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/nicovegasr/nib-git/internal/settings"
	"github.com/nicovegasr/nib-git/internal/stats"
)

// StatsReader is the narrow slice of the stats package the /stats screen needs.
type StatsReader interface {
	Stats(days int) (stats.Report, error)
}

// statsLoadedMsg answers the read numbered gen.
type statsLoadedMsg struct {
	gen    int
	report stats.Report
	err    error
}

const statsDefaultDays = 30

func (m Model) openStats() (Model, tea.Cmd) {
	m.statsOpen, m.statsDays = true, statsDefaultDays
	return m.loadStats()
}

// loadStats reads the window off the UI goroutine; answers to an older window
// are dropped.
func (m Model) loadStats() (Model, tea.Cmd) {
	m.statsGen++
	m.statsLoading = true
	reader, days, gen := m.stats, m.statsDays, m.statsGen
	return m, func() tea.Msg {
		report, err := reader.Stats(days)
		return statsLoadedMsg{gen: gen, report: report, err: err}
	}
}

func (m Model) statsLoaded(msg statsLoadedMsg) Model {
	if msg.gen == m.statsGen {
		m.statsLoading, m.statsReport, m.statsErr = false, msg.report, msg.err
	}
	return m
}

func (m Model) cycleStatsDays() (Model, tea.Cmd) {
	m.statsDays = settings.Next(settings.AuditDayChoices, m.statsDays)
	return m.loadStats()
}

func (m Model) updateStats(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "d":
		return m.cycleStatsDays()
	case "esc", "q":
		m.statsOpen = false
	}
	return m, nil
}

// clickStats cycles the window from the title row.
func (m Model) clickStats(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft && msg.Y == 0 {
		return m.cycleStatsDays()
	}
	return m, nil
}

// typeColors follow the commit's intent: new work green, fixes red.
var typeColors = map[string]lipgloss.TerminalColor{
	"feat": green, "fix": red, "hotfix": red, "refactor": magenta, "perf": magenta,
	"docs": cyan, "test": yellow, "red": red, "green": green,
}

func typeColor(t string) lipgloss.TerminalColor {
	if c, ok := typeColors[t]; ok {
		return c
	}
	return gray
}

const typeCol = 10

// statsChromeRows are the rows around the type bars: title, section titles,
// the activity row and its axis, blank lines, footer and a safety row.
const statsChromeRows = 10

func (m Model) statsView() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	r := m.statsReport
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s %s %s\n\n", paint(green, bold.Render("nib")), paint(cyan, bold.Render("/stats")),
		paint(cyan, fmt.Sprintf("últimos %d días ▾", m.statsDays)), dim("· rama actual"))
	switch {
	case m.statsLoading:
		b.WriteString(dim("  leyendo historial…") + "\n")
	case m.statsErr != nil:
		b.WriteString(paint(red, "  ✗ stats no disponible: "+lastLine(m.statsErr.Error())) + "\n")
	case r.Total == 0:
		b.WriteString(dim("  sin commits en esta ventana") + "\n")
	default:
		b.WriteString("  " + sectionTitle("POR TIPO", count(r.Total, "commit", "commits"), w-2) + "\n")
		for _, line := range m.typeLines(w) {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n" + m.activityView(w))
	}
	fmt.Fprintf(&b, "\n %s\n", m.footer())
	return b.String()
}

// typeLines is one bar per type, scaled to the most used one, cropped to the
// rows left for them.
func (m Model) typeLines(w int) []string {
	r := m.statsReport
	barW := min(max(w-typeCol-16, 5), 40)
	shown := len(r.Types)
	if free := m.height - statsChromeRows; m.height > 0 && shown > free {
		shown = max(free-1, 1) // the last free row says how many are left out
	}
	var lines []string
	for i, t := range r.Types {
		if i == shown {
			lines = append(lines, dim(fmt.Sprintf("+%d tipos más", len(r.Types)-shown)))
			break
		}
		bar := strings.Repeat("█", max(t.Count*barW/r.Types[0].Count, 1))
		name := padRight(truncate(t.Type, typeCol), typeCol)
		if t.Type == stats.Untyped {
			name = paint(yellow, name)
		}
		pct := fmt.Sprintf("%3.0f%%", 100*float64(t.Count)/float64(r.Total))
		lines = append(lines, name+" "+paint(typeColor(t.Type), padRight(bar, barW))+fmt.Sprintf(" %4d  ", t.Count)+dim(pct))
	}
	return lines
}

// activityView is the commits per bucket, oldest left, with its time axis.
func (m Model) activityView(w int) string {
	r := m.statsReport
	unit := "por día"
	if r.BucketDays > 1 {
		unit = "por semana"
	}
	n := max(len(r.Activity), 1)
	cell := min(max((w-4)/n, 1), 3)
	line := sparkline(r.Activity, cell)
	from := fmt.Sprintf("hace %d días", m.statsDays)
	axis := from + strings.Repeat(" ", max(n*cell-len([]rune(from))-3, 1)) + "hoy"
	return "  " + sectionTitle("ACTIVIDAD", unit, w-2) + "\n  " + baseline(line) + "\n  " + dim(axis) + "\n"
}

// baseline paints the bars green and the empty cells as a faint dotted line,
// so quiet stretches still read as time passing.
func baseline(line string) string {
	var b strings.Builder
	for _, r := range line {
		if r == ' ' {
			b.WriteString(paint(faint, "·"))
		} else {
			b.WriteString(paint(green, string(r)))
		}
	}
	return b.String()
}

var sparks = []rune(" ▁▂▃▄▅▆▇█")

// sparkline draws each count as a block cell wide, scaled to the busiest; any
// commit shows at least the lowest block.
func sparkline(counts []int, cell int) string {
	top := 0
	for _, c := range counts {
		top = max(top, c)
	}
	var b strings.Builder
	for _, c := range counts {
		level := 0
		if c > 0 {
			level = (c*(len(sparks)-1) + top - 1) / top
		}
		b.WriteString(strings.Repeat(string(sparks[level]), cell))
	}
	return b.String()
}
