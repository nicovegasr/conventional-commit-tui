// Package cli exposes repository analysis without starting the TUI.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/nicovegasr/nib-git/internal/history"
	"github.com/nicovegasr/nib-git/internal/index"
	"github.com/nicovegasr/nib-git/internal/pathignore"
)

const usage = `Usage:
  nib                                     Open the existing commit TUI
  nib index [--rebuild] [--json]            Index history reachable from HEAD
  nib coupled <file> [options]             Explain historical file co-change
  nib evaluate [options]                   Score coupling on held-out commits

Coupled options:
  --json              Emit a machine-readable report
  --min-support N     Minimum shared eligible commits (default 3)
  --max-files N       Exclude commits changing more than N files (default 50)
  --limit N           Maximum relationships (default 10)
  --evidence N        Commit examples per relationship, 0..100 (default 5)
  --before REV        Use strict ancestors of this commit only (OID, tag, HEAD~3)

Evaluate options (ADR-0009):
  --json              Emit the report with every case and prediction
  --window N          Latest first-parent commits to sample from (default 400)
  --stride N          Hold out every N-th of them (default 3)
  --prefix P          Only pick sources under this path prefix
  --ext .a,.b         Only pick sources with these extensions
  --min-support N     As in coupled (default 3)
  --max-files N       As in coupled (default 50)

Paths are relative to the repository root. Coupling is historical frequency,
not dependency or a risk prediction. .nibignore filters related paths at query
time; changing it does not require reindexing. Run nib index after HEAD changes.
`

// Run dispatches analytical commands from dir. No command starts the TUI here.
func Run(ctx context.Context, dir string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected an analysis command")
	}
	switch args[0] {
	case "help", "--help", "-h":
		_, err := io.WriteString(out, usage)
		return err
	case "index":
		return runIndex(ctx, dir, args[1:], out)
	case "coupled":
		return runCoupled(ctx, dir, args[1:], out)
	case "evaluate":
		return runEvaluate(ctx, dir, args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; run nib --help", args[0])
	}
}

func flags(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() { _, _ = io.WriteString(out, usage) }
	return fs
}

// parseArgs lets flags appear before or after positional arguments, so
// `nib coupled --json a.go` and `nib coupled a.go --json` agree. Everything
// after "--" is positional, for paths that start with a dash.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		consumed := len(args) - fs.NArg()
		if consumed > 0 && args[consumed-1] == "--" {
			return append(positional, fs.Args()...), nil
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// repositoryPath rejects paths that would escape the repository root.
func repositoryPath(file string) (string, error) {
	clean := path.Clean(file)
	if path.IsAbs(file) || file == "" || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("file must be relative to the repository root")
	}
	return clean, nil
}

func runIndex(ctx context.Context, dir string, args []string, out io.Writer) error {
	fs := flags("index", out)
	rebuild := fs.Bool("rebuild", false, "Replace the index atomically")
	asJSON := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		return helpError(err)
	}
	if fs.NArg() != 0 {
		return errors.New("usage: nib index [--rebuild] [--json]")
	}
	repo, err := history.Open(ctx, dir)
	if err != nil {
		return err
	}
	store, err := index.Open(repo.IndexPath(), false)
	if err != nil {
		return err
	}
	result, updateErr := store.Update(ctx, repo, *rebuild)
	closeErr := store.Close()
	if err = errors.Join(updateErr, closeErr); err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	_, err = fmt.Fprintf(out, "Indexed HEAD %s\nCommits: %d (%d new) · Paths: %d\n", result.Head, result.Commits, result.Added, result.Files)
	return err
}

func runCoupled(ctx context.Context, dir string, args []string, out io.Writer) error {
	opts := index.DefaultOptions()
	fs := flags("coupled", out)
	asJSON := fs.Bool("json", false, "Emit JSON")
	fs.IntVar(&opts.MinSupport, "min-support", opts.MinSupport, "Minimum shared eligible commits")
	fs.IntVar(&opts.MaxFiles, "max-files", opts.MaxFiles, "Maximum paths per eligible commit")
	fs.IntVar(&opts.Limit, "limit", opts.Limit, "Maximum relationships")
	fs.IntVar(&opts.Evidence, "evidence", opts.Evidence, "Maximum commit examples")
	fs.StringVar(&opts.Before, "before", "", "Use strict ancestors of this commit only")
	positional, err := parseArgs(fs, args)
	if err != nil {
		return helpError(err)
	}
	if len(positional) != 1 {
		return errors.New("usage: nib coupled <repository-relative-file> [options]")
	}
	file, err := repositoryPath(positional[0])
	if err != nil {
		return err
	}
	if err = opts.Validate(); err != nil {
		return err
	}
	repo, err := history.Open(ctx, dir)
	if err != nil {
		return err
	}
	if opts.Before != "" {
		if opts.Before, err = repo.ResolveCommit(ctx, opts.Before); err != nil {
			return err
		}
	}
	report, err := query(ctx, repo, file, opts)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	return renderReport(out, report)
}

