package occur

import (
	"bytes"
	"slices"
	"testing"
)

// oracleContents are every string of up to five bytes over "a", "b", CR and
// LF, and the longer contents the examples used to spell out.
func oracleContents() [][]byte {
	out := [][]byte{{}}
	frontier := [][]byte{{}}
	for range 5 {
		var next [][]byte
		for _, s := range frontier {
			for _, c := range []byte("ab\r\n") {
				next = append(next, append(bytes.Clone(s), c))
			}
		}
		out = append(out, next...)
		frontier = next
	}
	for _, s := range []string{
		"foo\nbar\n", "a\nfoo\nb\n", "a\n  x = foo()\n", "x(1)\ny\nx(2)\nx(3)\n",
		"ab ab\n", "a\nb\nc\nb\nc\n", "aaa\n", "abc", "x(1)\ny\nx(2)\n", "a\nb\nc\n",
		"a\r\nb\r\n", "x\ny\nx\n", "foo bar\n",
	} {
		out = append(out, []byte(s))
	}
	return out
}

// oracleOlds are every string of up to three bytes over the same bytes, and
// the old blocks the examples used.
func oracleOlds() [][]byte {
	var out [][]byte
	frontier := [][]byte{{}}
	for range 3 {
		var next [][]byte
		for _, s := range frontier {
			for _, c := range []byte("ab\r\n") {
				next = append(next, append(bytes.Clone(s), c))
			}
		}
		out = append(out, next...)
		frontier = next
	}
	for _, s := range []string{"z", "foo", "x(", "b\nc", "aa", "c", "a\nb", "a\r\nb", "x", "foo "} {
		out = append(out, []byte(s))
	}
	return out
}

// leftmost finds the non-overlapping occurrences of old the way a reader
// would: try each position from the left, take a match, and go on after it.
func leftmost(content, old []byte) []int {
	var found []int
	for i := 0; i+len(old) <= len(content); {
		if bytes.HasPrefix(content[i:], old) {
			found = append(found, i)
			i += len(old)
			continue
		}
		i++
	}
	return found
}

// Lines counts what bytes.Count counts and names the line each match starts
// on.
func TestLinesAgreesWithAScanFromTheLeft(t *testing.T) {
	for _, content := range oracleContents() {
		for _, old := range oracleOlds() {
			var want []int
			for _, pos := range leftmost(content, old) {
				want = append(want, 1+bytes.Count(content[:pos], newline))
			}
			got := Lines(content, old)
			if !slices.Equal(got, want) || len(got) != bytes.Count(content, old) {
				t.Fatalf("Lines(%q, %q) = %v, want %v", content, old, got, want)
			}
		}
	}
}

// With a new block that is not empty, Replace is bytes.ReplaceAll. Deletion
// has rules of its own, pinned by the examples in occur_test.go.
func TestReplaceAgreesWithReplaceAll(t *testing.T) {
	for _, content := range oracleContents() {
		for _, old := range oracleOlds() {
			for _, n := range []string{"y", "\n", "b", "z(", "ab"} {
				saved := bytes.Clone(content)
				got := Replace(content, old, []byte(n))
				if want := bytes.ReplaceAll(content, old, []byte(n)); !bytes.Equal(got, want) {
					t.Fatalf("Replace(%q, %q, %q) = %q, want %q", content, old, n, got, want)
				}
				if !bytes.Equal(content, saved) {
					t.Fatalf("Replace changed %q", saved)
				}
			}
		}
	}
}

// Mask blanks every byte of every match except line breaks, leaves every
// other byte as it was, and so keeps the length and the lines.
func TestMaskBlanksTheMatchesAndKeepsTheLines(t *testing.T) {
	for _, content := range oracleContents() {
		for _, old := range oracleOlds() {
			saved := bytes.Clone(content)
			want := bytes.Clone(content)
			for _, pos := range leftmost(content, old) {
				for i := pos; i < pos+len(old); i++ {
					if want[i] != '\n' && want[i] != '\r' {
						want[i] = 0
					}
				}
			}
			if got := Mask(content, old); !bytes.Equal(got, want) {
				t.Fatalf("Mask(%q, %q) = %q, want %q", content, old, got, want)
			}
			if !bytes.Equal(content, saved) {
				t.Fatalf("Mask changed %q", saved)
			}
		}
	}
}
