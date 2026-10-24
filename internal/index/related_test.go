package index

import (
	"context"
	"reflect"
	"testing"
)

func TestCoupledWithAnyKeepsEachPathsStrongestRelationship(t *testing.T) {
	s := linear(t, "AB", "AB", "AC", "DE", "CD")
	opts := DefaultOptions()
	opts.Rank = RankShared

	got, err := s.CoupledWithAny(context.Background(), []string{"A", "D", "new.go"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, r := range got {
		paths = append(paths, r.Path)
	}
	// B shares 2 commits with A; C and E share 1. Sources never appear.
	if want := []string{"B", "C", "E"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths %v, want %v", paths, want)
	}
	// C co-changed in 1 of A's 3 commits and 1 of D's 2: the tie keeps D's.
	if got[1].Ratio != 0.5 {
		t.Errorf("C ratio %v, want 0.5", got[1].Ratio)
	}

	opts.Limit = 1
	if got, _ := s.CoupledWithAny(context.Background(), []string{"A", "D"}, opts); len(got) != 1 {
		t.Errorf("limit ignored: %+v", got)
	}
}

func TestCoupledWithAnyWithoutHistoryIsEmpty(t *testing.T) {
	s := linear(t, "AB")
	got, err := s.CoupledWithAny(context.Background(), []string{"new.go"}, DefaultOptions())
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := s.CoupledWithAny(context.Background(), nil, Options{}); err == nil {
		t.Error("accepted invalid options")
	}
}
