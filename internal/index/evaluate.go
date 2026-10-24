package index

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/nicovegasr/nib-git/internal/pathignore"
	bolt "go.etcd.io/bbolt"
)

// Cutoffs are the ranking depths every predictor is scored at.
var Cutoffs = []int{1, 3, 5, 10}

// Predictor names, in report order (ADR-0009).
const (
	PredictorNib       = "nib"
	PredictorCochange  = "cochange"
	PredictorFrequency = "frequency"
)

// EvalOptions fixes the held-out sample before any result is seen. The
// embedded Options configure the predictor only, never the cases (ADR-0012).
type EvalOptions struct {
	Options
	Window       int      `json:"window"`
	Offset       int      `json:"offset,omitempty"`
	Stride       int      `json:"stride"`
	CaseMaxFiles int      `json:"case_max_files"`
	Prefix       string   `json:"source_prefix,omitempty"`
	Extensions   []string `json:"source_extensions,omitempty"`
}

// DefaultEvalOptions samples every third of the latest 400 non-merge commits,
// cases of up to 50 paths, with Nib's default filters.
func DefaultEvalOptions() EvalOptions {
	opts := DefaultOptions()
	opts.Limit = Cutoffs[len(Cutoffs)-1]
	opts.Evidence = 0
	return EvalOptions{Options: opts, Window: 400, Stride: 3, CaseMaxFiles: 50}
}

// Validate rejects an empty sample.
func (o EvalOptions) Validate() error {
	if o.Window < 1 || o.Stride < 1 || o.Offset < 0 {
		return errors.New("window and stride must be positive; offset must not be negative")
	}
	if o.CaseMaxFiles < 2 {
		return errors.New("case-max-files must be at least 2")
	}
	return o.Options.Validate()
}

// HoldoutCase is one held-out commit, auditable with git show <oid>.
type HoldoutCase struct {
	OID         string              `json:"oid"`
	Source      string              `json:"source"`
	Targets     []string            `json:"targets"`
	Predictions map[string][]string `json:"predictions"`
}

// Skipped counts sampled commits that could not become a case, by reason.
type Skipped struct {
	Size      int `json:"size"`
	NoSource  int `json:"no_source"`
	NoTargets int `json:"no_targets"`
}

// Metrics scores one predictor at one cutoff.
type Metrics struct {
	K              int     `json:"k"`
	Hits           int     `json:"hits"`
	FalsePositives int     `json:"false_positives"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"mean_recall"`
	Coverage       float64 `json:"coverage"`
}

// PredictorResult scores one predictor at every cutoff.
type PredictorResult struct {
	Name     string    `json:"name"`
	Answered int       `json:"answered_cases"`
	Metrics  []Metrics `json:"metrics"`
}

// Evaluation is the full, reproducible experiment report.
type Evaluation struct {
	Head       string            `json:"indexed_head"`
	Options    EvalOptions       `json:"options"`
	Sampled    int               `json:"sampled_commits"`
	Skipped    Skipped           `json:"skipped"`
	Predictors []PredictorResult `json:"predictors"`
	Cases      []HoldoutCase     `json:"cases"`
}

// Evaluate holds out sampled commits and asks each predictor, trained only on
// strict ancestors, which other files the commit changed (ADR-0009, ADR-0012).
func (s *Store) Evaluate(ctx context.Context, opts EvalOptions) (Evaluation, error) {
	e := Evaluation{Options: opts, Cases: []HoldoutCase{}}
	if err := opts.Validate(); err != nil {
		return e, err
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		m, err := metadata(tx)
		if err != nil {
			return err
		}
		if m.Head == "" {
			return errors.New("empty index; run nib index")
		}
		e.Head = m.Head
		sample, err := sampleCommits(ctx, tx, opts)
		if err != nil {
			return err
		}
		e.Sampled = len(sample)
		h := newTrainingSet(opts.Options)
		for _, commit := range sample {
			if err := h.advanceTo(ctx, tx, commit); err != nil {
				return err
			}
			c, ok := h.holdout(tx, commit, opts, &e.Skipped)
			if !ok {
				continue
			}
			if c.Predictions, err = h.predict(ctx, tx, c.Source, commit.Timestamp); err != nil {
				return err
			}
			e.Cases = append(e.Cases, c)
		}
		return nil
	})
	for _, name := range []string{PredictorNib, PredictorCochange, PredictorFrequency} {
		e.Predictors = append(e.Predictors, score(e.Cases, name, Cutoffs))
	}
	return e, err
}

