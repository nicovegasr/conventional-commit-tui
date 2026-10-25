package stats

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTypeOf(t *testing.T) {
	cases := map[string]string{
		"feat: add x":          "feat",
		"fix(ui): crash":       "fix",
		"  refactor: tidy  ":   "refactor",
		"docs(readme): update": "docs",
		"feat!: break":         "feat",
		"no prefix here":       Untyped,
		"Feat/audit (#13)":     Untyped,
		"":                     Untyped,
		":empty type":          Untyped,
		"two words: nope":      Untyped,
	}
	for subject, want := range cases {
		if got := typeOf(subject); got != want {
			t.Errorf("typeOf(%q) = %q, want %q", subject, got, want)
		}
	}
}

// now is a fixed clock: Saturday 25 October 2026, mid-afternoon.
var now = time.Date(2026, 10, 25, 15, 0, 0, 0, time.Local)

func TestReportCountsTypesByDescendingCountThenName(t *testing.T) {
	out := "2026-10-25\tfeat: a\n2026-10-24\tfix: b\n2026-10-24\tfeat: c\n2026-10-20\tchore: e\n2026-10-20\tfix: f\n2026-10-19\tdocs: g\n"
	got := report(out, 30, now)
	want := []TypeCount{{"feat", 2}, {"fix", 2}, {"chore", 1}, {"docs", 1}}
	if !reflect.DeepEqual(got.Types, want) || got.Total != 6 {
		t.Errorf("Types = %+v (total %d), want %+v", got.Types, got.Total, want)
	}
}

func TestReportBucketsDailyUpToThirtyDays(t *testing.T) {
	got := report("2026-10-25\tfeat: a\n2026-10-25\tfix: b\n2026-10-24\tfeat: c\n2026-10-19\tfeat: d\n", 7, now)
	if got.BucketDays != 1 {
		t.Fatalf("BucketDays = %d, want 1", got.BucketDays)
	}
	want := []int{1, 0, 0, 0, 0, 1, 2} // 19 Oct … 25 Oct, today last
	if !reflect.DeepEqual(got.Activity, want) {
		t.Errorf("Activity = %v, want %v", got.Activity, want)
	}
}

func TestReportBucketsWeeklyBeyondThirtyDays(t *testing.T) {
	got := report("2026-10-25\tfeat: a\n2026-10-19\tfeat: b\n2026-10-18\tfeat: c\n2026-07-28\tfeat: old\n", 90, now)
	if got.BucketDays != 7 || len(got.Activity) != 13 {
		t.Fatalf("BucketDays %d with %d buckets, want 7 and 13", got.BucketDays, len(got.Activity))
	}
	last := got.Activity[len(got.Activity)-2:]
	if !reflect.DeepEqual(last, []int{1, 2}) {
		t.Errorf("last two weeks = %v, want [1 2]", last)
	}
	if got.Activity[0] != 1 {
		t.Errorf("the oldest week = %d, want 1", got.Activity[0])
	}
}

func TestReportSkipsBlankAndUndatedLines(t *testing.T) {
	got := report("\n\nnot-a-date\tfeat: x\n2025-01-01\tfeat: too old\n", 30, now)
	if got.Total != 2 || len(got.Types) != 1 {
		t.Errorf("an undated commit still counts by type: %+v", got)
	}
	for _, n := range got.Activity {
		if n != 0 {
			t.Errorf("dates outside the window must not land in a bucket: %v", got.Activity)
		}
	}
}

func TestRunReadsTheWindowFromGit(t *testing.T) {
	orig := gitLog
	t.Cleanup(func() { gitLog = orig })

	var gotDays int
	gitLog = func(days int) (string, error) { gotDays = days; return "2026-10-25\tfeat: a\n", nil }
	got, err := Run(7, now)
	if err != nil || gotDays != 7 || got.Total != 1 {
		t.Errorf("Run = %+v, %v (asked %d days)", got, err, gotDays)
	}

	gitLog = func(int) (string, error) { return "", errors.New("boom") }
	if _, err := Run(7, now); err == nil {
		t.Error("expected the error to propagate")
	}
}
