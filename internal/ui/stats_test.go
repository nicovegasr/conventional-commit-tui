package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/nicovegasr/conventional-commit-tui/internal/stats"
)

// fakeStats answers every window with report and records the last one.
type fakeStats struct {
	report stats.Report
	err    error

	daysArg int
}

func (f *fakeStats) Stats(days int) (stats.Report, error) {
	f.daysArg = days
	return f.report, f.err
}

func statsModel(s *fakeStats) Model {
	m, _ := sampleModel()
	m.stats = s
	m.width, m.height = 100, 30
	return m
}

// openStatsScreen walks the palette into /stats and feeds back its answer.
func openStatsScreen(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = send(m, "/")
	for _, r := range "st" {
		m, _ = send(m, string(r))
	}
	m, cmd := send(m, "enter")
	if !m.statsOpen || !m.statsLoading {
		t.Fatal("/stats should open and start loading")
	}
	return run(t, m, cmd)
}

var sampleStats = stats.Report{
	Types:      []stats.TypeCount{{Type: "feat", Count: 6}, {Type: "fix", Count: 3}, {Type: stats.Untyped, Count: 1}},
	Total:      10,
	Activity:   []int{0, 2, 0, 5, 3},
	BucketDays: 1,
}

func TestStatsOpensOnThirtyDays(t *testing.T) {
	s := &fakeStats{report: sampleStats}
	m := openStatsScreen(t, statsModel(s))
	if s.daysArg != 30 || m.statsDays != 30 {
		t.Errorf("window %d/%d, want 30", s.daysArg, m.statsDays)
	}
	if m.statsLoading || m.statsReport.Total != 10 {
		t.Errorf("report not stored: %+v", m.statsReport)
	}
}

func TestStatsDCyclesTheWindowAndDropsTheOldAnswer(t *testing.T) {
	s := &fakeStats{}
	m := openStatsScreen(t, statsModel(s))
	old := m.statsGen
	m, cmd := send(m, "d")
	if m.statsDays != 90 || !m.statsLoading {
		t.Fatalf("d: window %d loading %v", m.statsDays, m.statsLoading)
	}
	next, _ := m.Update(statsLoadedMsg{gen: old, err: errors.New("late")})
	if next.(Model).statsErr != nil {
		t.Error("an answer for the old window must be dropped")
	}
	if m = run(t, m, cmd); s.daysArg != 90 || m.statsLoading {
		t.Errorf("read %d days", s.daysArg)
	}
}

func TestStatsClickOnTitleCyclesTheWindow(t *testing.T) {
	m := openStatsScreen(t, statsModel(&fakeStats{}))
	if m = click(m, 10, 0); m.statsDays != 90 {
		t.Errorf("window = %d, want 90", m.statsDays)
	}
}

func TestStatsEscGoesBack(t *testing.T) {
	m := openStatsScreen(t, statsModel(&fakeStats{}))
	if m, _ = send(m, "esc"); m.statsOpen {
		t.Error("esc should close /stats")
	}
}

func TestStatsViewShowsTypesAndActivity(t *testing.T) {
	m := openStatsScreen(t, statsModel(&fakeStats{report: sampleStats}))
	v := m.View()
	for _, want := range []string{"feat", "60%", stats.Untyped, "ACTIVIDAD", "por día"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	m.statsErr = errors.New("no git")
	if v = m.View(); !strings.Contains(v, "no git") {
		t.Error("the error should be shown")
	}
}

func TestStatsTypesFillTheRowsLeft(t *testing.T) {
	m := openStatsScreen(t, statsModel(&fakeStats{report: sampleStats}))
	m.height = statsChromeRows + 3
	if v := m.View(); strings.Contains(v, "más") {
		t.Error("three types fit in three free rows")
	}
	m.height = statsChromeRows + 2
	if lines := m.typeLines(100); len(lines) != 2 || !strings.Contains(lines[1], "+2 tipos más") {
		t.Errorf("two free rows: %q", lines)
	}
}

func TestSparklineScalesToTheBusiestBucket(t *testing.T) {
	if got := sparkline([]int{0, 4, 8}, 1); got != " ▄█" {
		t.Errorf("sparkline = %q", got)
	}
	if got := sparkline([]int{1, 0}, 2); got != "██  " {
		t.Errorf("sparkline width 2 = %q", got)
	}
}
