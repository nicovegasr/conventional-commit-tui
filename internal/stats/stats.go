// Package stats implements /stats: commits by conventional type and their
// activity over a time window (ADR-0018). It is the same kind of view the
// conventional-stats CLI produces, surfaced inside nib so you never leave the
// tool.
package stats

import (
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Untyped groups the subjects without a "type:" prefix.
const Untyped = "sin tipo"

// TypeCount is the number of commits for one conventional-commit type.
type TypeCount struct {
	Type  string // "feat", "fix", … (prefix before the first ":")
	Count int
}

// Report is the window's commits by type, plus how many fell in each bucket
// of BucketDays days, oldest first and ending today.
type Report struct {
	Types      []TypeCount
	Total      int
	Activity   []int
	BucketDays int
}

// weeklyBeyond is the widest window still shown per day.
const weeklyBeyond = 30

// Run reads the commits of the current branch over the last `days` days.
func Run(days int, now time.Time) (Report, error) {
	out, err := gitLog(days)
	if err != nil {
		return Report{}, err
	}
	return report(out, days, now), nil
}

// gitLog returns "YYYY-MM-DD<TAB>subject" per commit. It is a package var so
// tests can stub the shell-out.
var gitLog = func(days int) (string, error) {
	out, err := exec.Command("git", "log", "--since", strconv.Itoa(days)+" days ago", "--pretty=format:%cs%x09%s").Output()
	return string(out), err
}

func report(out string, days int, now time.Time) Report {
	r := Report{BucketDays: 1}
	if days > weeklyBeyond {
		r.BucketDays = 7
	}
	r.Activity = make([]int, (days+r.BucketDays-1)/r.BucketDays)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	counts := map[string]int{}
	for line := range strings.SplitSeq(out, "\n") {
		date, subject, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		counts[typeOf(subject)]++
		r.Total++
		day, err := time.ParseInLocation(time.DateOnly, date, now.Location())
		if err != nil {
			continue
		}
		age := int(math.Round(today.Sub(day).Hours() / 24)) // DST days are 23 or 25 hours
		if i := len(r.Activity) - 1 - age/r.BucketDays; age >= 0 && i >= 0 {
			r.Activity[i]++
		}
	}
	for t, c := range counts {
		r.Types = append(r.Types, TypeCount{Type: t, Count: c})
	}
	sort.Slice(r.Types, func(i, j int) bool {
		a, b := r.Types[i], r.Types[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Type < b.Type
	})
	return r
}

// typeOf extracts the conventional-commit type from a subject line: the word
// before the first ":", without its scope or breaking-change "!".
func typeOf(subject string) string {
	head, _, ok := strings.Cut(strings.TrimSpace(subject), ":")
	if !ok {
		return Untyped
	}
	head, _, _ = strings.Cut(head, "(")
	head = strings.TrimSuffix(strings.TrimSpace(head), "!")
	if head == "" || strings.ContainsAny(head, " \t") {
		return Untyped
	}
	return head
}
