package block

import (
	"bytes"
	"strings"
	"testing"
)

// joinRange writes the three blocks the way an agent writes them for --range.
func joinRange(start, end, new string) string {
	var b strings.Builder
	b.WriteString(start + "\n====\n" + end + "\n====\n")
	if new != "" {
		b.WriteString(new + "\n")
	}
	b.WriteString("====\n")
	return b.String()
}

// Every combination of three blocks, written as --range expects, comes back
// as the blocks it was written from, or is refused because the start or the
// end block is empty.
func TestSplitRangeReadsBackWhatWasWritten(t *testing.T) {
	for _, s := range blockTexts {
		for _, e := range blockTexts {
			for _, n := range blockTexts {
				input := []byte(joinRange(s, e, n))
				saved := bytes.Clone(input)
				got, err := SplitRange(input, []byte("===="))
				if !bytes.Equal(input, saved) {
					t.Fatalf("%q: input was changed", saved)
				}
				want := ""
				switch {
				case s == "":
					want = "start block is empty"
				case e == "":
					want = "end block is empty"
				}
				if want != "" {
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("%q: err = %v, want containing %q", input, err, want)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%q: err = %v", input, err)
				}
				if string(got.Start) != s || string(got.End) != e || string(got.New) != n {
					t.Fatalf("%q: got %q/%q/%q", input, got.Start, got.End, got.New)
				}
			}
		}
	}
}

func TestSplitRangeRefusesMalformedInput(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"empty", "", "no input on stdin"},
		{"no separator", "a\nb\n", `no "====" line found`},
		{"cut short", "a\n====\nb\n====\nc\n", `does not end with a "====" line`},
		{"trailing whitespace", "a\n====\nb\n==== \nc\n====\n", "line 4 looks like"},
		{"two separators, an old/new pair", "a\n====\nb\n====\n", `found 2 "====" lines, expected 3 with --range: one after the start block, one after the end block, and one after the new block, even an empty one`},
		{"four separators", "a\n====\n%%%%\n====\nc\n====\nd\n====\n", `found 4 "====" lines, expected 3 with --range; if a content line equals "====", pass -d '@@@@' and write @@@@ on every separator line`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SplitRange([]byte(tt.input), []byte("===="))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}
