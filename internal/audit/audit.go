// Package audit implements /audit: change-frequency hotspots.
//
// Primary signal is the NUMBER OF COMMITS that touch a file (Adam Tornhill's
// hotspot method, "Your Code as a Crime Scene"). Lines changed are noisy
// (renames, reformats, lockfiles) so they are only context, never the sort
// key. v0.2 idea: commits × file size = the real hotspot.
package audit

import (
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/nicovegasr/nib-git/internal/pathignore"
)

// Hotspot is one file ranked by change frequency.
type Hotspot struct {
	Path    string
	Commits int // primary signal
	Added   int // lines, context only
	Deleted int
}

// Report is an audit: the ranked hotspots and the patterns that were skipped.
type Report struct {
	Hotspots []Hotspot
	Ignored  []string
}

// Level is how worrying a hotspot's commit count is against the threshold.
type Level int

// Levels, from calm to "refactor?".
const (
	OK       Level = iota // under three quarters of the threshold
	Watch                 // at least three quarters of the threshold
	Refactor              // at or over the threshold
)

// LevelOf rates commits against threshold.
func LevelOf(commits, threshold int) Level {
	switch {
	case commits >= threshold:
		return Refactor
	case commits*4 >= threshold*3:
		return Watch
	}
	return OK
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

// Run returns files ordered by commit frequency over the last `days` days,
// skipping the defaults and .auditignore.
func Run(days int) (Report, error) {
	ignore, err := loadIgnore()
	if err != nil {
		return Report{}, err
	}
	out, err := gitLogNumstat(days)
	if err != nil {
		return Report{}, err
	}
	return Report{Hotspots: rank(out, ignore), Ignored: ignore}, nil
}

// loadIgnore returns the defaults plus the optional local .auditignore, which
// shares the .nibignore format, each pattern once.
func loadIgnore() ([]string, error) {
	custom, err := pathignore.Load(".auditignore")
	if err != nil {
		return nil, err
	}
	ignore := append([]string{}, defaultIgnore...)
	for _, p := range custom {
		if !slices.Contains(ignore, p) {
			ignore = append(ignore, p)
		}
	}
	return ignore, nil
}

// gitLogNumstat returns one "added<TAB>deleted<TAB>path" line per commit and
// file over the window. Package var so tests can stub the shell-out and
// exercise rank directly.
var gitLogNumstat = func(days int) (string, error) {
	out, err := exec.Command("git", "log", "--since", since(days),
		"--numstat", "--no-renames", "--pretty=format:").Output()
	return string(out), err
}

// rank counts commits and lines per path, drops ignored paths, and orders by
// descending commit frequency (the hotspot signal), then by path. Binary
// files ("-" counts) add commits but no lines.
func rank(out string, ignore []string) []Hotspot {
	byPath := map[string]*Hotspot{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || pathignore.Matches(fields[2], ignore) {
			continue
		}
		h := byPath[fields[2]]
		if h == nil {
			h = &Hotspot{Path: fields[2]}
			byPath[fields[2]] = h
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		h.Commits, h.Added, h.Deleted = h.Commits+1, h.Added+added, h.Deleted+deleted
	}

	hotspots := make([]Hotspot, 0, len(byPath))
	for _, h := range byPath {
		hotspots = append(hotspots, *h)
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
