package stats

import (
	"errors"
	"testing"
)

func TestTypeOf(t *testing.T) {
	cases := map[string]string{
		"feat: add x":          "feat",
		"fix(ui): crash":       "fix",
		"  refactor: tidy  ":   "refactor",
		"docs(readme): update": "docs",
		"no prefix here":       "other",
		"":                     "other",
		":empty type":          "other",
		"two words: nope":      "other",
	}
	for subject, want := range cases {
		if got := typeOf(subject); got != want {
			t.Errorf("typeOf(%q) = %q, want %q", subject, got, want)
		}
	}
}

func TestCountByTypeSortsDescending(t *testing.T) {
	out := "feat: a\nfix: b\nfeat: c\nfeat: d\nchore: e\nfix: f\n"
	got := countByType(out)

	if len(got) != 3 {
		t.Fatalf("expected 3 types, got %d: %+v", len(got), got)
	}
	if got[0].Type != "feat" || got[0].Count != 3 {
		t.Errorf("top = %+v, want feat:3", got[0])
	}
	// descending order invariant
	for i := 1; i < len(got); i++ {
		if got[i-1].Count < got[i].Count {
			t.Errorf("not sorted descending: %+v", got)
		}
	}
}

func TestCountByTypeIgnoresBlankLines(t *testing.T) {
	if got := countByType("\n\n  \n"); len(got) != 0 {
		t.Errorf("blank input should yield no counts, got %+v", got)
	}
}

func TestSince(t *testing.T) {
	cases := map[int]string{0: "30 days ago", -5: "30 days ago", 7: "7 days ago", 30: "30 days ago"}
	for in, want := range cases {
		if got := since(in); got != want {
			t.Errorf("since(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestByTypeUsesStubbedLog(t *testing.T) {
	orig := gitLogSubjects
	t.Cleanup(func() { gitLogSubjects = orig })

	gitLogSubjects = func(_ int) (string, error) { return "feat: a\nfix: b\n", nil }
	got, err := ByType(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 types, got %+v", got)
	}

	gitLogSubjects = func(_ int) (string, error) { return "", errors.New("boom") }
	if _, err := ByType(30); err == nil {
		t.Error("expected error to propagate")
	}
}

func TestCountByTypeBreaksTiesByType(t *testing.T) {
	got := countByType("fix: a.\nfeat: b.\n")
	if got[0].Type != "feat" || got[1].Type != "fix" {
		t.Errorf("ties must be ordered by type, got %+v", got)
	}
}
