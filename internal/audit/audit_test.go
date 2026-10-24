package audit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRankSortsByCommitFrequency(t *testing.T) {
	out := "1\t1\ta.go\n1\t0\tb.go\n2\t2\ta.go\n3\t0\ta.go\n0\t1\tb.go\n1\t1\tc.go\n"
	got := rank(out, nil)

	if len(got) != 3 {
		t.Fatalf("expected 3 hotspots, got %+v", got)
	}
	if got[0] != (Hotspot{Path: "a.go", Commits: 3, Added: 6, Deleted: 3}) {
		t.Errorf("top hotspot = %+v, want a.go:3 +6 -3", got[0])
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Commits < got[i].Commits {
			t.Errorf("not sorted by frequency: %+v", got)
		}
	}
}

func TestRankLinesNeverOutrankCommits(t *testing.T) {
	got := rank("1000\t1000\tbig.go\n1\t0\tsmall.go\n1\t0\tsmall.go\n", nil)
	if got[0].Path != "small.go" {
		t.Errorf("commits are the sort key, got %+v", got)
	}
}

func TestRankCountsBinaryFilesWithoutLines(t *testing.T) {
	got := rank("-\t-\tlogo.png\n-\t-\tlogo.png\n", nil)
	if got[0] != (Hotspot{Path: "logo.png", Commits: 2}) {
		t.Errorf("binary hotspot = %+v", got[0])
	}
}

func TestRankAppliesIgnore(t *testing.T) {
	out := "1\t1\tsrc/app.go\n1\t1\tnode_modules/x.js\n1\t1\tyarn.lock\n1\t1\tsrc/app.go\n"
	got := rank(out, defaultIgnore)

	if len(got) != 1 || got[0].Path != "src/app.go" {
		t.Errorf("ignore not applied, got %+v", got)
	}
}

func TestRankBreaksTiesByPath(t *testing.T) {
	got := rank("1\t1\tb.go\n1\t1\ta.go\n", nil)
	if got[0].Path != "a.go" || got[1].Path != "b.go" {
		t.Errorf("ties must be ordered by path, got %+v", got)
	}
}

func TestLevelAgainstThreshold(t *testing.T) {
	cases := map[int]Level{25: Refactor, 20: Refactor, 15: Watch, 14: OK, 0: OK}
	for commits, want := range cases {
		if got := LevelOf(commits, 20); got != want {
			t.Errorf("LevelOf(%d, 20) = %v, want %v", commits, got, want)
		}
	}
}

func TestLoadIgnoreReadsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".auditignore"),
		[]byte("# comment\n\nvendor/\n*.gen.go\nnode_modules/\n"), 0o644); err != nil {
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
	if !contains(got, "node_modules/") || len(got) != len(defaultIgnore)+2 {
		t.Errorf("want the defaults plus two new patterns, each once: %v", got)
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

func TestRunRejectsInvalidAuditIgnore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".auditignore"), []byte("[\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	if _, err := Run(30); err == nil {
		t.Error("a malformed .auditignore must not be silently ignored")
	}
}

func TestRunUsesStubbedLogAndReportsIgnored(t *testing.T) {
	orig := gitLogNumstat
	t.Cleanup(func() { gitLogNumstat = orig })

	gitLogNumstat = func(_ int) (string, error) { return "1\t0\tx.go\n2\t1\tx.go\n", nil }
	chdir(t, t.TempDir())
	got, err := Run(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hotspots) != 1 || got.Hotspots[0].Commits != 2 {
		t.Errorf("Run = %+v, want x.go:2", got.Hotspots)
	}
	if len(got.Ignored) != len(defaultIgnore) {
		t.Errorf("Ignored = %v, want the defaults", got.Ignored)
	}

	gitLogNumstat = func(_ int) (string, error) { return "", errors.New("boom") }
	if _, err := Run(30); err == nil {
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
