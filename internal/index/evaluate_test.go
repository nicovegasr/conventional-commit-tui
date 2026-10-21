package index

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/nicovegasr/nib-git/internal/history"
)

func TestScoreCountsEmptySlotsAgainstThePredictor(t *testing.T) {
	cases := []HoldoutCase{
		{Targets: []string{"B", "C"}, Predictions: map[string][]string{"p": {"B", "X", "C"}}},
		{Targets: []string{"D"}, Predictions: map[string][]string{"p": {}}},
	}
	got := score(cases, "p", []int{1, 3})
	want := PredictorResult{Name: "p", Answered: 1, Metrics: []Metrics{
		{K: 1, Hits: 1, Precision: 0.5, Recall: 0.25, Coverage: 0.5},
		{K: 3, Hits: 2, FalsePositives: 1, Precision: 2.0 / 6, Recall: 0.5, Coverage: 0.5},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("score = %+v\nwant  %+v", got, want)
	}
	if empty := score(nil, "p", []int{5}); empty.Metrics[0] != (Metrics{K: 5}) {
		t.Errorf("no cases must score zero, not NaN: %+v", empty)
	}
}

// branchingHistory has a side branch, a merge, an undersized commit and a
// commit that only adds files, so every skip reason and the DAG are exercised.
func branchingHistory() (fakeSource, map[string][]string) {
	type spec struct {
		parents []int
		files   string // upper case: modified, lower case: added
	}
	specs := []spec{
		{nil, "ab"}, {[]int{0}, "AB"}, {[]int{1}, "Ac"}, {[]int{2}, "Bd"},
		{[]int{2}, "ABC"}, {[]int{4, 3}, ""}, {[]int{5}, "ABCD"}, {[]int{6}, "A"},
		{[]int{7}, "xy"}, {[]int{8}, "AB"},
	}
	src := fakeSource{ancestor: true}
	parents := map[string][]string{}
	for i, s := range specs {
		c := history.Commit{OID: fmt.Sprintf("%040x", i+1), Timestamp: int64(i + 1), Message: "change"}
		for _, p := range s.parents {
			c.Parents = append(c.Parents, src.commits[p].OID)
		}
		for _, r := range s.files {
			kind, path := "M", string(r)
			if r >= 'a' {
				kind, path = "A", string(r-'a'+'A')
			}
			c.Changes = append(c.Changes, history.Change{Path: path, Kind: kind})
		}
		parents[c.OID] = c.Parents
		src.commits = append(src.commits, c)
	}
	src.head = src.commits[len(src.commits)-1].OID
	return src, parents
}

func evaluated(t *testing.T, opts EvalOptions) (*Store, Evaluation, fakeSource, map[string][]string) {
	t.Helper()
	s := openStore(t)
	src, parents := branchingHistory()
	if _, err := s.Update(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	e, err := s.Evaluate(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return s, e, src, parents
}

func TestEvaluateSamplesTheFirstParentChain(t *testing.T) {
	opts := DefaultEvalOptions()
	opts.Stride = 1
	_, e, src, _ := evaluated(t, opts)

	if e.Sampled != 9 || e.Skipped != (Skipped{Merges: 1, Size: 1, NoSource: 2}) {
		t.Fatalf("sampled %d, skipped %+v", e.Sampled, e.Skipped)
	}
	var oids []string
	for _, c := range e.Cases {
		oids = append(oids, c.OID)
		if slices.Contains(c.Targets, c.Source) {
			t.Errorf("source leaked into targets: %+v", c)
		}
	}
	want := []string{src.commits[1].OID, src.commits[2].OID, src.commits[4].OID, src.commits[6].OID, src.commits[9].OID}
	if !reflect.DeepEqual(oids, want) {
		t.Errorf("cases %v, want oldest-first %v (the side commit is not on the chain)", oids, want)
	}
}

func TestEvaluateTrainsOnlyOnStrictAncestors(t *testing.T) {
	for _, rank := range []string{RankShared, RankJaccard, RankRecent} {
		t.Run(rank, func(t *testing.T) { assertMatchesStrictAncestorQueries(t, rank) })
	}
}

func assertMatchesStrictAncestorQueries(t *testing.T, rank string) {
	opts := DefaultEvalOptions()
	opts.Stride, opts.MinSupport, opts.Rank, opts.HalfLifeDays = 1, 1, rank, 1
	s, e, src, parents := evaluated(t, opts)

	for _, c := range e.Cases {
		nib, err := s.Coupled(context.Background(), c.Source, Options{MinSupport: 1, MaxFiles: opts.MaxFiles, Limit: 10, Rank: rank, HalfLifeDays: 1, Before: c.OID})
		if err != nil {
			t.Fatal(err)
		}
		cochange, err := s.Coupled(context.Background(), c.Source, Options{MinSupport: 1, MaxFiles: math.MaxInt, Limit: 10, Before: c.OID})
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][]string{
			PredictorNib:       relationshipPaths(nib.Relationships),
			PredictorCochange:  relationshipPaths(cochange.Relationships),
			PredictorFrequency: bruteForceFrequency(src, parents, c.OID, c.Source, opts),
		}
		if !reflect.DeepEqual(c.Predictions, want) {
			t.Errorf("%s/%s predictions %v, want %v", c.OID, c.Source, c.Predictions, want)
		}
	}
}

func bruteForceFrequency(src fakeSource, parents map[string][]string, cutoff, source string, opts EvalOptions) []string {
	ancestors := map[string]bool{}
	var walk func(string)
	walk = func(oid string) {
		for _, p := range parents[oid] {
			if !ancestors[p] {
				ancestors[p] = true
				walk(p)
			}
		}
	}
	walk(cutoff)
	counts := map[string]int{}
	for _, c := range src.commits {
		if ancestors[c.OID] && len(c.Parents) <= 1 && len(c.Changes) <= opts.MaxFiles {
			for _, change := range c.Changes {
				counts[change.Path]++
			}
		}
	}
	paths := []string{}
	for p, n := range counts {
		if p != source && n >= opts.MinSupport {
			paths = append(paths, p)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		if counts[paths[i]] != counts[paths[j]] {
			return counts[paths[i]] > counts[paths[j]]
		}
		return paths[i] < paths[j]
	})
	return paths[:min(len(paths), opts.Limit)]
}

func TestEvaluateSourceFiltersAndIgnoredTargets(t *testing.T) {
	opts := DefaultEvalOptions()
	opts.Stride, opts.Extensions = 1, []string{".go"}
	if _, e, _, _ := evaluated(t, opts); len(e.Cases) != 0 || e.Skipped.NoSource != 7 {
		t.Errorf("extension filter: %+v", e.Skipped)
	}

	opts = DefaultEvalOptions()
	opts.Stride, opts.Prefix, opts.Ignore = 1, "A", []string{"B"}
	_, e, _, _ := evaluated(t, opts)
	if e.Skipped.NoTargets != 2 {
		t.Errorf("commits whose only other path is ignored must be skipped: %+v", e.Skipped)
	}
	for _, c := range e.Cases {
		if c.Source != "A" || slices.Contains(c.Targets, "B") {
			t.Errorf("filters not applied: %+v", c)
		}
	}
}

func TestEvaluateOffsetSkipsTheLatestCommits(t *testing.T) {
	opts := DefaultEvalOptions()
	opts.Stride, opts.Offset, opts.Window = 1, 3, 3
	_, e, src, _ := evaluated(t, opts)
	// Below the 3 latest chain commits come c7, c6 (merge) and c5.
	if e.Sampled != 3 || e.Skipped.Merges != 1 || len(e.Cases) != 2 || e.Cases[1].OID != src.commits[6].OID {
		t.Fatalf("sampled %d skipped %+v cases %+v", e.Sampled, e.Skipped, e.Cases)
	}
}

func TestEvaluateRejectsInvalidOptionsAndEmptyIndex(t *testing.T) {
	for _, mutate := range []func(*EvalOptions){
		func(o *EvalOptions) { o.Window = 0 },
		func(o *EvalOptions) { o.Offset = -1 },
		func(o *EvalOptions) { o.Rank = "best" },
		func(o *EvalOptions) { o.Rank = RankRecent },
	} {
		bad := DefaultEvalOptions()
		mutate(&bad)
		if _, err := openStore(t).Evaluate(context.Background(), bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if _, err := openStore(t).Evaluate(context.Background(), DefaultEvalOptions()); err == nil {
		t.Error("evaluated an empty index")
	}
}
