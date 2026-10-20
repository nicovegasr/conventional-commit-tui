// Command nib is a fast, terminal-first commit tool (conventional commits TUI).
//
// Run with no arguments to open the commit flow. Slash-commands (/audit,
// /settings) drive the heavier features. See docs/ux-mockups.html for the
// intended UX.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nicovegasr/nib-git/internal/cli"
	"github.com/nicovegasr/nib-git/internal/git"
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
	model := ui.New(gitRepo{}, gitLogger{})
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "nib:", err)
		os.Exit(1)
	}
}
