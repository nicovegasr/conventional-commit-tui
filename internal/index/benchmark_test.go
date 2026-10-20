package index

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/nicovegasr/nib-git/internal/history"
)

func benchmarkSource(count int) fakeSource {
	s := fakeSource{ancestor: true}
	for i := 0; i < count; i++ {
		c := history.Commit{OID: fmt.Sprintf("%040x", i+1), Timestamp: int64(i), Message: "feat: inventory."}
		for _, path := range []string{"inventory.go", fmt.Sprintf("group-%d.go", i%20), fmt.Sprintf("test-%d.go", i%100)} {
			c.Changes = append(c.Changes, history.Change{Path: path, Kind: "M", Added: 1})
		}
		s.commits = append(s.commits, c)
	}
	s.head = s.commits[len(s.commits)-1].OID
	return s
}

func BenchmarkIndex1000(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "index.db"), false)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	src := benchmarkSource(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = s.Update(context.Background(), src, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCoupled1000(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "index.db"), false)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err = s.Update(context.Background(), benchmarkSource(1000), false); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = s.Coupled(context.Background(), "inventory.go", DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
