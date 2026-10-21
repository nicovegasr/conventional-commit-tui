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

Coupled options:
  --json              Emit a machine-readable report
  --min-support N     Minimum shared eligible commits (default 3)
  --max-files N       Exclude commits changing more than N files (default 50)
  --limit N           Maximum relationships (default 10)
  --evidence N        Commit examples per relationship, 0..100 (default 5)
  --before OID        Use strict ancestors of this indexed commit only

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
	default:
		return fmt.Errorf("unknown command %q; run nib --help", args[0])
	}
}

func flags(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
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
	if len(args) == 0 {
		return errors.New("usage: nib coupled <repository-relative-file> [options]")
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := io.WriteString(out, usage)
		return err
	}
	file := args[0]
	if path.IsAbs(file) || file == "" || path.Clean(file) == ".." || strings.HasPrefix(path.Clean(file), "../") {
		return errors.New("file must be relative to the repository root")
	}
	opts := index.DefaultOptions()
	fs := flags("coupled", out)
	asJSON := fs.Bool("json", false, "Emit JSON")
	fs.IntVar(&opts.MinSupport, "min-support", opts.MinSupport, "Minimum shared eligible commits")
	fs.IntVar(&opts.MaxFiles, "max-files", opts.MaxFiles, "Maximum paths per eligible commit")
	fs.IntVar(&opts.Limit, "limit", opts.Limit, "Maximum relationships")
	fs.IntVar(&opts.Evidence, "evidence", opts.Evidence, "Maximum commit examples")
	fs.StringVar(&opts.Before, "before", "", "Use strict ancestors of this indexed commit only")
	if err := fs.Parse(args[1:]); err != nil {
		return helpError(err)
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected argument after coupled options")
	}
	if err := opts.Validate(); err != nil {
		return err
	}
	repo, err := history.Open(ctx, dir)
	if err != nil {
		return err
	}
	report, err := query(ctx, repo, path.Clean(file), opts)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	return renderReport(out, report)
}

func query(ctx context.Context, repo history.Repository, file string, opts index.Options) (index.Report, error) {
	ignored, err := pathignore.Load(filepath.Join(repo.Root, ".nibignore"))
	if err != nil {
		return index.Report{}, err
	}
	opts.Ignore = ignored
	store, err := index.Open(repo.IndexPath(), true)
	if err != nil {
		return index.Report{}, err
	}
	if err = store.Check(repo.Identity(), repo.Head); err != nil {
		return index.Report{}, errors.Join(err, store.Close())
	}
	report, queryErr := store.Coupled(ctx, file, opts)
	return report, errors.Join(queryErr, store.Close())
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
