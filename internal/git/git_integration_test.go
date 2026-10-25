//go:build integration

// Integration tests exercise the real git CLI against a throwaway repository,
// covering the happy paths the unit tests stub out. Run with:
//
//	make test-integration   # go test -tags=integration ./...
//
// They are excluded from the default `make test` and the coverage gate on
// purpose (ADR-0002): the shell-out seam stays "uncovered" by unit metrics so
// the discipline holds, while these give an end-to-end safety net.
package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo creates an initialised git repo in a temp dir and chdir's into it for
// the duration of the test.
func newRepo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@nib.local"},
		{"config", "user.name", "nib test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(".", name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStageCommitLogRoundTrip(t *testing.T) {
	newRepo(t)

	writeFile(t, "a.go", "package a\n")
	writeFile(t, "b.go", "package b\n")

	// Status sees two untracked files.
	changes, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("Status() = %+v, want 2 untracked", changes)
	}
	for _, c := range changes {
		if c.Status != '?' || c.Staged {
			t.Errorf("untracked file misread: %+v", c)
		}
	}

	// Stage exactly one file (never `git add .`).
	if err := Stage([]string{"a.go"}); err != nil {
		t.Fatal(err)
	}
	changes, _ = Status()
	var staged, unstaged int
	for _, c := range changes {
		if c.Staged {
			staged++
		} else {
			unstaged++
		}
	}
	if staged != 1 || unstaged != 1 {
		t.Errorf("after staging a.go: staged=%d unstaged=%d, want 1/1", staged, unstaged)
	}

	// Commit and confirm it lands in the log.
	if err := Commit("feat: add a."); err != nil {
		t.Fatal(err)
	}
	out, err := Log("current", 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := "feat: add a."; !strings.Contains(out, want) {
		t.Errorf("Log() = %q, want it to contain %q", out, want)
	}
}

func TestStatusKeepsPathsWithSpacesStageable(t *testing.T) {
	newRepo(t)
	writeFile(t, "my file.go", "package a\n")

	changes, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "my file.go" {
		t.Fatalf("Status() = %+v, want the verbatim path", changes)
	}
	if err := Stage([]string{changes[0].Path}); err != nil {
		t.Fatal(err)
	}
}

func TestCommitErrorCarriesHookReason(t *testing.T) {
	newRepo(t)
	hooks, err := filepath.Abs("hooks")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho 'lint failed' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "config", "core.hooksPath", hooks).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	writeFile(t, "a.go", "package a\n")
	if err := Stage([]string{"a.go"}); err != nil {
		t.Fatal(err)
	}

	err = Commit("feat: add a.")
	if err == nil || !strings.Contains(err.Error(), "lint failed") {
		t.Fatalf("Commit() error = %v, want the hook's reason", err)
	}
}

func TestCommitErrorCarriesStdoutReason(t *testing.T) {
	newRepo(t)
	err := Commit("feat: nothing.")
	if err == nil || !strings.Contains(err.Error(), "nothing to commit") {
		t.Fatalf("Commit() error = %v, want git's explanation", err)
	}
}

func TestDiffFileShowsTrackedAndUntrackedChanges(t *testing.T) {
	newRepo(t)
	writeFile(t, "a.go", "package a\n\nfunc old() {}\n")
	if err := Stage([]string{"a.go"}); err != nil {
		t.Fatal(err)
	}
	if err := Commit("feat: add a."); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "a.go", "package a\n\nfunc new() {}\n")
	writeFile(t, "n.txt", "hi\n")

	d, err := DiffFile(FileChange{Path: "a.go", Status: 'M'})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Lines) != 4 || d.Added != 1 || d.Removed != 1 {
		t.Errorf("tracked diff = %+v, want the whole file with one change", d)
	}
	d, err = DiffFile(FileChange{Path: "n.txt", Status: '?'})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Lines) != 1 || d.Lines[0] != (DiffLine{Kind: Added, Text: "hi", Old: 1, New: 1}) {
		t.Errorf("untracked diff = %+v", d)
	}
}

func TestStageFromASubdirectoryAfterMovingToTheRoot(t *testing.T) {
	newRepo(t)
	if err := os.Mkdir("docs", 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "README.md", "a\n")
	want, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("docs"); err != nil {
		t.Fatal(err)
	}

	root, err := Root()
	if err != nil || root != want {
		t.Fatalf("Root() from docs/ = %q, %v; want %q", root, err, want)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	changes, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if err := Stage([]string{changes[0].Path}); err != nil {
		t.Errorf("status paths must be stageable from the root: %v", err)
	}
}
