package pathignore

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadIgnoresCommentsAndBlankLines(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".nibignore")
	if err := os.WriteFile(file, []byte("# generated files\n\n*.lock\ndist/\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"*.lock", "dist/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() = %v, want %v", got, want)
	}
}

func TestLoadMissingFileReturnsNoPatterns(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), ".nibignore"))
	if err != nil || len(got) != 0 {
		t.Fatalf("Load() = %v, %v; want no patterns and no error", got, err)
	}
}

func TestLoadRejectsMalformedGlob(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".nibignore")
	if err := os.WriteFile(file, []byte("[broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file); err == nil {
		t.Fatal("Load() accepted malformed glob")
	}
}

func TestMatchesBasenameGlobsAndDirectoryNames(t *testing.T) {
	patterns := []string{"*.lock", "*-lock.yaml", "generated/", "node_modules/"}
	cases := map[string]bool{
		"pnpm-lock.yaml":                true,
		"nested/package.lock":           true,
		"src/generated/output.go":       true,
		"node_modules/pkg/index.js":     true,
		"src/node_modules/pkg/index.js": true,
		"src/generated-code/output.go":  false,
		"src/application.go":            false,
	}
	for path, want := range cases {
		if got := Matches(path, patterns); got != want {
			t.Errorf("Matches(%q) = %v, want %v", path, got, want)
		}
	}
}
