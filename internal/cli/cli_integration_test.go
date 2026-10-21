//go:build integration

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nicovegasr/nib-git/internal/history"
	"github.com/nicovegasr/nib-git/internal/index"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-C", dir, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}
	out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Alex")
	git(t, dir, "config", "user.email", "alex@example.test")
	return dir
}
func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
func commit(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "add", "--all")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "feat: inventory.")
}
func runJSON(t *testing.T, dir string, args []string, target any) {
	t.Helper()
	var out bytes.Buffer
	if err := Run(context.Background(), dir, args, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), target); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
}
func TestRealGitIncrementalMatchesRebuild(t *testing.T) {
	dir := repo(t)
	write(t, dir, "A", "1\n")
	write(t, dir, "B", "1\n")
	commit(t, dir)
	var initial index.Result
	runJSON(t, dir, []string{"index", "--json"}, &initial)
	write(t, dir, "A", "2\n")
	write(t, dir, "B", "2\n")
	commit(t, dir)
	if err := Run(context.Background(), dir, []string{"coupled", "A", "--json"}, &bytes.Buffer{}); !errors.Is(err, index.ErrStale) {
		t.Fatalf("%v", err)
	}
	var updated index.Result
	runJSON(t, dir, []string{"index", "--json"}, &updated)
	if initial.Added != 1 || updated.Added != 1 || updated.Commits != 2 {
		t.Fatalf("initial %+v updated %+v", initial, updated)
	}
	var before, after index.Report
	args := []string{"coupled", "A", "--min-support", "1", "--json"}
	runJSON(t, dir, args, &before)
	var heldOut index.Report
	cutoff := git(t, dir, "rev-parse", "HEAD")
	runJSON(t, dir, []string{"coupled", "A", "--before", cutoff, "--min-support", "1", "--json"}, &heldOut)
	if heldOut.Eligible != 1 || len(heldOut.Relationships) != 1 || heldOut.Relationships[0].Shared != 1 || heldOut.Options.Before != cutoff {
		t.Fatalf("cutoff commit leaked into historical query: %+v", heldOut)
	}
	var abbreviated index.Report
	runJSON(t, dir, []string{"coupled", "--json", "--before", "HEAD", "--min-support", "1", "A"}, &abbreviated)
	if !reflect.DeepEqual(abbreviated, heldOut) {
		t.Fatalf("a symbolic cutoff with leading flags must match the full OID: %+v", abbreviated)
	}
	runJSON(t, dir, []string{"index", "--rebuild", "--json"}, &updated)
	runJSON(t, dir, args, &after)
	if !reflect.DeepEqual(before, after) || before.Eligible != 2 || before.Relationships[0].Shared != 2 {
		t.Fatalf("before %+v after %+v", before, after)
	}
	git(t, dir, "reset", "--hard", "HEAD~1")
	if err := Run(context.Background(), dir, []string{"index"}, &bytes.Buffer{}); !errors.Is(err, index.ErrRewrite) {
		t.Fatalf("%v", err)
	}
	runJSON(t, dir, []string{"index", "--rebuild", "--json"}, &updated)
	if updated.Commits != 1 {
		t.Fatalf("%+v", updated)
	}
}

func TestRealGitNibignoreFiltersCandidatesWithoutReindexing(t *testing.T) {
	dir := repo(t)
	for i := 0; i < 2; i++ {
		content := "first\n"
		if i > 0 {
			content = "second\n"
		}
		write(t, dir, "A", content)
		write(t, dir, "B", content)
		write(t, dir, "package-lock.json", content)
		commit(t, dir)
	}
	var indexed index.Result
	runJSON(t, dir, []string{"index", "--json"}, &indexed)

	var before index.Report
	runJSON(t, dir, []string{"coupled", "A", "--min-support", "1", "--json"}, &before)
	if len(before.Relationships) != 2 || before.Eligible != 2 {
		t.Fatalf("unfiltered report = %+v", before)
	}
	write(t, dir, ".nibignore", "# generated dependency metadata\npackage-lock.json\n")

	var after index.Report
	runJSON(t, dir, []string{"coupled", "A", "--min-support", "1", "--json"}, &after)
	if after.Eligible != before.Eligible || after.Relationships[0].Path != "B" || len(after.Relationships) != 1 || !reflect.DeepEqual(after.Options.Ignore, []string{"package-lock.json"}) {
		t.Fatalf("filtered report = %+v", after)
	}
	if err := Run(context.Background(), dir, []string{"coupled", "package-lock.json", "--min-support", "1"}, &bytes.Buffer{}); !errors.Is(err, index.ErrIgnoredPath) {
		t.Fatalf("ignored source error = %v", err)
	}
}

