package index

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/nicovegasr/conventional-commit-tui/internal/history"
)

// linear indexes commits of the given paths, one per day, A always first.
func linear(t *testing.T, commits ...string) *Store {
	t.Helper()
	src := fakeSource{ancestor: true}
	for i, paths := range commits {
		c := history.Commit{OID: fmt.Sprintf("%040x", i+1), Timestamp: int64(i) * secondsPerDay}
		if i > 0 {
			c.Parents = []string{src.commits[i-1].OID}
		}
		for _, p := range paths {
			c.Changes = append(c.Changes, history.Change{Path: string(p), Kind: "M"})
		}
		src.commits = append(src.commits, c)
	}
	src.head = src.commits[len(src.commits)-1].OID
	s := openStore(t)
	if _, err := s.Update(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	return s
}

func rankedPaths(t *testing.T, s *Store, opts Options) []string {
	t.Helper()
	r, err := s.Coupled(context.Background(), "A", opts)
	if err != nil {
		t.Fatal(err)
	}
	return relationshipPaths(r.Relationships)
}

func TestJaccardDemotesFilesThatChangeWithEverything(t *testing.T) {
	// B and C each share 2 commits with A, but B also changes alone 3 times.
	s := linear(t, "AB", "AB", "AC", "AC", "B", "B", "B")
	opts := Options{MinSupport: 1, MaxFiles: 50, Limit: 10}
	if got := rankedPaths(t, s, opts); !reflect.DeepEqual(got, []string{"B", "C"}) {
		t.Errorf("shared = %v, ties by path", got)
	}
	opts.Rank = RankJaccard
	if got := rankedPaths(t, s, opts); !reflect.DeepEqual(got, []string{"C", "B"}) {
		t.Errorf("jaccard = %v, want C (2/4) above B (2/7)", got)
	}
}

func TestRecentPrefersNewerCoChanges(t *testing.T) {
	// B shares 3 old commits with A; C shares 2 recent ones.
	s := linear(t, "AB", "AB", "AB", "A", "A", "A", "A", "A", "A", "AC", "AC")
	opts := Options{MinSupport: 1, MaxFiles: 50, Limit: 10}
	if got := rankedPaths(t, s, opts); got[0] != "B" {
		t.Errorf("shared = %v, want B first", got)
	}
	opts.Rank, opts.HalfLifeDays = RankRecent, 2
	if got := rankedPaths(t, s, opts); got[0] != "C" {
		t.Errorf("recent = %v, want C first", got)
	}
}
