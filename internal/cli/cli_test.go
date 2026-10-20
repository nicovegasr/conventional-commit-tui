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
	for _, args := range [][]string{nil, {"missing"}, {"index", "extra"}, {"index", "--bad"}, {"coupled"}, {"coupled", "../outside"}, {"coupled", "/absolute"}, {"coupled", "A", "extra"}} {
		if err := Run(context.Background(), "missing", args, &bytes.Buffer{}); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func TestReportShowsDenominatorAndEscapesPaths(t *testing.T) {
	r := index.Report{Path: "A\nB", Eligible: 4, Excluded: 2, Options: index.DefaultOptions(), Relationships: []index.Relationship{{Path: "C", Shared: 3, Ratio: 0.75, Evidence: []index.Evidence{{OID: "abc", Subject: "change\n"}}}}}
	var out bytes.Buffer
	if err := renderReport(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"A\nB"`, "75.0%  3/4", "Excluded: 2", "abc"} {
		if !strings.Contains(out.String(), part) {
			t.Errorf("missing %q: %s", part, out.String())
		}
	}
}