func TestRealGitRenameBinaryAndSpecialPaths(t *testing.T) {
	dir := repo(t)
	old := "old\tname\n.txt"
	newPath := " nuevo ñ\n.txt "
	write(t, dir, old, "inventory\n")
	write(t, dir, "image.bin", "\x00\x01\x02")
	commit(t, dir)
	git(t, dir, "mv", "--", old, newPath)
	commit(t, dir)
	r, err := history.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var commits []history.Commit
	if err = r.Walk(context.Background(), "", func(c history.Commit) error { commits = append(commits, c); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || !commits[0].Changes[0].Binary || commits[1].Changes[0].PreviousPath != old || commits[1].Changes[0].Path != newPath {
		t.Fatalf("%+v", commits)
	}
	var result index.Result
	runJSON(t, dir, []string{"index", "--json"}, &result)
	var report index.Report
	runJSON(t, dir, []string{"coupled", newPath, "--min-support", "1", "--json"}, &report)
	if report.Eligible != 1 || len(report.Relationships) != 0 {
		t.Fatalf("rename was silently joined: %+v", report)
	}
}
func TestRealGitMergeImportsSideBranchOnce(t *testing.T) {
	dir := repo(t)
	write(t, dir, "A", "1\n")
	commit(t, dir)
	git(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir, "A", "2\n")
	write(t, dir, "B", "2\n")
	commit(t, dir)
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "C", "1\n")
	commit(t, dir)
	var result index.Result
	runJSON(t, dir, []string{"index", "--json"}, &result)
	git(t, dir, "merge", "--no-ff", "-m", "merge side", "side")
	runJSON(t, dir, []string{"index", "--json"}, &result)
	if result.Added != 2 || result.Commits != 4 {
		t.Fatalf("%+v", result)
	}
	var report index.Report
	runJSON(t, dir, []string{"coupled", "A", "--min-support", "1", "--json"}, &report)
	if report.Eligible != 2 || report.Relationships[0].Shared != 1 || report.Relationships[0].Ratio != 0.5 {
		t.Fatalf("%+v", report)
	}
	mergeOID := git(t, dir, "rev-parse", "HEAD")
	var historical index.Report
	runJSON(t, dir, []string{"coupled", "A", "--before", mergeOID, "--min-support", "1", "--json"}, &historical)
	if historical.Eligible != 2 || historical.Relationships[0].Path != "B" || historical.Relationships[0].Shared != 1 {
		t.Fatalf("strict merge ancestors = %+v", historical)
	}
	var before index.Report
	runJSON(t, dir, []string{"coupled", "A", "--min-support", "1", "--json"}, &before)
	runJSON(t, dir, []string{"index", "--rebuild", "--json"}, &result)
	runJSON(t, dir, []string{"coupled", "A", "--min-support", "1", "--json"}, &report)
	if !reflect.DeepEqual(before, report) {
		t.Fatalf("merge incremental differs: %+v %+v", before, report)
	}
}
func TestRealGitWorktreeIsolationAndShallowRejection(t *testing.T) {
	dir := repo(t)
	write(t, dir, "A", "1\n")
	commit(t, dir)
	worktree := filepath.Join(t.TempDir(), "linked")
	git(t, dir, "worktree", "add", "--detach", worktree, "HEAD")
	main, err := history.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := history.Open(context.Background(), worktree)
	if err != nil {
		t.Fatal(err)
	}
	if main.IndexPath() == linked.IndexPath() {
		t.Fatal("worktrees share index")
	}
	var result index.Result
	runJSON(t, worktree, []string{"index", "--json"}, &result)
	if _, err = os.Stat(main.IndexPath()); !os.IsNotExist(err) {
		t.Fatalf("main worktree was changed: %v", err)
	}
	clone := filepath.Join(t.TempDir(), "shallow")
	git(t, dir, "clone", "-q", "--depth=1", "file://"+dir, clone)
	if _, err = history.Open(context.Background(), clone); err == nil || !strings.Contains(err.Error(), "shallow") {
		t.Fatalf("%v", err)
	}
}
func TestRealGitEvaluateHoldsOutCommits(t *testing.T) {
	dir := repo(t)
	for i := range 6 {
		write(t, dir, "A", strings.Repeat("a", i+1))
		write(t, dir, "B", strings.Repeat("b", i+1))
		commit(t, dir)
	}
	if err := Run(context.Background(), dir, []string{"evaluate"}, &bytes.Buffer{}); err == nil {
		t.Fatal("evaluated without an index")
	}
	runJSON(t, dir, []string{"index", "--json"}, &index.Result{})

	var e index.Evaluation
	runJSON(t, dir, []string{"evaluate", "--stride", "1", "--min-support", "1", "--json"}, &e)
	// The root only adds files; each later commit modifies A and B together.
	if e.Sampled != 6 || len(e.Cases) != 5 || e.Skipped.NoSource != 1 {
		t.Fatalf("%+v", e)
	}
	nib := e.Predictors[0]
	if nib.Name != index.PredictorNib || nib.Metrics[0].Hits != 5 || nib.Metrics[0].Precision != 1 {
		t.Fatalf("perfect co-change must be predicted at k=1: %+v", nib)
	}
	var text bytes.Buffer
	if err := Run(context.Background(), dir, []string{"evaluate", "--stride", "1"}, &text); err != nil || !strings.Contains(text.String(), "Cases: 5 of 6") {
		t.Fatalf("%v\n%s", err, text.String())
	}
}
