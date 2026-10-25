// Package git wraps the git CLI for the operations nib needs. It shells out to
// `git` rather than using a library to stay small and fast.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FileChange is a single entry from `git status`.
type FileChange struct {
	Path   string
	Status rune // M, A, D, R, ? (porcelain XY collapsed to a single hint)
	Staged bool
}

// Status returns the working-tree changes via `git status --porcelain -z`.
// Every untracked file is listed, not just its new folder, so each one can be
// marked and diffed on its own.
func Status() ([]FileChange, error) {
	out, err := run("status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// parseStatus reads NUL-separated porcelain v1 entries ("XY path"). Paths are
// verbatim (no quoting), and a rename or copy is followed by its original path,
// which is skipped: staging the new path is what the commit flow needs.
func parseStatus(out string) []FileChange {
	var changes []FileChange
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		x, y := entry[0], entry[1]
		if x == 'R' || x == 'C' {
			i++
		}
		status := rune(x)
		if status == ' ' {
			status = rune(y)
		}
		changes = append(changes, FileChange{Path: entry[3:], Status: status, Staged: x != ' ' && x != '?'})
	}
	return changes
}

// Stage stages exactly the given files (replaces the old `git add .`).
func Stage(files []string) error {
	if len(files) == 0 {
		return nil
	}
	_, err := run(append([]string{"add", "--"}, files...)...)
	return err
}

// Commit creates a commit with the given full subject line.
func Commit(subject string) error {
	_, err := run("commit", "-m", subject)
	return err
}

// Log returns a pretty one-line log. scope is "current" or "all".
func Log(scope string, limit int) (string, error) {
	args := []string{"log", "--oneline", "--decorate", "--color=never"}
	if scope == "all" {
		args = append(args, "--all", "--graph")
	}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	return run(args...)
}

// Root is the repository's top-level directory. `git status` paths are
// relative to it, so nib runs from there whatever directory it starts in.
func Root() (string, error) {
	out, err := run("rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

// run executes git with the given args. It is a package var so tests can stub
// the shell-out and exercise the parsing without a real repository. A failure
// still returns stdout: `diff --no-index` exits 1 just to say files differ.
var run = func(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), failure(args[0], err, stdout.String(), stderr.String())
	}
	return stdout.String(), nil
}

// failure carries git's own explanation (a rejecting hook, "nothing to
// commit"), which goes to stderr or stdout depending on the command.
func failure(subcommand string, err error, stdout, stderr string) error {
	reason := strings.TrimSpace(stderr)
	if reason == "" {
		reason = strings.TrimSpace(stdout)
	}
	return fmt.Errorf("git %s: %w: %s", subcommand, err, reason)
}
