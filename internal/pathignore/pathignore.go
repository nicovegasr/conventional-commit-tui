// Package pathignore loads the small glob format used by repository analyses.
package pathignore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Load reads one glob per line. Blank lines and lines beginning with # are ignored.
func Load(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignore file: %w", err)
	}

	var patterns []string
	for line, raw := range strings.Split(string(data), "\n") {
		pattern := strings.TrimSpace(raw)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		check := strings.TrimSuffix(pattern, "/")
		if _, err := filepath.Match(check, ""); err != nil {
			return nil, fmt.Errorf("ignore file line %d: %w", line+1, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

// Matches supports basename globs and literal directory names ending in /.
func Matches(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.HasSuffix(pattern, "/") {
			directory := strings.TrimSuffix(pattern, "/")
			if path == directory || strings.HasPrefix(path, directory+"/") || strings.Contains(path, "/"+directory+"/") {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}
