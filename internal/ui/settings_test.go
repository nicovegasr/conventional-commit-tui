package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/conventional-commit-tui/internal/git"
	"github.com/nicovegasr/conventional-commit-tui/internal/settings"
)

// fakeStore serves prefs (Default() when unset) and records saves.
type fakeStore struct {
	prefs   *settings.Settings
	loadErr error
	saveErr error

	saved []settings.Settings
}

func (f *fakeStore) Load() (settings.Settings, error) {
	if f.prefs == nil {
		return settings.Default(), f.loadErr
	}
	return *f.prefs, f.loadErr
}

func (f *fakeStore) Save(s settings.Settings) error {
	f.saved = append(f.saved, s)
	return f.saveErr
}

func storeModel(store *fakeStore) (Model, *fakeLogger) {
	repo := &fakeRepo{status: []git.FileChange{{Path: "a.go", Status: 'M'}}}
	lg := &fakeLogger{out: "abc123 feat: x\n"}
	return New(repo, lg, &fakeHistory{}, store, &fakeAuditor{}, &fakeStats{}, &fakeDiffer{}, &fakeEditor{}), lg
}

// openSettings walks the palette into /settings.
func openSettings(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = send(m, "/")
	m, _ = send(m, "s")
	m, _ = send(m, "enter")
	if !m.settingsOpen {
		t.Fatal("/s + enter should open /settings")
	}
	return m
}

// changeRow highlights row and changes it with space.
func changeRow(m Model, row int) (Model, tea.Cmd) {
	m.settingsIdx = row
	return send(m, "space")
}

func TestSlashOpensPaletteOnlyWhileMessageIsEmpty(t *testing.T) {
	m, _ := sampleModel()
	if m, _ = send(m, "/"); !m.palette {
		t.Fatal("/ in the Files zone should open the palette")
	}

	m, _ = sampleModel()
	m.focus = zoneMessage
	if m, _ = send(m, "/"); !m.palette || m.message != "" {
		t.Fatalf("/ on an empty message opens the palette, message %q", m.message)
	}

	m, _ = sampleModel()
	m.focus, m.message = zoneMessage, "a"
	if m, _ = send(m, "/"); m.palette || m.message != "a/" {
		t.Errorf("/ after text is text: palette %v, message %q", m.palette, m.message)
	}
}

func TestPaletteFiltersAndRunsSettings(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "/")
	for _, r := range "zz" {
		m, _ = send(m, string(r))
	}
	if len(m.paletteMatches()) != 0 {
		t.Fatal("zz matches no command")
	}
	if m, _ = send(m, "enter"); !m.palette || m.settingsOpen {
		t.Fatal("enter with no match keeps the palette open")
	}
	m, _ = send(m, "backspace")
	m, _ = send(m, "backspace")
	m, _ = send(m, "s")
	if m, _ = send(m, "enter"); m.palette || !m.settingsOpen {
		t.Error("enter on /settings opens it and closes the palette")
	}
}

func TestPaletteClosesWithEscOrBackspaceOnEmptyQuery(t *testing.T) {
	m, _ := sampleModel()
	m, _ = send(m, "/")
	if m, _ = send(m, "esc"); m.palette {
		t.Error("esc closes the palette")
	}
	m, _ = send(m, "/")
	if m, _ = send(m, "backspace"); m.palette {
		t.Error("backspace on an empty query closes the palette")
	}
}

func TestSettingsEscReturnsWithoutQuitting(t *testing.T) {
	m, _ := sampleModel()
	m = openSettings(t, m)
	m, cmd := send(m, "q")
	if m.settingsOpen || cmd != nil {
		t.Error("q leaves /settings instead of quitting nib")
	}
}

func TestEveryChangeIsSaved(t *testing.T) {
	store := &fakeStore{}
	m, _ := storeModel(store)
	m = openSettings(t, m)
	m, cmd := changeRow(m, int(rowLog))
	if _, ok := batched[settingsSavedMsg](cmd); !ok {
		t.Fatal("a change must be saved")
	}
	if len(store.saved) != 1 || store.saved[0].ShowLog || m.prefs.ShowLog {
		t.Errorf("saved %+v, prefs %+v", store.saved, m.prefs)
	}
}

func TestLogOffStopsReloadingIt(t *testing.T) {
	m, lg := storeModel(&fakeStore{})
	m, _ = changeRow(openSettings(t, m), int(rowLog))
	if m.logVisible() {
		t.Error("the log panel must hide")
	}
	lg.scopeArg, m.prefs.ReloadSeconds = "", 0 // no tick to wait for
	_, cmd := m.Update(pollMsg{gen: m.pollGen})
	batched[tea.QuitMsg](cmd) // run every reload command
	if lg.scopeArg != "" {
		t.Error("a poll must not run git log while its panel is off")
	}
}

