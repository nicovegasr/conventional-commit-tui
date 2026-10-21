// Package audit implements /audit: change-frequency hotspots.
//
// Primary signal is the NUMBER OF COMMITS that touch a file (Adam Tornhill's
// hotspot method, "Your Code as a Crime Scene"). Lines changed are noisy
// (renames, reformats, lockfiles) so they are only context, never the sort
// key. v0.2 idea: commits × file size = the real hotspot.
package audit

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/nicovegasr/nib-git/internal/pathignore"
)

// Hotspot is one file ranked by change frequency.
type Hotspot struct {
	Path    string
	Commits int // primary signal
	// TODO(lines): +/- churn as a secondary context column.
}

// defaultIgnore are obvious high-churn-by-design files, not code smells.
var defaultIgnore = []string{
	"*.lock",
	"package-lock.json",
	"pnpm-lock.yaml",
	"yarn.lock",
	"dist/",
	"build/",
	"node_modules/",
}

// Run returns files ordered by commit frequency over the last `days` days.
// `exclude` are extra inline excludes on top of the defaults and .auditignore.
func Run(days int, exclude []string) ([]Hotspot, error) {
	ignore, err := loadIgnore()
	if err != nil {
		return nil, err
	}
	out, err := gitLogNames(days)
	if err != nil {
		return nil, err
	}
	return rank(out, append(ignore, exclude...)), nil
}

// loadIgnore returns the defaults plus the optional local .auditignore, which
// shares the .nibignore format.
func loadIgnore() ([]string, error) {
	custom, err := pathignore.Load(".auditignore")
	if err != nil {
		return nil, err
	}
	return append(append([]string{}, defaultIgnore...), custom...), nil
}

// gitLogNames returns the changed file paths (one per line) over the window.
// Package var so tests can stub the shell-out and exercise rank directly.
var gitLogNames = func(days int) (string, error) {
	out, err := exec.Command("git", "log",
		"--since", since(days), "--name-only", "--pretty=format:").Output()
	return string(out), err
}

// rank counts commits per file path, drops ignored paths, and orders by
// descending commit frequency (the hotspot signal), then by path.
func rank(out string, ignore []string) []Hotspot {
	counts := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		path := strings.TrimSpace(line)
		if path == "" || pathignore.Matches(path, ignore) {
			continue
		}
		counts[path]++
	}

	hotspots := make([]Hotspot, 0, len(counts))
	for p, c := range counts {
		hotspots = append(hotspots, Hotspot{Path: p, Commits: c})
	}
	sort.Slice(hotspots, func(i, j int) bool {
		a, b := hotspots[i], hotspots[j]
		if a.Commits != b.Commits {
			return a.Commits > b.Commits
		}
		return a.Path < b.Path
	})
	return hotspots
}

// since formats a git --since window, defaulting to 30 days.
func since(days int) string {
	if days <= 0 {
		days = 30
	}
	return strconv.Itoa(days) + " days ago"
}
