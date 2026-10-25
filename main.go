// Command nib is a fast, terminal-first commit tool (conventional commits TUI).
//
// Run with no arguments to open the commit flow. Slash-commands
// (/changes, /stats, /audit, /settings) drive the heavier features. See docs/ux-mockups.html for the
// intended UX.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/nib-git/internal/audit"
	"github.com/nicovegasr/nib-git/internal/cli"
	"github.com/nicovegasr/nib-git/internal/editor"
	"github.com/nicovegasr/nib-git/internal/git"
	"github.com/nicovegasr/nib-git/internal/index"
	"github.com/nicovegasr/nib-git/internal/settings"
	"github.com/nicovegasr/nib-git/internal/stats"
	"github.com/nicovegasr/nib-git/internal/ui"
)

// gitRepo adapts the git package to the ui.Repo port injected into the model.
type gitRepo struct{}

func (gitRepo) Status() ([]git.FileChange, error) { return git.Status() }
func (gitRepo) Stage(files []string) error        { return git.Stage(files) }
func (gitRepo) Commit(subject string) error       { return git.Commit(subject) }

// gitLogger adapts the git package to the ui.Logger port injected into the model.
type gitLogger struct{}

func (gitLogger) Log(scope string, limit int) (string, error) { return git.Log(scope, limit) }

// indexHistory adapts the history index to the ui.History port.
type indexHistory struct{ ctx context.Context }

func (h indexHistory) Refresh() error { return cli.Refresh(h.ctx, ".") }

func (h indexHistory) Related(files []string, limit int) ([]ui.Related, error) {
	relationships, err := cli.Related(h.ctx, ".", files, limit)
	if errors.Is(err, index.ErrStale) {
		return nil, ui.ErrStaleHistory
	}
	related := make([]ui.Related, len(relationships))
	for i, r := range relationships {
		related[i] = ui.Related{Path: r.Path, Ratio: r.Ratio}
	}
	return related, err
}

// gitAudit adapts the audit package to the ui.Auditor port.
type gitAudit struct{}

func (gitAudit) Audit(days int) (audit.Report, error) { return audit.Run(days) }

// gitStats adapts the stats package to the ui.StatsReader port.
type gitStats struct{}

func (gitStats) Stats(days int) (stats.Report, error) { return stats.Run(days, time.Now()) }

// gitDiffer adapts the git package to the ui.Differ port.
type gitDiffer struct{}

func (gitDiffer) Diff(f git.FileChange) (git.Diff, error) { return git.DiffFile(f) }

// userEditor adapts the editor package to the ui.Editor port.
type userEditor struct{}

func (userEditor) Command(path string, line int) *exec.Cmd { return editor.Command(path, line) }

// fileStore adapts the settings file to the ui.Store port.
type fileStore struct{}

func (fileStore) Load() (settings.Settings, error) {
	path, err := settings.Path()
	if err != nil {
		return settings.Default(), err
	}
	return settings.Load(path)
}

func (fileStore) Save(s settings.Settings) error {
	path, err := settings.Path()
	if err != nil {
		return err
	}
	return settings.Save(path, s)
}

// moveToRepoRoot makes the commit TUI work from any subdirectory: the paths
// it stages and diffs are relative to the repository root.
func moveToRepoRoot() error {
	root, err := git.Root()
	if err != nil {
		return err
	}
	return os.Chdir(root)
}

func main() {
	if len(os.Args) > 1 {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := cli.Run(ctx, ".", os.Args[1:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "nib:", err)
			os.Exit(1)
		}
		return
	}
	if err := moveToRepoRoot(); err != nil {
		fmt.Fprintln(os.Stderr, "nib:", err)
		os.Exit(1)
	}
	model := ui.New(gitRepo{}, gitLogger{}, indexHistory{ctx: context.Background()}, fileStore{}, gitAudit{}, gitStats{}, gitDiffer{}, userEditor{})
	p := tea.NewProgram(model, tea.WithAltScreen()) // the model enables the mouse from its settings
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nib:", err)
		os.Exit(1)
	}
}
