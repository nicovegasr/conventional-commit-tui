package index

import (
	"context"
	"errors"
	"sort"
)

// CoupledWithAny ranks the paths that co-changed with any source, keeping each
// path's strongest relationship (ADR-0014). Sources without history or excluded
// by .nibignore are skipped, and sources never appear in the result.
func (s *Store) CoupledWithAny(ctx context.Context, sources []string, opts Options) ([]Relationship, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	best := map[string]Relationship{}
	for _, source := range sources {
		r, err := s.Coupled(ctx, source, opts)
		if errors.Is(err, ErrNoHistory) || errors.Is(err, ErrIgnoredPath) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, rel := range r.Relationships {
			if current, ok := best[rel.Path]; !ok || stronger(rel, current) {
				best[rel.Path] = rel
			}
		}
	}
	for _, source := range sources {
		delete(best, source)
	}
	related := make([]Relationship, 0, len(best))
	for _, rel := range best {
		related = append(related, rel)
	}
	sort.Slice(related, func(i, j int) bool {
		if stronger(related[i], related[j]) != stronger(related[j], related[i]) {
			return stronger(related[i], related[j])
		}
		return related[i].Path < related[j].Path
	})
	return related[:min(len(related), opts.Limit)], nil
}

// stronger orders relationships of different sources: scores share a unit
// (weighted co-changes), so the ADR-0010 ranking still applies.
func stronger(a, b Relationship) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Shared != b.Shared {
		return a.Shared > b.Shared
	}
	return a.Ratio > b.Ratio
}
