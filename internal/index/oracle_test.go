package index

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/nicovegasr/conventional-commit-tui/internal/history"
)

// The oracle deliberately scans every commit instead of sharing the indexed
// algorithm. This catches denominator and eligibility mistakes independently.
func bruteForce(commits []history.Commit, source string, maxFiles int) (int, map[string]int) {
	eligible := 0
	shared := map[string]int{}
	for _, c := range commits {
		if len(c.Parents) > 1 || len(c.Changes) > maxFiles {
			continue
		}
		found := false
		for _, change := range c.Changes {
			if change.Path == source {
				found = true
			}
		}
		if !found {
			continue
		}
		eligible++
		for _, change := range c.Changes {
			if change.Path != source {
				shared[change.Path]++
			}
		}
	}
	return eligible, shared
}

func TestIndexedCouplingMatchesBruteForce(t *testing.T) {
	for seed := int64(0); seed < 12; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			src := fakeSource{ancestor: true}
			for i := 0; i < 50; i++ {
				c := history.Commit{OID: fmt.Sprintf("%040x", i+1), Timestamp: int64(rng.Intn(10)), Message: "change"}
				for j := 0; j < 10; j++ {
					if rng.Intn(3) == 0 {
						c.Changes = append(c.Changes, history.Change{Path: fmt.Sprint(j), Kind: "M"})
					}
				}
				if i%7 == 0 {
					c.Parents = []string{"parent1", "parent2"}
				}
				src.commits = append(src.commits, c)
			}
			src.head = src.commits[len(src.commits)-1].OID
			s := openStore(t)
			if _, err := s.Update(context.Background(), src, false); err != nil {
				t.Fatal(err)
			}
			for j := 0; j < 10; j++ {
				path := fmt.Sprint(j)
				r, err := s.Coupled(context.Background(), path, Options{MinSupport: 1, MaxFiles: 4, Limit: 20, Evidence: 2})
				if err != nil {
					t.Fatal(err)
				}
				eligible, shared := bruteForce(src.commits, path, 4)
				if r.Eligible != eligible || len(r.Relationships) != len(shared) {
					t.Fatalf("source %s: got %+v, want eligible %d, shared %v", path, r, eligible, shared)
				}
				for _, relation := range r.Relationships {
					want := shared[relation.Path]
					if relation.Shared != want || relation.Ratio != float64(want)/float64(eligible) {
						t.Fatalf("%+v, want %d/%d", relation, want, eligible)
					}
				}
			}
		})
	}
}
