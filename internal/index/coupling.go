package index

import (
	"context"
	"encoding/binary"
	"errors"
	"sort"
	"strings"

	"github.com/nicovegasr/nib-git/internal/pathignore"
	bolt "go.etcd.io/bbolt"
)

// Options defines the eligible population and output bounds.
type Options struct {
	MinSupport int      `json:"min_support"`
	MaxFiles   int      `json:"max_files"`
	Limit      int      `json:"limit"`
	Evidence   int      `json:"evidence_limit"`
	Ignore     []string `json:"ignore_patterns,omitempty"`
}

// ErrIgnoredPath indicates that a query's source is excluded by .nibignore.
var ErrIgnoredPath = errors.New("path is excluded by .nibignore")

// DefaultOptions suppresses single-observation and mass-change relationships.
func DefaultOptions() Options { return Options{MinSupport: 3, MaxFiles: 50, Limit: 10, Evidence: 5} }

// Validate rejects invalid populations and unbounded evidence output.
func (o Options) Validate() error {
	if o.MinSupport < 1 || o.MaxFiles < 1 || o.Limit < 1 || o.Evidence < 0 || o.Evidence > 100 {
		return errors.New("support, max-files and limit must be positive; evidence must be 0..100")
	}
	return nil
}

// Evidence makes an observation inspectable using git show <oid>.
type Evidence struct {
	OID       string `json:"oid"`
	Timestamp int64  `json:"timestamp"`
	Subject   string `json:"subject"`
}

// Relationship is directional historical frequency, not dependency or risk.
type Relationship struct {
	Path     string     `json:"path"`
	Shared   int        `json:"shared_commits"`
	Ratio    float64    `json:"cochange_ratio"`
	Evidence []Evidence `json:"evidence"`
}

// Report includes the denominator and exclusions needed to interpret ratios.
type Report struct {
	Head          string         `json:"indexed_head"`
	Path          string         `json:"path"`
	Eligible      int            `json:"eligible_commits"`
	Excluded      int            `json:"excluded_commits"`
	Options       Options        `json:"filters"`
	Relationships []Relationship `json:"relationships"`
}

type candidate struct {
	shared   int
	evidence []Evidence
}

// Coupled traverses only source postings and their changed-file lists.
func (s *Store) Coupled(ctx context.Context, path string, opts Options) (Report, error) {
	r := Report{Path: path, Options: opts, Relationships: []Relationship{}}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if err := opts.Validate(); err != nil {
		return r, err
	}
	if pathignore.Matches(path, opts.Ignore) {
		return r, ErrIgnoredPath
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		m, err := metadata(tx)
		if err != nil {
			return err
		}
		if m.Head == "" {
			return errors.New("empty index; run nib index")
		}
		r.Head = m.Head
		fid := bucket(tx, "files").Get([]byte(path))
		if fid == nil {
			return errors.New("path has no indexed history (paths are repository-relative; renames remain separate)")
		}
		sourceID := binary.BigEndian.Uint64(fid)
		candidates := map[uint64]*candidate{}
		err = postings(tx, fid, func(c storedCommit) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(c.Parents) > 1 || len(c.Files) > opts.MaxFiles {
				r.Excluded++
				return nil
			}
			r.Eligible++
			for _, f := range c.Files {
				if f.FileID == sourceID {
					continue
				}
				candidatePath := string(bucket(tx, "paths").Get(key(f.FileID)))
				if !pathignore.Matches(candidatePath, opts.Ignore) {
					observe(candidates, f.FileID, c, opts.Evidence)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		r.Relationships = rankRelationships(tx, candidates, r.Eligible, opts)
		return nil
	})
	return r, err
}

func rankRelationships(tx *bolt.Tx, candidates map[uint64]*candidate, eligible int, opts Options) []Relationship {
	relationships := []Relationship{}
	for id, c := range candidates {
		if c.shared < opts.MinSupport {
			continue
		}
		relationships = append(relationships, Relationship{Path: string(bucket(tx, "paths").Get(key(id))), Shared: c.shared, Ratio: float64(c.shared) / float64(eligible), Evidence: c.evidence})
	}
	sort.Slice(relationships, func(i, j int) bool {
		a, b := relationships[i], relationships[j]
		if a.Shared != b.Shared {
			return a.Shared > b.Shared
		}
		return a.Path < b.Path
	})
	if len(relationships) > opts.Limit {
		relationships = relationships[:opts.Limit]
	}
	return relationships
}

func observe(candidates map[uint64]*candidate, id uint64, c storedCommit, limit int) {
	item := candidates[id]
	if item == nil {
		item = &candidate{evidence: []Evidence{}}
		candidates[id] = item
	}
	item.shared++
	if limit == 0 {
		return
	}
	e := Evidence{OID: c.OID, Timestamp: c.Timestamp, Subject: strings.SplitN(c.Message, "\n", 2)[0]}
	item.evidence = append(item.evidence, e)
	// Timestamp and OID provide stable evidence even when incremental ingestion
	// visits merged side branches in a different order than a full rebuild.
	sort.Slice(item.evidence, func(i, j int) bool {
		a, b := item.evidence[i], item.evidence[j]
		if a.Timestamp != b.Timestamp {
			return a.Timestamp > b.Timestamp
		}
		return a.OID < b.OID
	})
	if len(item.evidence) > limit {
		item.evidence = item.evidence[:limit]
	}
}
