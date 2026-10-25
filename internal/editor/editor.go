// Package editor picks the user's editor and opens a file at a line in it
// (ADR-0017). nib never edits files itself.
package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// fallbacks are tried in order when neither $VISUAL nor $EDITOR is set:
// micro and nano behave like a regular text box, vi is always there.
var fallbacks = []string{"micro", "nano"}

// Package vars so tests can stub the environment.
var (
	getenv   = os.Getenv
	lookPath = exec.LookPath
)

// Command opens path at line in the user's editor. tea.ExecProcess attaches
// it to the terminal.
func Command(path string, line int) *exec.Cmd {
	argv := Argv(path, line)
	return exec.Command(argv[0], argv[1:]...)
}

// Argv is the command line that opens path at line. $EDITOR may carry its own
// flags ("code --wait"); the line goes in each editor's own syntax.
func Argv(path string, line int) []string {
	argv := strings.Fields(getenv("VISUAL"))
	if len(argv) == 0 {
		argv = strings.Fields(getenv("EDITOR"))
	}
	if len(argv) == 0 {
		argv = []string{installed()}
	}
	at := path + ":" + strconv.Itoa(line)
	switch filepath.Base(argv[0]) {
	case "code", "cursor":
		return append(argv, "-g", at)
	case "hx", "helix", "subl", "zed":
		return append(argv, at)
	}
	return append(argv, "+"+strconv.Itoa(line), path)
}

func installed() string {
	for _, name := range fallbacks {
		if _, err := lookPath(name); err == nil {
			return name
		}
	}
	return "vi"
}
