// Package stats implements /stats: a breakdown of commits by conventional
// type (feat, fix, refactor, …) over a time window. This is the same kind of
// view the conventional-stats CLI produces, surfaced inside nib so you never
// leave the tool.
//
// SCAFFOLD: parsing is wired; the TUI render and time grouping are TODO.
package stats

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// TypeCount is the number of commits for one conventional-commit type.
type TypeCount struct {
	Type  string // "feat", "fix", … (prefix before the first ":")
	Count int
}

// ByType counts commits per conventional-commit type over the last `days`
// days on the current branch. Commits whose subject has no "type:" prefix are
// grouped under "other".
//
// TODO(timeline): commits/day or per-week trend.
// TODO(authors): --author breakdown.
func ByType(days int) ([]TypeCount, error) {
	out, err := gitLogSubjects(days)
	if err != nil {
		return nil, err
	}
	return countByType(out), nil
}

// gitLogSubjects returns one commit subject per line. It is a package var so
// tests can stub the shell-out and exercise countByType directly.
var gitLogSubjects = func(days int) (string, error) {
	out, err := exec.Command("git", "log",
		"--since", since(days), "--pretty=format:%s").Output()
	return string(out), err
}

// countByType tallies conventional-commit types from newline-separated
// subjects, sorted by descending count, then by type.
func countByType(out string) []TypeCount {
	counts := map[string]int{}
	for _, subject := range strings.Split(out, "\n") {
		if strings.TrimSpace(subject) == "" {
			continue
		}
		counts[typeOf(subject)]++
	}

	result := make([]TypeCount, 0, len(counts))
	for t, c := range counts {
		result = append(result, TypeCount{Type: t, Count: c})
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Type < b.Type
	})
	return result
}

// typeOf extracts the conventional-commit type from a subject line.
func typeOf(subject string) string {
	subject = strings.TrimSpace(subject)
	if i := strings.Index(subject, ":"); i > 0 {
		t := strings.TrimSpace(subject[:i])
		// strip an optional scope, e.g. "feat(ui)" -> "feat"
		if p := strings.Index(t, "("); p > 0 {
			t = t[:p]
		}
		if t != "" && !strings.ContainsAny(t, " \t") {
			return t
		}
	}
	return "other"
}

// since formats a git --since window, defaulting to 30 days.
func since(days int) string {
	if days <= 0 {
		days = 30
	}
	return strconv.Itoa(days) + " days ago"
}