// sampleCommits holds out every stride-th commit of the window, in index
// order: parents before children, so the training set mostly grows.
func sampleCommits(ctx context.Context, tx *bolt.Tx, opts EvalOptions) ([]storedCommit, error) {
	window, err := sampleWindow(ctx, tx, opts.Offset, opts.Window)
	var sample []storedCommit
	for i := 0; i < len(window); i += opts.Stride {
		sample = append(sample, window[i])
	}
	slices.SortFunc(sample, func(a, b storedCommit) int { return cmp.Compare(a.ID, b.ID) })
	return sample, err
}

// sampleWindow returns up to window non-merge commits below the offset latest
// ones. The index holds exactly the indexed HEAD's history; ordering by author
// time and OID keeps the window independent of how the index was built.
func sampleWindow(ctx context.Context, tx *bolt.Tx, offset, window int) ([]storedCommit, error) {
	var commits []storedCommit
	err := bucket(tx, "commits").ForEach(func(id, _ []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		c, err := readCommit(tx, id)
		if err == nil && len(c.Parents) <= 1 {
			commits = append(commits, c)
		}
		return err
	})
	slices.SortFunc(commits, func(a, b storedCommit) int {
		return cmp.Or(cmp.Compare(a.Timestamp, b.Timestamp), strings.Compare(a.OID, b.OID))
	})
	end := max(len(commits)-offset, 0)
	return commits[max(end-window, 0):end], err
}

// trainingSet is what the current held-out commit may learn from: its strict
// ancestors and their global file frequencies.
type trainingSet struct {
	opts      Options
	ancestors map[string]struct{}
	frequency map[uint64]*candidate
	previous  string
}

func newTrainingSet(opts Options) *trainingSet {
	h := &trainingSet{opts: opts}
	h.reset()
	return h
}

func (h *trainingSet) reset() {
	h.ancestors, h.frequency = map[string]struct{}{}, map[uint64]*candidate{}
}

// advanceTo makes the set the strict ancestors of c. It grows when c descends
// from the previous sample and is rebuilt when c is on another branch.
func (h *trainingSet) advanceTo(ctx context.Context, tx *bolt.Tx, c storedCommit) error {
	if err := h.extend(ctx, tx, c.Parents); err != nil {
		return err
	}
	if _, descends := h.ancestors[h.previous]; h.previous != "" && !descends {
		h.reset()
		if err := h.extend(ctx, tx, c.Parents); err != nil {
			return err
		}
	}
	h.previous = c.OID
	return nil
}

func (h *trainingSet) extend(ctx context.Context, tx *bolt.Tx, parents []string) error {
	return extendAncestors(ctx, tx, h.ancestors, parents, func(ancestor storedCommit) {
		if !eligibleObservation(ancestor, h.opts) {
			return
		}
		for _, f := range ancestor.Files {
			if !pathignore.Matches(string(bucket(tx, "paths").Get(key(f.FileID))), h.opts.Ignore) {
				observe(h.frequency, f.FileID, ancestor, 0, 1)
			}
		}
	})
}

