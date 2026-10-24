// Package settings holds user-toggleable options. Philosophy: "rich by
// default, everything heavy can be switched off". They are persisted as TOML
// and edited live from /settings (ADR-0015).
package settings

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
)

// Git-log scopes.
const (
	ScopeCurrent = "current"
	ScopeAll     = "all"
)

// ReloadChoices are the auto-reload intervals in seconds; 0 turns it off.
var ReloadChoices = []int{0, 1, 2, 5, 10}

// Settings is the persisted configuration.
type Settings struct {
	ShowLog       bool   `toml:"show_log"`       // git-log panel visible (off => leaner/faster)
	ShowRelated   bool   `toml:"show_related"`   // related-files panel visible (ADR-0014)
	MouseCapture  bool   `toml:"mouse_capture"`  // off restores native terminal selection/scroll
	LogScope      string `toml:"log_scope"`      // ScopeCurrent | ScopeAll
	ReloadSeconds int    `toml:"reload_seconds"` // one of ReloadChoices

	AuditDays      int `toml:"audit_days"`      // default window for /audit
	AuditThreshold int `toml:"audit_threshold"` // commit count that flags a file as "refactor?"
}

// Default returns the out-of-the-box configuration: rich, with everything on.
func Default() Settings {
	return Settings{
		ShowLog:        true,
		ShowRelated:    true,
		MouseCapture:   true,
		LogScope:       ScopeCurrent,
		ReloadSeconds:  2,
		AuditDays:      30,
		AuditThreshold: 20,
	}
}

// Path is where the settings live: $XDG_CONFIG_HOME/nib/settings.toml, else
// ~/.config/nib/settings.toml.
func Path() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "nib", "settings.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "nib", "settings.toml"), nil
}

// Load reads path over Default(): a missing file or key keeps the default, and
// an out-of-range value falls back to it.
func Load(path string) (Settings, error) {
	s := Default()
	if _, err := toml.DecodeFile(path, &s); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Default(), err
	}
	d := Default()
	if s.LogScope != ScopeCurrent && s.LogScope != ScopeAll {
		s.LogScope = d.LogScope
	}
	if !slices.Contains(ReloadChoices, s.ReloadSeconds) {
		s.ReloadSeconds = d.ReloadSeconds
	}
	return s, nil
}

// Save writes s to path, creating its directory.
func Save(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(s); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// NextReload is the interval after seconds in ReloadChoices, wrapping to off.
func NextReload(seconds int) int {
	i := slices.Index(ReloadChoices, seconds)
	return ReloadChoices[(i+1)%len(ReloadChoices)]
}
