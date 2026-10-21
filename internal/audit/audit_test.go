package audit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRankSortsByCommitFrequency(t *testing.T) {
	out := "a.go\nb.go\na.go\na.go\nb.go\nc.go\n"
	got := rank(out, nil)

	if len(got) != 3 {
		t.Fatalf("expected 3 hotspots, got %+v", got)
	}
	if got[0].Path != "a.go" || got[0].Commits != 3 {
		t.Errorf("top hotspot = %+v, want a.go:3", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Commits < got[i].Commits {
			t.Errorf("not sorted by frequency: %+v", got)
		}
	}
}

func TestRankAppliesIgnore(t *testing.T) {
	out := "src/app.go\nnode_modules/x.js\nyarn.lock\nsrc/app.go\n"
	got := rank(out, defaultIgnore)

	if len(got) != 1 || got[0].Path != "src/app.go" {
		t.Errorf("ignore not applied, got %+v", got)
	}
}

func TestLoadIgnoreReadsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".auditignore"),
		[]byte("# comment\n\nvendor/\n*.gen.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	got, err := loadIgnore()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "vendor/") || !contains(got, "*.gen.go") {
		t.Errorf("custom patterns missing: %v", got)
	}
	if !contains(got, "node_modules/") {
		t.Errorf("default patterns missing: %v", got)
	}
}

func TestLoadIgnoreFallsBackToDefaults(t *testing.T) {
	chdir(t, t.TempDir()) // no .auditignore present
	got, err := loadIgnore()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(defaultIgnore) {
		t.Errorf("expected only defaults, got %v", got)
	}
}

func TestSince(t *testing.T) {
	cases := map[int]string{0: "30 days ago", -1: "30 days ago", 14: "14 days ago"}
	for in, want := range cases {
		if got := since(in); got != want {
			t.Errorf("since(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRankBreaksTiesByPath(t *testing.T) {
	got := rank("b.go\na.go\n", nil)
	if got[0].Path != "a.go" || got[1].Path != "b.go" {
		t.Errorf("ties must be ordered by path, got %+v", got)
	}
}

func TestRunRejectsInvalidAuditIgnore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".auditignore"), []byte("[\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	if _, err := Run(30, nil); err == nil {
		t.Error("a malformed .auditignore must not be silently ignored")
	}
}

func TestRunUsesStubbedLog(t *testing.T) {
	orig := gitLogNames
	t.Cleanup(func() { gitLogNames = orig })

	gitLogNames = func(_ int) (string, error) { return "x.go\nx.go\n", nil }
	chdir(t, t.TempDir())
	got, err := Run(30, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Commits != 2 {
		t.Errorf("Run = %+v, want x.go:2", got)
	}

	gitLogNames = func(_ int) (string, error) { return "", errors.New("boom") }
	if _, err := Run(30, nil); err == nil {
		t.Error("expected error to propagate")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