// holdout turns a sampled commit into a case, or records why it cannot be one.
func (h *trainingSet) holdout(tx *bolt.Tx, c storedCommit, opts EvalOptions, skipped *Skipped) (HoldoutCase, bool) {
	if len(c.Files) < 2 || len(c.Files) > opts.CaseMaxFiles {
		skipped.Size++
		return HoldoutCase{}, false
	}
	var sources, paths []string
	for _, f := range c.Files {
		p := string(bucket(tx, "paths").Get(key(f.FileID)))
		if pathignore.Matches(p, opts.Ignore) {
			continue
		}
		paths = append(paths, p)
		if f.Kind == "M" && opts.isSource(p) {
			sources = append(sources, p)
		}
	}
	if len(sources) == 0 {
		skipped.NoSource++
		return HoldoutCase{}, false
	}
	source := pickSource(c.OID, sources)
	targets := slices.DeleteFunc(paths, func(p string) bool { return p == source })
	if len(targets) == 0 {
		skipped.NoTargets++
		return HoldoutCase{}, false
	}
	slices.Sort(targets)
	return HoldoutCase{OID: c.OID, Source: source, Targets: targets}, true
}

func (o EvalOptions) isSource(p string) bool {
	return strings.HasPrefix(p, o.Prefix) && (len(o.Extensions) == 0 || slices.Contains(o.Extensions, path.Ext(p)))
}

// pickSource chooses deterministically from the OID, never by the analyst.
func pickSource(oid string, sources []string) string {
	slices.Sort(sources)
	digest := sha256.Sum256([]byte(oid))
	return sources[binary.BigEndian.Uint64(digest[:8])%uint64(len(sources))]
}

// predict ranks with Nib's options and both baselines. Jaccard's candidate
// totals are exactly the global eligible frequencies already accumulated.
func (h *trainingSet) predict(ctx context.Context, tx *bolt.Tx, source string, now int64) (map[string][]string, error) {
	fid := bucket(tx, "files").Get([]byte(source))
	unfiltered := Options{MinSupport: 1, MaxFiles: math.MaxInt, Limit: h.opts.Limit, Rank: RankShared, Ignore: h.opts.Ignore}
	predictions := map[string][]string{}
	for name, opts := range map[string]Options{PredictorNib: h.opts, PredictorCochange: unfiltered} {
		obs, err := observeSource(ctx, tx, fid, opts, h.ancestors, now)
		if err != nil {
			return nil, err
		}
		predictions[name] = relationshipPaths(rankRelationships(tx, obs.candidates, obs.eligible, opts, h.total))
	}
	predictions[PredictorFrequency] = h.mostFrequent(tx, source)
	return predictions, nil
}

// mostFrequent ranks one extra path so that dropping the source still fills
// the limit. The ratio denominator is irrelevant: only the order is used.
func (h *trainingSet) mostFrequent(tx *bolt.Tx, source string) []string {
	opts := h.opts
	opts.Limit++
	opts.Rank = RankShared
	paths := relationshipPaths(rankRelationships(tx, h.frequency, 1, opts, h.total))
	paths = slices.DeleteFunc(paths, func(p string) bool { return p == source })
	return paths[:min(len(paths), h.opts.Limit)]
}

func (h *trainingSet) total(id uint64) int {
	if c := h.frequency[id]; c != nil {
		return c.shared
	}
	return 0
}

func relationshipPaths(relationships []Relationship) []string {
	paths := make([]string, len(relationships))
	for i, r := range relationships {
		paths[i] = r.Path
	}
	return paths
}

// score aggregates one predictor's ranked predictions over every case.
func score(cases []HoldoutCase, name string, cutoffs []int) PredictorResult {
	result := PredictorResult{Name: name}
	for _, c := range cases {
		if len(c.Predictions[name]) > 0 {
			result.Answered++
		}
	}
	for _, k := range cutoffs {
		m := Metrics{K: k}
		var recall float64
		covered := 0
		for _, c := range cases {
			predicted := c.Predictions[name][:min(k, len(c.Predictions[name]))]
			hits := 0
			for _, p := range predicted {
				if slices.Contains(c.Targets, p) {
					hits++
				}
			}
			m.Hits += hits
			m.FalsePositives += len(predicted) - hits
			recall += float64(hits) / float64(len(c.Targets))
			if hits > 0 {
				covered++
			}
		}
		if n := len(cases); n > 0 {
			m.Precision = float64(m.Hits) / float64(k*n)
			m.Recall = recall / float64(n)
			m.Coverage = float64(covered) / float64(n)
		}
		result.Metrics = append(result.Metrics, m)
	}
	return result
}
