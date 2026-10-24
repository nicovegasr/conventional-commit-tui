package git

import (
	"strconv"
	"strings"
)

// LineKind tells a diff line apart: unchanged, added or removed.
type LineKind int

// The kinds of diff line.
const (
	Context LineKind = iota
	Added
	Removed
)

// DiffLine is one line of a file's diff. Old and New are its line numbers in
// each version; a line missing from one side keeps the number of the next line
// there, which is where an editor should land.
type DiffLine struct {
	Kind     LineKind
	Text     string
	Old, New int
}

// Diff is a whole file against HEAD.
type Diff struct {
	Lines          []DiffLine
	Added, Removed int
	Binary         bool
}

// wholeFile is a context wide enough to show every line of any real file.
const wholeFile = "--unified=1000000"

// DiffFile diffs the working copy of f against HEAD, staged and unstaged
// changes together, with the whole file as context. An untracked file diffs
// against /dev/null.
func DiffFile(f FileChange) (Diff, error) {
	if f.Status != '?' {
		out, err := run("diff", "HEAD", "--no-color", "--no-ext-diff", wholeFile, "--", f.Path)
		if err != nil {
			return Diff{}, err
		}
		return parseDiff(out), nil
	}
	out, err := run("diff", "--no-index", "--no-color", "--no-ext-diff", wholeFile, "--", "/dev/null", f.Path)
	if err != nil && out == "" { // --no-index exits 1 whenever the files differ
		return Diff{}, err
	}
	return parseDiff(out), nil
}

// parseDiff reads a unified diff. The headers before the first hunk are
// skipped; each hunk header resets the line numbers.
func parseDiff(out string) Diff {
	var d Diff
	old, nu, inHunk := 0, 0, false
	for line := range strings.SplitSeq(strings.TrimSuffix(out, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "@@"):
			old, nu = hunkStarts(line)
			inHunk = true
		case !inHunk:
			d.Binary = d.Binary || strings.HasPrefix(line, "Binary files ")
		case strings.HasPrefix(line, "+"):
			d.Lines = append(d.Lines, DiffLine{Kind: Added, Text: line[1:], Old: old, New: nu})
			d.Added++
			nu++
		case strings.HasPrefix(line, "-"):
			d.Lines = append(d.Lines, DiffLine{Kind: Removed, Text: line[1:], Old: old, New: nu})
			d.Removed++
			old++
		case strings.HasPrefix(line, " "):
			d.Lines = append(d.Lines, DiffLine{Kind: Context, Text: line[1:], Old: old, New: nu})
			old++
			nu++
		}
	}
	return d
}

// hunkStarts reads "@@ -old[,n] +new[,n] @@". An empty side ("-0,0") starts
// at line 1, where its first line would go.
func hunkStarts(header string) (old, nu int) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 1, 1
	}
	return max(rangeStart(fields[1]), 1), max(rangeStart(fields[2]), 1)
}

func rangeStart(r string) int {
	start, _, _ := strings.Cut(r[1:], ",")
	n, _ := strconv.Atoi(start)
	return n
}
