package index

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestEvidenceStableAcrossTraversalOrder(t *testing.T) {
	s := openStore(t)
	src := golden()
	if _, err := s.Update(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	opts := Options{MinSupport: 1, MaxFiles: 50, Limit: 10, Evidence: 2}
	before, err := s.Coupled(context.Background(), "A", opts)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(src.commits)-1; i < j; i, j = i+1, j-1 {
		src.commits[i], src.commits[j] = src.commits[j], src.commits[i]
	}
	if _, err = s.Update(context.Background(), src, true); err != nil {
		t.Fatal(err)
	}
	after, err := s.Coupled(context.Background(), "A", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("evidence depends on ingestion order: %+v %+v", before, after)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Coupled(ctx, "A", opts); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
