package occur

import (
	"bytes"
	"slices"
	"testing"
)

// FindStretch, held against what the stretch is: the start block once, then
// the first end block that begins after it ends, or the lines the end block
// occurs on above the start block when there is none.
func TestFindStretchTakesTheFirstEndAfterTheOneStart(t *testing.T) {
	ends := [][]byte{[]byte("a"), []byte("b"), []byte("\n"), []byte("ab"), []byte("b\n"), []byte("a\na")}
	for _, content := range oracleContents() {
		for _, start := range oracleOlds() {
			for _, end := range ends {
				saved := bytes.Clone(content)
				st := FindStretch(content, start, end)
				if !bytes.Equal(content, saved) {
					t.Fatalf("FindStretch changed %q", saved)
				}
				starts := leftmost(content, start)
				if len(st.Starts) != len(starts) {
					t.Fatalf("FindStretch(%q, %q, %q).Starts = %v, want %d of them", content, start, end, st.Starts, len(starts))
				}
				if len(starts) != 1 {
					if st.End != -1 {
						t.Fatalf("FindStretch(%q, %q, %q) found a stretch with %d starts", content, start, end, len(starts))
					}
					continue
				}
				begin, from := starts[0], starts[0]+len(start)
				var after []int
				for i := from; i+len(end) <= len(content); i++ {
					if bytes.HasPrefix(content[i:], end) {
						after = append(after, i)
					}
				}
				if len(after) == 0 {
					var want []int
					for _, pos := range leftmost(content[:begin], end) {
						want = append(want, 1+bytes.Count(content[:pos], newline))
					}
					if st.End != -1 || !slices.Equal(st.Before, want) {
						t.Fatalf("FindStretch(%q, %q, %q) = %+v, want no stretch and Before %v", content, start, end, st, want)
					}
					continue
				}
				if st.Begin != begin || st.End != after[0]+len(end) {
					t.Fatalf("FindStretch(%q, %q, %q) = %d-%d, want %d-%d", content, start, end, st.Begin, st.End, begin, after[0]+len(end))
				}
				// The stretch occurs once, so Replace rewrites exactly it.
				old := content[st.Begin:st.End]
				if got := bytes.Count(content, old); got != 1 {
					t.Fatalf("stretch %q of %q occurs %d times", old, content, got)
				}
				want := slices.Concat(content[:st.Begin], []byte("N"), content[st.End:])
				if got := Replace(content, old, []byte("N")); !bytes.Equal(got, want) {
					t.Fatalf("Replace(%q, %q, N) = %q, want %q", content, old, got, want)
				}
				first, last := st.Lines(content)
				if first != 1+bytes.Count(content[:st.Begin], newline) || last < first || last-first != bytes.Count(old[:len(old)-1], newline) {
					t.Fatalf("Lines of %q in %q = %d-%d", old, content, first, last)
				}
			}
		}
	}
}

func TestRangeReports(t *testing.T) {
	tests := []struct{ got, want string }{
		{RangeSummary("f", 41, 88, nil), "f: replaced lines 41-88 (48 lines)"},
		{RangeSummary("f", 5, 5, []string{"CRLF"}), "f: replaced line 5 (1 line; CRLF)"},
		{StartMismatch("f", []int{3, 9}, true), "f: start block found 2 times (lines 3, 9), expected 1; widen the start block to match only 1"},
		{StartMismatch("f", nil, false), "f: start block found 0 times, expected 1; no similar text found, read the file again"},
		{StartMismatch("f", nil, true), "f: start block found 0 times, expected 1"},
		{EndMismatch("f", 7, []int{2}, false), "f: end block found 0 times after the start block (line 7); it occurs before the start block, at line 2"},
		{EndMismatch("f", 7, nil, false), "f: end block found 0 times after the start block (line 7); no similar text found, read the file again"},
		{EndMismatch("f", 7, nil, true), "f: end block found 0 times after the start block (line 7)"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}
