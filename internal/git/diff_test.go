package git

import (
	"errors"
	"reflect"
	"testing"
)

const modifiedDiff = `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,4 +1,4 @@
 package a
-	old()
+	new()
+	more()
 }
\ No newline at end of file
`

func TestParseDiffNumbersEveryLine(t *testing.T) {
	got := parseDiff(modifiedDiff)
	want := Diff{Lines: []DiffLine{
		{Kind: Context, Text: "package a", Old: 1, New: 1},
		{Kind: Removed, Text: "\told()", Old: 2, New: 2},
		{Kind: Added, Text: "\tnew()", Old: 3, New: 2},
		{Kind: Added, Text: "\tmore()", Old: 3, New: 3},
		{Kind: Context, Text: "}", Old: 3, New: 4},
	}, Added: 2, Removed: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDiff =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseDiffFollowsHunkHeaders(t *testing.T) {
	got := parseDiff("@@ -10,2 +12,2 @@ func x() {\n a\n+b\n@@ -40 +43 @@\n-c\n")
	news := []int{12, 13, 43}
	for i, l := range got.Lines {
		if l.New != news[i] {
			t.Errorf("line %d New = %d, want %d", i, l.New, news[i])
		}
	}
	if got.Lines[2].Old != 40 {
		t.Errorf("removed Old = %d, want 40", got.Lines[2].Old)
	}
}

func TestParseDiffFlagsBinaryFiles(t *testing.T) {
	got := parseDiff("diff --git a/x.png b/x.png\nBinary files a/x.png and b/x.png differ\n")
	if !got.Binary || len(got.Lines) != 0 {
		t.Errorf("parseDiff = %+v, want binary with no lines", got)
	}
}

func TestDiffAgainstHeadWithTheWholeFile(t *testing.T) {
	var gotArgs []string
	stubRun(t, func(args ...string) (string, error) { gotArgs = args; return modifiedDiff, nil })

	got, err := DiffFile(FileChange{Path: "a.go", Status: 'M'})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"diff", "HEAD", "--no-color", "--no-ext-diff", "--unified=1000000", "--", "a.go"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
	if got.Added != 2 {
		t.Errorf("Added = %d, want 2", got.Added)
	}
}

func TestDiffUntrackedAgainstDevNull(t *testing.T) {
	var gotArgs []string
	stubRun(t, func(args ...string) (string, error) {
		gotArgs = args
		return "@@ -0,0 +1 @@\n+hi\n", errors.New("exit status 1") // --no-index exits 1 on a difference
	})

	got, err := DiffFile(FileChange{Path: "n.txt", Status: '?'})
	if err != nil {
		t.Fatalf("a difference is not an error: %v", err)
	}
	want := []string{"diff", "--no-index", "--no-color", "--no-ext-diff", "--unified=1000000", "--", "/dev/null", "n.txt"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
	if len(got.Lines) != 1 || got.Lines[0].Kind != Added {
		t.Errorf("lines = %+v", got.Lines)
	}
}

func TestDiffPropagatesErrors(t *testing.T) {
	stubRun(t, func(_ ...string) (string, error) { return "", errors.New("bad HEAD") })
	for _, f := range []FileChange{{Path: "a.go", Status: 'M'}, {Path: "n.txt", Status: '?'}} {
		if _, err := DiffFile(f); err == nil {
			t.Errorf("%s: expected the error", f.Path)
		}
	}
}
