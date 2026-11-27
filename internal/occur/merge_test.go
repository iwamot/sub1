package occur

import (
	"bytes"
	"slices"
	"testing"
)

// contents are every string of up to five characters over "a", "b", and a
// line break, so that occurrences can touch, nest, and sit on line breaks.
func contents() [][]byte {
	out := [][]byte{{}}
	frontier := [][]byte{{}}
	for range 5 {
		var next [][]byte
		for _, s := range frontier {
			for _, c := range []byte("ab\n") {
				next = append(next, append(bytes.Clone(s), c))
			}
		}
		out = append(out, next...)
		frontier = next
	}
	return out
}

var mergeOlds = [][]byte{[]byte("a"), []byte("b"), []byte("ab"), []byte("ba"), []byte("a\n"), []byte("\nb"), []byte("aa")}
var mergeNews = [][]byte{{}, []byte("X"), []byte("a")}

// overlapsByBrute reports whether an occurrence of one old block shares a
// byte with an occurrence of another, found by comparing every pair of
// ranges.
func overlapsByBrute(content []byte, olds [][]byte) bool {
	for i := range olds {
		for j := i + 1; j < len(olds); j++ {
			for _, a := range offsets(content, olds[i]) {
				for _, b := range offsets(content, olds[j]) {
					if a < b+len(olds[j]) && b < a+len(olds[i]) {
						return true
					}
				}
			}
		}
	}
	return false
}

// Merge reports an overlap exactly when two pairs' occurrences share text,
// and otherwise returns every occurrence of every pair once, in file order,
// with no two old blocks sharing a byte. Applying the merge of a single pair
// is Replace.
func TestMergeFindsOverlapsAndOrdersEdits(t *testing.T) {
	checked, overlapped := 0, 0
	for _, content := range contents() {
		for _, o1 := range mergeOlds {
			for _, o2 := range mergeOlds {
				if bytes.Equal(o1, o2) {
					continue
				}
				for _, n := range mergeNews {
					saved := bytes.Clone(content)
					sets := [][]Edit{Edits(content, o1, n), Edits(content, o2, n)}
					merged, overlap := Merge(content, sets)
					if !bytes.Equal(content, saved) {
						t.Fatalf("%q: content was changed", saved)
					}
					if want := overlapsByBrute(content, [][]byte{o1, o2}); (overlap != nil) != want {
						t.Fatalf("%q, %q, %q: overlap = %v, want %v", content, o1, o2, overlap, want)
					}
					if overlap != nil {
						overlapped++
						if overlap.First != 0 || overlap.Second != 1 || overlap.Line < 1 || overlap.Line > 1+bytes.Count(content, newline) {
							t.Fatalf("%q, %q, %q: overlap = %+v", content, o1, o2, *overlap)
						}
						continue
					}
					checked++
					if len(merged) != len(sets[0])+len(sets[1]) {
						t.Fatalf("%q, %q, %q: %d edits, want %d", content, o1, o2, len(merged), len(sets[0])+len(sets[1]))
					}
					for k := 1; k < len(merged); k++ {
						if merged[k].Start < merged[k-1].OldEnd {
							t.Fatalf("%q, %q, %q: edits %d and %d share text", content, o1, o2, k-1, k)
						}
					}
				}
			}
			for _, n := range mergeNews {
				one, overlap := Merge(content, [][]Edit{Edits(content, o1, n)})
				if overlap != nil || !bytes.Equal(Apply(content, one), Replace(content, o1, n)) {
					t.Fatalf("%q, %q, %q: merge of one pair differs from Replace", content, o1, n)
				}
			}
		}
	}
	if checked == 0 || overlapped == 0 {
		t.Fatalf("checked %d without overlap and %d with one; want both", checked, overlapped)
	}
}

// Without overlap, the merged pairs give what replacing one pair after the
// other gives whenever neither new block brings in or breaks up an
// occurrence of the other pair's old block, which is when the order of the
// two calls could not matter. Empty new blocks are left to the example
// below: whether a deletion takes its line break is judged against the file
// as given, and two calls judge the second deletion against what the first
// left.
func TestMergeMatchesTwoCallsWhenTheyCannotInteract(t *testing.T) {
	compared := 0
	for _, content := range contents() {
		for _, o1 := range mergeOlds {
			for _, o2 := range mergeOlds {
				if bytes.Equal(o1, o2) {
					continue
				}
				for _, n1 := range mergeNews[1:] {
					for _, n2 := range mergeNews[1:] {
						merged, overlap := Merge(content, [][]Edit{Edits(content, o1, n1), Edits(content, o2, n2)})
						if overlap != nil {
							continue
						}
						first := Replace(content, o1, n1)
						second := Replace(first, o2, n2)
						other := Replace(Replace(content, o2, n2), o1, n1)
						if !bytes.Equal(second, other) || !slices.Equal(Lines(first, o2), linesAfter(content, first, o2)) {
							continue
						}
						compared++
						if got := Apply(content, merged); !bytes.Equal(got, second) {
							t.Fatalf("%q with %q→%q and %q→%q: merged %q, two calls %q", content, o1, n1, o2, n2, got, second)
						}
					}
				}
			}
		}
	}
	if compared == 0 {
		t.Fatal("no combination was compared")
	}
}

// linesAfter is the number of occurrences of old that the first call left as
// they were: the count in content when the first call did not change how
// many there are, and a value that never matches otherwise.
func linesAfter(content, first, old []byte) []int {
	if len(offsets(content, old)) != len(offsets(first, old)) {
		return []int{-1}
	}
	return Lines(first, old)
}

// Deleting the two halves of a line deletes them and keeps the line break:
// neither half is a whole line in the file as given. Two calls would delete
// the line break as well, the second half having become a whole line after
// the first call.
func TestMergeJudgesDeletionsAgainstTheFileAsGiven(t *testing.T) {
	content := []byte("ab\nc\n")
	merged, overlap := Merge(content, [][]Edit{Edits(content, []byte("a"), nil), Edits(content, []byte("b"), nil)})
	if overlap != nil {
		t.Fatalf("overlap = %+v", *overlap)
	}
	if got, want := string(Apply(content, merged)), "\nc\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMergeNamesTheLaterOccurrence(t *testing.T) {
	content := []byte("one\ntwo three\nfour\n")
	sets := [][]Edit{
		Edits(content, []byte("three\nfour"), []byte("x")),
		Edits(content, []byte("two three"), []byte("y")),
	}
	_, overlap := Merge(content, sets)
	if overlap == nil || *overlap != (Overlap{First: 0, Second: 1, Line: 2}) {
		t.Fatalf("overlap = %+v, want pairs 0 and 1 at line 2", overlap)
	}
}
