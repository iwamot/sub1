package block

import (
	"bytes"
	"strings"
	"testing"
)

// blockTexts are the blocks pairs are built from: empty, one line, several
// lines, a blank line, and lines that look like a separator without being
// one.
var blockTexts = []string{"", "a", "a\nb", "\n", "b\n", "==== ", " ====", "==", "日本語"}

// joinPairs writes pairs the way an agent writes them for --pairs.
func joinPairs(pairs [][2]string) string {
	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(p[0] + "\n====\n")
		if p[1] != "" {
			b.WriteString(p[1] + "\n")
		}
		b.WriteString("====\n")
	}
	return b.String()
}

// wantError is what SplitPairs refuses in pairs that are written correctly:
// an empty old block, identical blocks, and two pairs with one old block.
func wantError(pairs [][2]string) string {
	seen := map[string]bool{}
	for _, p := range pairs {
		switch {
		case p[0] == "":
			return "old block is empty"
		case p[0] == p[1]:
			return "identical"
		case seen[p[0]]:
			return "the same old block"
		}
		seen[p[0]] = true
	}
	return ""
}

// Every combination of up to three pairs, written as --pairs expects, comes
// back as the pairs it was written from, or is refused for the reason that
// applies to it.
func TestSplitPairsReadsBackWhatWasWritten(t *testing.T) {
	var combos [][][2]string
	for _, o1 := range blockTexts {
		for _, n1 := range blockTexts {
			combos = append(combos, [][2]string{{o1, n1}})
			for _, o2 := range blockTexts {
				for _, n2 := range []string{"", "x", o1} {
					combos = append(combos, [][2]string{{o1, n1}, {o2, n2}})
					combos = append(combos, [][2]string{{o1, n1}, {o2, n2}, {"z", o2}})
				}
			}
		}
	}
	for _, pairs := range combos {
		input := []byte(joinPairs(pairs))
		saved := bytes.Clone(input)
		got, err := SplitPairs(input, []byte("===="))
		if !bytes.Equal(input, saved) {
			t.Fatalf("%q: input was changed", saved)
		}
		if want := wantError(pairs); want != "" {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%q: err = %v, want containing %q", input, err, want)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: err = %v", input, err)
		}
		if len(got) != len(pairs) {
			t.Fatalf("%q: %d pairs, want %d", input, len(got), len(pairs))
		}
		for i, p := range pairs {
			if string(got[i].Old) != p[0] || string(got[i].New) != p[1] {
				t.Fatalf("%q: pair %d = %q/%q, want %q/%q", input, i+1, got[i].Old, got[i].New, p[0], p[1])
			}
		}
	}
}

// One pair reads the same as Split reads it.
func TestSplitPairsReadsOnePairAsSplitDoes(t *testing.T) {
	for _, o := range blockTexts {
		for _, n := range blockTexts {
			input := []byte(joinPairs([][2]string{{o, n}}))
			single, errSingle := Split(input, []byte("===="))
			pairs, errPairs := SplitPairs(input, []byte("===="))
			if (errSingle == nil) != (errPairs == nil) {
				t.Fatalf("%q: Split err = %v, SplitPairs err = %v", input, errSingle, errPairs)
			}
			if errSingle == nil && (len(pairs) != 1 || !bytes.Equal(pairs[0].Old, single.Old) || !bytes.Equal(pairs[0].New, single.New)) {
				t.Fatalf("%q: SplitPairs = %q, Split = %q", input, pairs, single)
			}
		}
	}
}

func TestSplitPairsRefusesMalformedInput(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"empty", "", "no input on stdin"},
		{"no separator", "a\nb\n", `no "====" line found`},
		{"cut short", "a\n====\nb\n====\nc\n====\nd\n", `does not end with a "====" line`},
		{"trailing whitespace", "a\n====\nb\n==== \n", "line 4 looks like"},
		{"odd count", "a\n====\nb\n====\nc\n====\n", `found 3 "====" lines, expected an even number with --pairs`},
		{"odd count names a free separator", "a\n====\n%%%%\n====\nc\n====\n", "pass -d '@@@@' and write @@@@ on every separator line"},
		{"empty second old", "a\n====\nb\n====\n====\nc\n====\n", "pair 2: old block is empty"},
		{"identical second pair", "a\n====\nb\n====\nc\n====\nc\n====\n", "pair 2: old and new blocks are identical"},
		{"same old in pairs 1 and 3", "a\n====\nb\n====\nc\n====\nd\n====\na\n====\ne\n====\n", "pair 1 and pair 3 have the same old block; remove one of them"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SplitPairs([]byte(tt.input), []byte("===="))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}