func TestLogScopeTogglesAndReloads(t *testing.T) {
	m, lg := storeModel(&fakeStore{})
	m, cmd := changeRow(openSettings(t, m), int(rowScope))
	if m.prefs.LogScope != settings.ScopeAll || !m.logLoading {
		t.Fatalf("scope %q loading %v", m.prefs.LogScope, m.logLoading)
	}
	if _, ok := batched[logLoadedMsg](cmd); !ok || lg.scopeArg != settings.ScopeAll {
		t.Errorf("log reloaded with scope %q", lg.scopeArg)
	}
	if m, _ = changeRow(m, int(rowScope)); m.prefs.LogScope != settings.ScopeCurrent {
		t.Error("the scope toggles back to the current branch")
	}
}

func TestScopeOnlyChangesWhileTheLogIsOn(t *testing.T) {
	m, _ := storeModel(&fakeStore{})
	m, _ = changeRow(openSettings(t, m), int(rowLog))
	m, cmd := changeRow(m, int(rowScope))
	if m.prefs.LogScope != settings.ScopeCurrent || cmd != nil {
		t.Errorf("scope %q changed with the log off", m.prefs.LogScope)
	}
}

func TestRelatedOffClearsThePanel(t *testing.T) {
	m, _ := storeModel(&fakeStore{})
	m.related = []Related{{Path: "b.go"}}
	if m, _ = changeRow(openSettings(t, m), int(rowRelated)); m.prefs.ShowRelated || m.related != nil {
		t.Errorf("related off: prefs %v related %v", m.prefs.ShowRelated, m.related)
	}
}

func TestMouseCaptureTogglesLive(t *testing.T) {
	m, _ := storeModel(&fakeStore{})
	m, cmd := changeRow(openSettings(t, m), int(rowMouse))
	if m.prefs.MouseCapture {
		t.Fatal("mouse capture should switch off")
	}
	disable := reflect.TypeOf(tea.DisableMouse())
	found := false
	for _, msg := range messages(cmd) {
		if reflect.TypeOf(msg) == disable {
			found = true
		}
	}
	if !found {
		t.Error("switching off must disable the mouse now")
	}
}

func TestReloadIntervalStartsANewLoopAndDropsTheOld(t *testing.T) {
	m, _ := storeModel(&fakeStore{})
	old := m.pollGen
	m, _ = changeRow(openSettings(t, m), int(rowReload))
	if m.prefs.ReloadSeconds != 5 || m.pollGen == old {
		t.Fatalf("reload %d gen %d", m.prefs.ReloadSeconds, m.pollGen)
	}
	if _, cmd := m.Update(pollMsg{gen: old}); cmd != nil {
		t.Error("a tick of the old interval must be dropped")
	}
	m.prefs.ReloadSeconds = 0
	if m.poll() != nil {
		t.Error("auto reload off schedules no tick")
	}
}

func TestNewAppliesStoredSettings(t *testing.T) {
	prefs := settings.Default()
	prefs.ShowLog, prefs.MouseCapture, prefs.ReloadSeconds = false, false, 0
	m, _ := storeModel(&fakeStore{prefs: &prefs})
	if m.logLoading || m.logVisible() {
		t.Error("a stored log-off must not show or load the log")
	}
	enable := reflect.TypeOf(tea.EnableMouseCellMotion())
	for _, msg := range messages(m.Init()) {
		if reflect.TypeOf(msg) == enable {
			t.Error("a stored mouse-off must not enable the mouse")
		}
	}
}

func TestUnreadableSettingsFallBackToDefaults(t *testing.T) {
	m, _ := storeModel(&fakeStore{loadErr: errors.New("bad toml")})
	if m.prefs != settings.Default() || !strings.Contains(m.flash, "bad toml") {
		t.Errorf("prefs %+v flash %q", m.prefs, m.flash)
	}
}

func TestFailedSaveIsReported(t *testing.T) {
	m, _ := sampleModel()
	next, _ := m.Update(settingsSavedMsg{err: errors.New("read-only")})
	if !strings.Contains(next.(Model).flash, "read-only") {
		t.Error("a failed save must say so")
	}
}

func TestClickingCommandsButtonThenItemOpensSettings(t *testing.T) {
	m, _ := sampleModel()
	m = click(m, 2, m.layout().footerRow)
	if !m.palette {
		t.Fatal("clicking / comandos opens the palette")
	}
	m = click(m, 4, m.layout().dropStart+1) // /audit, /settings
	if !m.settingsOpen {
		t.Error("clicking /settings opens it")
	}
	m = click(m, 4, headerRows+0)
	if m.prefs.ShowLog {
		t.Error("clicking a settings row changes it")
	}
}
