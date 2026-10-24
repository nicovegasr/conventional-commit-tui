package index

import (
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/nicovegasr/nib-git/internal/pathignore"
	bolt "go.etcd.io/bbolt"
)

// Ranking modes order related paths (ADR-0010). All of them keep MinSupport
// on shared commits; an empty mode means RankShared.
const (
	RankShared  = "shared"  // most shared eligible commits
	RankJaccard = "jaccard" // shared / eligible commits touching either path
	RankRecent  = "recent"  // shared commits weighted by a half-life in days
)

// Options defines the eligible population, ranking and output bounds.
type Options struct {
	MinSupport   int      `json:"min_support"`
	MaxFiles     int      `json:"max_files"`
	Limit        int      `json:"limit"`
	Evidence     int      `json:"evidence_limit"`
	Rank         string   `json:"rank,omitempty"`
	HalfLifeDays int      `json:"half_life_days,omitempty"`
	Before       string   `json:"before,omitempty"`
	Ignore       []string `json:"ignore_patterns,omitempty"`
}

// ErrIgnoredPath indicates that a query's source is excluded by .nibignore.
var ErrIgnoredPath = errors.New("path is excluded by .nibignore")

// ErrNoHistory indicates a path the index has never seen.
var ErrNoHistory = errors.New("path has no indexed history (paths are repository-relative; renames remain separate)")

// DefaultOptions are the configuration chosen on held-out tuning windows and
// confirmed on newer validation windows (ADR-0010): every observation counts,
// mass changes are excluded and recent co-changes weigh more.
func DefaultOptions() Options {
	return Options{MinSupport: 1, MaxFiles: 50, Limit: 10, Evidence: 5, Rank: RankRecent, HalfLifeDays: 90}
}

// Validate rejects invalid populations, rankings and unbounded evidence output.
func (o Options) Validate() error {
	if o.MinSupport < 1 || o.MaxFiles < 1 || o.Limit < 1 || o.Evidence < 0 || o.Evidence > 100 {
		return errors.New("support, max-files and limit must be positive; evidence must be 0..100")
	}
	switch o.Rank {
	case "", RankShared, RankJaccard:
		return nil
	case RankRecent:
		if o.HalfLifeDays < 1 {
			return errors.New("rank recent needs a positive half-life in days")
		}
		return nil
	}
	return errors.New("rank must be shared, jaccard or recent")
}

const secondsPerDay = 24 * 60 * 60

// weight is how much one observation counts towards RankRecent: it halves
// every HalfLifeDays before the reference time.
func (o Options) weight(c storedCommit, now int64) float64 {
	if o.Rank != RankRecent {
		return 1
	}
	ageDays := float64(max(now-c.Timestamp, 0)) / secondsPerDay
	return math.Pow(0.5, ageDays/float64(o.HalfLifeDays))
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
	Score    float64    `json:"score"`
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
	weight   float64
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
			return ErrNoHistory
		}
		ancestors, err := ancestorsBefore(ctx, tx, opts.Before)
		if err != nil {
			return err
		}
		reference, err := commitByOID(tx, cmp.Or(opts.Before, m.Head))
		if err != nil {
			return err
		}
		obs, err := observeSource(ctx, tx, fid, opts, ancestors, reference.Timestamp)
		if err != nil {
			return err
		}
		r.Eligible, r.Excluded = obs.eligible, obs.excluded
		var totals map[uint64]int
		if opts.Rank == RankJaccard {
			if totals, err = eligibleTotals(ctx, tx, obs.candidates, opts, ancestors); err != nil {
				return err
			}
		}
		r.Relationships = rankRelationships(tx, obs.candidates, r.Eligible, opts, func(id uint64) int { return totals[id] })
		return nil
	})
	return r, err
}

// sourceObservations are the co-changes seen across one source's postings.
type sourceObservations struct {
	eligible, excluded int
	candidates         map[uint64]*candidate
}

// eligibleTotals counts each supported candidate's own eligible commits, the
// union term RankJaccard needs.
func eligibleTotals(ctx context.Context, tx *bolt.Tx, candidates map[uint64]*candidate, opts Options, ancestors map[string]struct{}) (map[uint64]int, error) {
	totals := map[uint64]int{}
	for id, c := range candidates {
		if c.shared < opts.MinSupport {
			continue
		}
		err := postings(tx, key(id), func(commit storedCommit) error {
			if isBeforeCutoff(commit, ancestors) && eligibleObservation(commit, opts) {
				totals[id]++
			}
			return ctx.Err()
		})
		if err != nil {
			return nil, err
		}
	}
	return totals, nil
}

