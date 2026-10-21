package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nicovegasr/nib-git/internal/index"
)

func TestHelpDoesNotNeedRepository(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), "missing", []string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "nib coupled") {
		t.Fatal(out.String())
	}
}
func TestInvalidCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"missing"}, {"index", "extra"}, {"index", "--bad"}, {"coupled"}, {"coupled", "../outside"}, {"coupled", "/absolute"}, {"coupled", "A", "extra"}, {"coupled", "--json"}, {"coupled", "--limit", "0", "A"}} {
		if err := Run(context.Background(), "missing", args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func TestCoupledHelpPrintsUsage(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), "missing", []string{"coupled", "A", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "--before REV") {
		t.Fatal(out.String())
	}
}
func TestParseArgsAcceptsFlagsAnywhere(t *testing.T) {
	for _, args := range [][]string{{"--json", "a.go"}, {"a.go", "--json"}, {"--json", "--", "a.go"}} {
		fs := flags("test", &bytes.Buffer{})
		asJSON := fs.Bool("json", false, "")
		positional, err := parseArgs(fs, args)
		if err != nil || !*asJSON || len(positional) != 1 || positional[0] != "a.go" {
			t.Errorf("parseArgs(%v) = %v, %v, json=%v", args, positional, err, *asJSON)
		}
	}
	fs := flags("test", &bytes.Buffer{})
	fs.Bool("json", false, "")
	if positional, _ := parseArgs(fs, []string{"--", "--json"}); len(positional) != 1 || positional[0] != "--json" {
		t.Errorf("after -- a dash path is positional, got %v", positional)
	}
}
func TestRepositoryPathCleansAndRejectsEscapes(t *testing.T) {
	if got, err := repositoryPath("./src//a.go"); err != nil || got != "src/a.go" {
		t.Errorf("repositoryPath = %q, %v", got, err)
	}
	for _, file := range []string{"", "..", "../x", "/abs", "a/../../x"} {
		if _, err := repositoryPath(file); err == nil {
			t.Errorf("accepted %q", file)
		}
	}
}
func TestReportShowsDenominatorAndEscapesPaths(t *testing.T) {
	opts := index.DefaultOptions()
	opts.Ignore = []string{"*.lock", "dist/"}
	r := index.Report{Path: "A\nB", Eligible: 4, Excluded: 2, Options: opts, Relationships: []index.Relationship{{Path: "C", Shared: 3, Ratio: 0.75, Evidence: []index.Evidence{{OID: "abc", Subject: "change\n"}}}}}
	var out bytes.Buffer
	if err := renderReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"A\nB"`, "75.0%  3/4", "Excluded: 2", "Ignored paths: *.lock, dist/", "abc"} {
		if !strings.Contains(out.String(), part) {
			t.Errorf("missing %q: %s", part, out.String())
		}
	}
}
