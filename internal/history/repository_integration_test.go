//go:build integration

package history

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func gitForHistoryTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-C", dir, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false"}
	out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeHistoryFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func commitHistoryFile(t *testing.T, dir, message string) {
	t.Helper()
	gitForHistoryTest(t, dir, "add", "--all")
	gitForHistoryTest(t, dir, "commit", "-q", "-m", message)
}

func TestWalkBatchesPreserveOrderAndIncrementalRange(t *testing.T) {
	dir := t.TempDir()
	gitForHistoryTest(t, dir, "init", "-q", "-b", "main")
	gitForHistoryTest(t, dir, "config", "user.name", "Test")
	gitForHistoryTest(t, dir, "config", "user.email", "test@example.test")
	writeHistoryFile(t, dir, "root.txt")
	commitHistoryFile(t, dir, "root")
	gitForHistoryTest(t, dir, "checkout", "-q", "-b", "side")
	writeHistoryFile(t, dir, "side.txt")
	commitHistoryFile(t, dir, "side")
	gitForHistoryTest(t, dir, "checkout", "-q", "main")
	writeHistoryFile(t, dir, "main.txt")
	commitHistoryFile(t, dir, "main")
	gitForHistoryTest(t, dir, "merge", "--no-ff", "-q", "-m", "merge side", "side")

	repo, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var full, batched []Commit
	if err = repo.Walk(context.Background(), "", func(c Commit) error { full = append(full, c); return nil }); err != nil {
		t.Fatal(err)
	}
	if err = repo.walkBatches(context.Background(), "", 2, func(c Commit) error { batched = append(batched, c); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(batched, full) {
		t.Fatalf("batched history differs from single batch:\nbatched=%+v\nfull=%+v", batched, full)
	}
	if len(batched) != 4 || len(batched[3].Parents) != 2 || len(batched[3].Changes) != 0 {
		t.Fatalf("merge metadata or diff semantics changed: %+v", batched)
	}

	var incremental []Commit
	if err = repo.walkBatches(context.Background(), batched[0].OID, 2, func(c Commit) error { incremental = append(incremental, c); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(incremental, batched[1:]) {
		t.Fatalf("incremental range differs: got %d commits, want %d", len(incremental), len(batched)-1)
	}
}
