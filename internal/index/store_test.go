package index

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nicovegasr/nib-git/internal/history"
)

type fakeSource struct {
	commits  []history.Commit
	head     string
	ancestor bool
	fail     error
}

func (s fakeSource) Identity() string                                 { return "test-repo" }
func (s fakeSource) Revision() string                                 { return s.head }
func (s fakeSource) IsAncestor(context.Context, string) (bool, error) { return s.ancestor, nil }
func (s fakeSource) Walk(_ context.Context, after string, visit func(history.Commit) error) error {
	start := after == ""
	for _, c := range s.commits {
		if start {
			if err := visit(c); err != nil {
				return err
			}
		}
		if c.OID == after {
			start = true
		}
	}
	return s.fail
}
func golden() fakeSource {
	s := fakeSource{ancestor: true}
	for i, paths := range [][]string{{"A"}, {"A", "B"}, {"A", "B"}, {"B"}, {"A", "B", "C"}} {
		c := history.Commit{OID: fmt.Sprintf("%040x", i+1), Timestamp: int64(i + 1), Message: "change"}
		for _, p := range paths {
			c.Changes = append(c.Changes, history.Change{Path: p, Kind: "M", Added: 1})
		}
		s.commits = append(s.commits, c)
	}
	s.head = s.commits[len(s.commits)-1].OID
	return s
}
func openStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "index.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func TestGoldenDirectionalEvidence(t *testing.T) {
	s := openStore(t)
	src := golden()
	result, err := s.Update(context.Background(), src, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 5 || result.Commits != 5 || result.Files != 3 {
		t.Fatalf("%+v", result)
	}
	opts := Options{MinSupport: 1, MaxFiles: 50, Limit: 10, Evidence: 2}
	a, err := s.Coupled(context.Background(), "A", opts)
	if err != nil {
		t.Fatal(err)
	}
	if a.Eligible != 4 || len(a.Relationships) != 2 || a.Relationships[0].Path != "B" || a.Relationships[0].Shared != 3 || a.Relationships[0].Ratio != 0.75 || len(a.Relationships[0].Evidence) != 2 {
		t.Fatalf("%+v", a)
	}
	c, err := s.Coupled(context.Background(), "C", opts)
	if err != nil {
		t.Fatal(err)
	}
	if c.Eligible != 1 || c.Relationships[0].Ratio != 1 {
		t.Fatalf("%+v", c)
	}
}
func TestIncrementalEqualsRebuildAndIdempotent(t *testing.T) {
	s := openStore(t)
	src := golden()
	first := src
	first.commits = src.commits[:3]
	first.head = first.commits[2].OID
	if _, err := s.Update(context.Background(), first, false); err != nil {
		t.Fatal(err)
	}
	r, err := s.Update(context.Background(), src, false)
	if err != nil || r.Added != 2 {
		t.Fatalf("%+v %v", r, err)
	}
	opts := Options{MinSupport: 1, MaxFiles: 50, Limit: 10, Evidence: 10}
	before, err := s.Coupled(context.Background(), "A", opts)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Update(context.Background(), src, false)
	if err != nil || r.Added != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = s.Update(context.Background(), src, true); err != nil {
		t.Fatal(err)
	}
	after, err := s.Coupled(context.Background(), "A", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("incremental %+v != rebuild %+v", before, after)
	}
}
func TestRollbackRewriteAndIdentity(t *testing.T) {
	s := openStore(t)
	src := golden()
	if _, err := s.Update(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	broken := src
	broken.head = "new"
	broken.fail = errors.New("interrupted")
	if _, err := s.Update(context.Background(), broken, true); err == nil {
		t.Fatal("want failed rebuild")
	}
	m, err := s.Metadata()
	if err != nil || m.Head != src.head || m.Commits != 5 {
		t.Fatalf("%+v %v", m, err)
	}
	broken.fail = nil
	broken.ancestor = false
	if _, err := s.Update(context.Background(), broken, false); !errors.Is(err, ErrRewrite) {
		t.Fatalf("%v", err)
	}
	if err = s.Check("other", src.head); err == nil {
		t.Fatal("identity mismatch accepted")
	}
	if err = s.Check(src.Identity(), "new"); !errors.Is(err, ErrStale) {
		t.Fatalf("%v", err)
	}
}
func TestMassChangesAndMergesExcluded(t *testing.T) {
	s := openStore(t)
	src := golden()
	src.commits[0].Parents = []string{"p1", "p2"}
	if _, err := s.Update(context.Background(), src, false); err != nil {
		t.Fatal(err)
	}
	r, err := s.Coupled(context.Background(), "A", Options{MinSupport: 2, MaxFiles: 2, Limit: 10, Evidence: 10})
	if err != nil {
		t.Fatal(err)
	}
	if r.Eligible != 2 || r.Excluded != 2 || len(r.Relationships) != 1 || r.Relationships[0].Ratio != 1 {
		t.Fatalf("%+v", r)
	}
	if _, err = s.Coupled(context.Background(), "missing", DefaultOptions()); err == nil {
		t.Fatal("unknown file accepted")
	}
	if _, err = s.Coupled(context.Background(), "A", Options{}); err == nil {
		t.Fatal("invalid options accepted")
	}
}
func TestReopenAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	s, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Update(ctx, golden(), false); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if _, err = s.Update(context.Background(), golden(), false); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if err = r.Check("test-repo", golden().head); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Update(context.Background(), golden(), false); err == nil {
		t.Fatal("readonly write accepted")
	}
}
