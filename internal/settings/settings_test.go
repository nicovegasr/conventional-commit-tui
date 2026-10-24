package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultIsRichByDefault(t *testing.T) {
	d := Default()

	if !d.ShowLog || !d.ShowRelated || !d.MouseCapture {
		t.Errorf("panels and mouse should default on: %+v", d)
	}
	if d.LogScope != ScopeCurrent {
		t.Errorf("LogScope = %q, want current", d.LogScope)
	}
	if d.ReloadSeconds != 2 {
		t.Errorf("ReloadSeconds = %d, want 2", d.ReloadSeconds)
	}
	if d.AuditDays != 30 || d.AuditThreshold != 20 {
		t.Errorf("audit defaults = %d/%d, want 30/20", d.AuditDays, d.AuditThreshold)
	}
}

func TestLoadMissingFileIsDefault(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if s != Default() {
		t.Errorf("got %+v, want Default()", s)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nib", "settings.toml") // parent created by Save
	want := Default()
	want.ShowLog, want.MouseCapture, want.LogScope, want.ReloadSeconds = false, false, ScopeAll, 0

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFillsMissingKeysWithDefaults(t *testing.T) {
	s, err := Load(write(t, "show_log = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.ShowLog = false
	if s != want {
		t.Errorf("got %+v, want %+v", s, want)
	}
}

func TestLoadReplacesOutOfRangeValuesWithDefaults(t *testing.T) {
	s, err := Load(write(t, "log_scope = \"custom\"\nreload_seconds = 7\naudit_days = 3\naudit_threshold = 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s != Default() {
		t.Errorf("got %+v, want defaults", s)
	}
}

func TestLoadRejectsMalformedFile(t *testing.T) {
	if _, err := Load(write(t, "show_log = = true")); err == nil {
		t.Error("expected a parse error")
	}
}

func TestNextCyclesThroughOff(t *testing.T) {
	var got []int
	r := 2
	for range ReloadChoices {
		r = Next(ReloadChoices, r)
		got = append(got, r)
	}
	want := []int{5, 10, 0, 1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cycle = %v, want %v", got, want)
		}
	}
}

func TestPathPrefersXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p, _ := Path(); p != "/xdg/nib/settings.toml" {
		t.Errorf("Path = %q", p)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/nico")
	if p, _ := Path(); p != "/home/nico/.config/nib/settings.toml" {
		t.Errorf("Path = %q", p)
	}
}
