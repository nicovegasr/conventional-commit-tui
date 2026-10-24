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

	"github.com/nicovegasr/nib-git/internal/cli"
	"github.com/nicovegasr/nib-git/internal/git"
	"github.com/nicovegasr/nib-git/internal/index"
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
	model := ui.New(gitRepo{}, gitLogger{}, indexHistory{ctx: context.Background()})
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nib:", err)
		os.Exit(1)
	}
}
