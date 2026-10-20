package history

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repository identifies a worktree and its private index location.
type Repository struct{ Root, GitDir, Head string }

func command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "--no-replace-objects"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	return cmd
}

func output(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := command(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Open captures HEAD once and rejects incomplete history.
func Open(ctx context.Context, dir string) (Repository, error) {
	var r Repository
	root, err := output(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return r, err
	}
	r.Root = root
	gitDir, err := output(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return r, err
	}
	r.GitDir, err = filepath.EvalSymlinks(gitDir)
	if err != nil {
		return r, err
	}
	shallow, err := output(ctx, dir, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return r, err
	}
	if shallow != "false" {
		return r, errors.New("shallow history is incomplete; fetch full history before indexing")
	}
	r.Head, err = output(ctx, dir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return r, fmt.Errorf("repository needs at least one commit: %w", err)
	}
	return r, nil
}

// IndexPath returns a worktree-local database path outside tracked files.
func (r Repository) IndexPath() string { return filepath.Join(r.GitDir, "nib", "index.db") }

// Identity is stable across branch changes in this worktree.
func (r Repository) Identity() string { return r.GitDir }

// Revision is the captured immutable HEAD.
func (r Repository) Revision() string { return r.Head }

// IsAncestor distinguishes divergent history from command failures.
func (r Repository) IsAncestor(ctx context.Context, oid string) (bool, error) {
	err := command(ctx, r.Root, "merge-base", "--is-ancestor", oid, r.Head).Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return err == nil, err
}

// Walk streams commits reachable from captured HEAD but not from after.
// The callback's error aborts the Git process and the caller's transaction.
func (r Repository) Walk(ctx context.Context, after string, visit func(Commit) error) error {
	args := []string{
		"-c", "diff.renameLimit=1000", "log",
		"--reverse", "--topo-order", "--root",
		"--no-show-signature", "--no-notes", "--no-use-mailmap", "--encoding=UTF-8",
		"--no-ext-diff", "--no-textconv", "--no-color", "--no-abbrev",
		"--no-diff-merges", "--raw", "--numstat", "-z", "--find-renames=50%",
		"--format=%x00nib-commit%x00%H%x00%P%x00%an%x00%ae%x00%at%x00%B%x00", r.Head,
	}
	if after != "" {
		args = append(args, "^"+after)
	}
	args = append(args, "--")
	cmd := command(ctx, r.Root, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	parseErr := Parse(stdout, visit)
	if parseErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if parseErr != nil {
		return parseErr
	}
	if waitErr != nil {
		return fmt.Errorf("git history: %w: %s", waitErr, stderr.String())
	}
	return nil
}