// observeSource scans the source's postings, restricted to ancestors when the
// set is non-nil. now is the reference time of RankRecent.
func observeSource(ctx context.Context, tx *bolt.Tx, fid []byte, opts Options, ancestors map[string]struct{}, now int64) (sourceObservations, error) {
	obs := sourceObservations{candidates: map[uint64]*candidate{}}
	sourceID := binary.BigEndian.Uint64(fid)
	err := postings(tx, fid, func(c storedCommit) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !isBeforeCutoff(c, ancestors) {
			return nil
		}
		if !eligibleObservation(c, opts) {
			obs.excluded++
			return nil
		}
		obs.eligible++
		collectCandidates(tx, sourceID, c, opts, obs.candidates, opts.weight(c, now))
		return nil
	})
	return obs, err
}

func eligibleObservation(c storedCommit, opts Options) bool {
	return len(c.Parents) <= 1 && len(c.Files) <= opts.MaxFiles
}

func collectCandidates(tx *bolt.Tx, sourceID uint64, c storedCommit, opts Options, candidates map[uint64]*candidate, weight float64) {
	for _, f := range c.Files {
		if f.FileID == sourceID {
			continue
		}
		candidatePath := string(bucket(tx, "paths").Get(key(f.FileID)))
		if !pathignore.Matches(candidatePath, opts.Ignore) {
			observe(candidates, f.FileID, c, opts.Evidence, weight)
		}
	}
}

func isBeforeCutoff(c storedCommit, ancestors map[string]struct{}) bool {
	if ancestors == nil {
		return true
	}
	_, ok := ancestors[c.OID]
	return ok
}

// ancestorsBefore returns the strict ancestor set of a cutoff commit. The
// cutoff itself is excluded so its changed files remain suitable for holdout
// evaluation.
func ancestorsBefore(ctx context.Context, tx *bolt.Tx, cutoff string) (map[string]struct{}, error) {
	if cutoff == "" {
		return nil, nil
	}
	cutoffCommit, err := commitByOID(tx, cutoff)
	if err != nil {
		return nil, errors.New("cutoff commit is not in the indexed history")
	}
	seen := make(map[string]struct{})
	return seen, extendAncestors(ctx, tx, seen, cutoffCommit.Parents, func(storedCommit) {})
}

// extendAncestors adds to seen every commit reachable from parents that it
// does not hold yet, reporting each newly seen commit to visit. Growing one set
// along a first-parent chain therefore traverses the history only once.
func extendAncestors(ctx context.Context, tx *bolt.Tx, seen map[string]struct{}, parents []string, visit func(storedCommit)) error {
	stack := append([]string(nil), parents...)
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(stack) - 1
		oid := stack[last]
		stack = stack[:last]
		if _, ok := seen[oid]; ok {
			continue
		}
		commit, err := commitByOID(tx, oid)
		if err != nil {
			return errors.New("cutoff ancestry is incomplete in the index")
		}
		seen[oid] = struct{}{}
		visit(commit)
		stack = append(stack, commit.Parents...)
	}
	return nil
}

// rankRelationships orders supported candidates by the ranking mode's score.
// total is a candidate's own eligible-commit count; only RankJaccard reads it.
func rankRelationships(tx *bolt.Tx, candidates map[uint64]*candidate, eligible int, opts Options, total func(uint64) int) []Relationship {
	relationships := []Relationship{}
	for id, c := range candidates {
		if c.shared < opts.MinSupport {
			continue
		}
		r := Relationship{Path: string(bucket(tx, "paths").Get(key(id))), Shared: c.shared, Ratio: float64(c.shared) / float64(eligible), Evidence: c.evidence}
		switch opts.Rank {
		case RankJaccard:
			r.Score = float64(c.shared) / float64(eligible+total(id)-c.shared)
		case RankRecent:
			r.Score = c.weight
		default:
			r.Score = float64(c.shared)
		}
		relationships = append(relationships, r)
	}
	sort.Slice(relationships, func(i, j int) bool {
		a, b := relationships[i], relationships[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
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

func observe(candidates map[uint64]*candidate, id uint64, c storedCommit, limit int, weight float64) {
	item := candidates[id]
	if item == nil {
		item = &candidate{evidence: []Evidence{}}
		candidates[id] = item
	}
	item.shared++
	item.weight += weight
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
