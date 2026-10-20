package history

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func header(oid string) string {
	return "\x00nib-commit\x00" + oid + "\x00\x00Alex\x00alex@example.test\x00123\x00feat: inventory\n\nDetails\n\x00\x00\n"
}
func TestParsePreservesPathsAndRenameStats(t *testing.T) {
	oid := strings.Repeat("a", 64)
	old := "old\tfile\n.txt"
	path := " nuevo\tñ\n.txt "
	stream := header(oid) + ":100644 100644 a b R100\x00" + old + "\x00" + path + "\x00:000000 100644 a b A\x00image.bin\x000\t0\t\x00" + old + "\x00" + path + "\x00-\t-\timage.bin\x00"
	var got []Commit
	if err := Parse(strings.NewReader(stream), func(c Commit) error { got = append(got, c); return nil }); err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: path, PreviousPath: old, Kind: "R"}, {Path: "image.bin", Kind: "A", Binary: true}}
	if len(got) != 1 || got[0].OID != oid || got[0].Author != "Alex" || !reflect.DeepEqual(got[0].Changes, want) {
		t.Fatalf("%+v", got)
	}
}
func TestParseEmptyCommitsAndCallbackFailure(t *testing.T) {
	stream := header(strings.Repeat("a", 40)) + header(strings.Repeat("b", 40))
	count := 0
	if err := Parse(strings.NewReader(stream), func(Commit) error { count++; return nil }); err != nil || count != 2 {
		t.Fatalf("%d %v", count, err)
	}
	want := errors.New("stop")
	if err := Parse(strings.NewReader(stream), func(Commit) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
}
func TestParseRejectsMalformedOrTruncatedHistory(t *testing.T) {
	h := header(strings.Repeat("a", 40))
	for _, stream := range []string{
		"unterminated", "orphan\x00", "\x00nib-commit\x00", header("bad"),
		strings.Replace(h, "123", "invalid", 1),
		h + ":bad\x00", h + ":100644 100644 a b M\x00", h + ":100644 100644 a b M\x00A\x00",
		h + "1\t1\tA\x00", h + ":100644 100644 a b M\x00A\x00bad\x00",
		h + ":100644 100644 a b M\x00A\x00-1\t0\tA\x00",
		h + ":100644 100644 a b M\x00A\x000\tbad\tA\x00",
		h + ":100644 100644 a b R100\x00old\x00",
	} {
		if err := Parse(strings.NewReader(stream), func(Commit) error { return nil }); err == nil {
			t.Errorf("accepted malformed history %q", stream)
		}
	}
}
