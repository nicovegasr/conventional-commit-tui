// Command nib is a fast, terminal-first commit tool (conventional commits TUI).
//
// Run with no arguments to open the commit flow. Slash-commands (/audit,
// /settings) drive the heavier features. See docs/ux-mockups.html for the
// intended UX.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/nib-git/internal/audit"
	"github.com/nicovegasr/nib-git/internal/cli"
	"github.com/nicovegasr/nib-git/internal/git"
	"github.com/nicovegasr/nib-git/internal/index"
	"github.com/nicovegasr/nib-git/internal/settings"
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
	model := ui.New(gitRepo{}, gitLogger{}, indexHistory{ctx: context.Background()}, fileStore{}, gitAudit{})
	p := tea.NewProgram(model, tea.WithAltScreen()) // the model enables the mouse from its settings
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nib:", err)
		os.Exit(1)
	}
}