func runEvaluate(ctx context.Context, dir string, args []string, out io.Writer) error {
	opts := index.DefaultEvalOptions()
	fs := flags("evaluate", out)
	asJSON := fs.Bool("json", false, "Emit JSON")
	extensions := fs.String("ext", "", "Comma-separated source extensions")
	fs.IntVar(&opts.Window, "window", opts.Window, "Latest first-parent commits to sample from")
	fs.IntVar(&opts.Stride, "stride", opts.Stride, "Hold out every N-th commit")
	fs.StringVar(&opts.Prefix, "prefix", "", "Source path prefix")
	fs.IntVar(&opts.MinSupport, "min-support", opts.MinSupport, "Minimum shared eligible commits")
	fs.IntVar(&opts.MaxFiles, "max-files", opts.MaxFiles, "Maximum paths per eligible commit")
	positional, err := parseArgs(fs, args)
	if err != nil {
		return helpError(err)
	}
	if len(positional) != 0 {
		return errors.New("usage: nib evaluate [options]")
	}
	if *extensions != "" {
		opts.Extensions = strings.Split(*extensions, ",")
	}
	if err = opts.Validate(); err != nil {
		return err
	}
	repo, err := history.Open(ctx, dir)
	if err != nil {
		return err
	}
	var evaluation index.Evaluation
	err = withFreshIndex(repo, func(store *index.Store, ignore []string) error {
		opts.Ignore = ignore
		var evalErr error
		evaluation, evalErr = store.Evaluate(ctx, opts)
		return evalErr
	})
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(evaluation)
	}
	return renderEvaluation(out, evaluation)
}

func query(ctx context.Context, repo history.Repository, file string, opts index.Options) (index.Report, error) {
	var report index.Report
	err := withFreshIndex(repo, func(store *index.Store, ignore []string) error {
		opts.Ignore = ignore
		var err error
		report, err = store.Coupled(ctx, file, opts)
		return err
	})
	return report, err
}

// withFreshIndex opens the index read-only, rejects a stale or foreign
// snapshot and passes the current .nibignore rules to read.
func withFreshIndex(repo history.Repository, read func(*index.Store, []string) error) error {
	ignore, err := pathignore.Load(filepath.Join(repo.Root, ".nibignore"))
	if err != nil {
		return err
	}
	store, err := index.Open(repo.IndexPath(), true)
	if err != nil {
		return err
	}
	if err = store.Check(repo.Identity(), repo.Head); err != nil {
		return errors.Join(err, store.Close())
	}
	return errors.Join(read(store, ignore), store.Close())
}

func helpError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func renderReport(out io.Writer, r index.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Historical co-change: %q\nSnapshot: %s\nEligible commits: %d · Excluded: %d\nFilters: support >= %d, paths/commit <= %d\n", r.Path, r.Head, r.Eligible, r.Excluded, r.Options.MinSupport, r.Options.MaxFiles)
	if len(r.Options.Ignore) == 0 {
		b.WriteString("Ignored paths: none\n\n")
	} else {
		fmt.Fprintf(&b, "Ignored paths: %s\n\n", strings.Join(r.Options.Ignore, ", "))
	}
	if len(r.Relationships) == 0 {
		b.WriteString("No relationships meet these filters.\n")
	}
	for _, relation := range r.Relationships {
		fmt.Fprintf(&b, "%.1f%%  %d/%d  %q\n", relation.Ratio*100, relation.Shared, r.Eligible, relation.Path)
		for _, e := range relation.Evidence {
			fmt.Fprintf(&b, "  %s  %q\n", e.OID, e.Subject)
		}
	}
	b.WriteString("\nHistorical co-change does not establish dependency or predict regressions.\n")
	_, err := io.WriteString(out, b.String())
	return err
}

func renderEvaluation(out io.Writer, e index.Evaluation) error {
	var b strings.Builder
	o := e.Options
	fmt.Fprintf(&b, "Held-out evaluation · snapshot %s\n", e.Head)
	fmt.Fprintf(&b, "Sample: every %d of the latest %d first-parent commits", o.Stride, o.Window)
	if o.Prefix != "" || len(o.Extensions) > 0 {
		fmt.Fprintf(&b, " · sources %q %s", o.Prefix, strings.Join(o.Extensions, ","))
	}
	fmt.Fprintf(&b, "\nCases: %d of %d sampled · skipped: %d merges, %d by size, %d without source, %d without targets\n", len(e.Cases), e.Sampled, e.Skipped.Merges, e.Skipped.Size, e.Skipped.NoSource, e.Skipped.NoTargets)
	fmt.Fprintf(&b, "Filters: support >= %d, paths/commit <= %d\n\n", o.MinSupport, o.MaxFiles)
	fmt.Fprintf(&b, "%-10s %4s %6s %6s %10s %8s %9s\n", "predictor", "k", "hits", "fp", "precision", "recall", "coverage")
	for _, p := range e.Predictors {
		for _, m := range p.Metrics {
			fmt.Fprintf(&b, "%-10s %4d %6d %6d %9.1f%% %7.1f%% %8.1f%%\n", p.Name, m.K, m.Hits, m.FalsePositives, m.Precision*100, m.Recall*100, m.Coverage*100)
		}
	}
	b.WriteString("\nnib = coupled with filters · cochange = unfiltered co-change · frequency = global file frequency.\n")
	b.WriteString("One repository's sample is evidence to report, not a general claim (ADR-0009).\n")
	_, err := io.WriteString(out, b.String())
	return err
}
