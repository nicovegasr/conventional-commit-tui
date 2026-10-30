package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/conventional-commit-tui/internal/audit"
)

// fakeAuditor answers every window with report and records the last one.
type fakeAuditor struct {
	report audit.Report
	err    error

	daysArg int
}

func (f *fakeAuditor) Audit(days int) (audit.Report, error) {
	f.daysArg = days
	return f.report, f.err
}

func auditModel(a *fakeAuditor) Model {
	m, _ := sampleModel()
	m.auditor = a
	return m
}

// openAuditScreen walks the palette into /audit and feeds back its answer.
func openAuditScreen(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = send(m, "/")
	for _, r := range "au" {
		m, _ = send(m, string(r))
	}
	m, cmd := send(m, "enter")
	if !m.auditOpen || !m.auditLoading {
		t.Fatal("/audit should open and start loading")
	}
	return run(t, m, cmd)
}

func TestAuditOpensWithTheSettingsWindow(t *testing.T) {
	a := &fakeAuditor{report: audit.Report{Hotspots: []audit.Hotspot{{Path: "x.go", Commits: 3}}}}
	m := openAuditScreen(t, auditModel(a))
	if a.daysArg != 30 || m.auditDays != 30 {
		t.Errorf("window %d/%d, want the default 30", a.daysArg, m.auditDays)
	}
	if m.auditLoading || len(m.auditReport.Hotspots) != 1 {
		t.Errorf("report not stored: %+v", m.auditReport)
	}
}

func TestAuditDCyclesTheWindowAndDropsTheOldAnswer(t *testing.T) {
	a := &fakeAuditor{}
	m := openAuditScreen(t, auditModel(a))
	old := m.auditGen
	m, cmd := send(m, "d")
	if m.auditDays != 90 || !m.auditLoading {
		t.Fatalf("d: window %d loading %v", m.auditDays, m.auditLoading)
	}
	next, _ := m.Update(auditLoadedMsg{gen: old, err: errors.New("late")})
	if next.(Model).auditErr != nil {
		t.Error("an answer for the old window must be dropped")
	}
	if m = run(t, m, cmd); a.daysArg != 90 || m.auditLoading {
		t.Errorf("audited %d days", a.daysArg)
	}
}

func TestAuditClickOnTitleCyclesTheWindow(t *testing.T) {
	m := openAuditScreen(t, auditModel(&fakeAuditor{}))
	if m = click(m, 10, 0); m.auditDays != 90 {
		t.Errorf("window = %d, want 90", m.auditDays)
	}
}

func TestAuditScrollStaysInBounds(t *testing.T) {
	var hotspots []audit.Hotspot
	for range 30 {
		hotspots = append(hotspots, audit.Hotspot{Path: "f.go", Commits: 1})
	}
	m := openAuditScreen(t, auditModel(&fakeAuditor{report: audit.Report{Hotspots: hotspots}}))
	m.height = 18 // 10 rows visible
	if m, _ = send(m, "k"); m.auditTop != 0 {
		t.Error("cannot scroll above the first row")
	}
	for range 40 {
		m, _ = send(m, "j")
	}
	if m.auditTop != 20 {
		t.Errorf("auditTop = %d, want 20 (last page)", m.auditTop)
	}
	next, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if next.(Model).auditTop != 19 {
		t.Error("the wheel scrolls up")
	}
}

func TestAuditEscReturnsWithoutQuitting(t *testing.T) {
	m := openAuditScreen(t, auditModel(&fakeAuditor{}))
	if m, cmd := send(m, "esc"); m.auditOpen || cmd != nil {
		t.Error("esc leaves /audit")
	}
}

func TestAuditRatesHotspotsAgainstTheThreshold(t *testing.T) {
	report := audit.Report{
		Hotspots: []audit.Hotspot{{Path: "hot.go", Commits: 23}, {Path: "warm.go", Commits: 16}, {Path: "cold.go", Commits: 2}},
		Ignored:  []string{"*.lock"},
	}
	m := openAuditScreen(t, auditModel(&fakeAuditor{report: report}))
	view := m.View()
	for _, want := range []string{"refactor?", "vigilar", "ok", "excluidos: *.lock"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
}

func TestAuditErrorIsShown(t *testing.T) {
	m := openAuditScreen(t, auditModel(&fakeAuditor{err: errors.New("not a git repository")}))
	if !strings.Contains(m.View(), "not a git repository") {
		t.Error("the audit error must be shown")
	}
}

func TestSettingsCycleAuditDefaults(t *testing.T) {
	m, _ := storeModel(&fakeStore{})
	m, _ = changeRow(openSettings(t, m), int(rowAuditDays))
	m, _ = changeRow(m, int(rowThreshold))
	if m.prefs.AuditDays != 90 || m.prefs.AuditThreshold != 30 {
		t.Errorf("audit defaults %d/%d, want 90/30", m.prefs.AuditDays, m.prefs.AuditThreshold)
	}
}
