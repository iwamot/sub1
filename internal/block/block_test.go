package block

import (
	"bytes"
	"strings"
	"testing"
)

// The rejections, each with the message it owes the caller. What Split reads
// from well-formed input is a property, below.
func TestSplitRejects(t *testing.T) {
	sep := []byte("====")
	tests := []struct {
		name    string
		input   string
		sep     []byte
		wantErr string
	}{
		{
			name:    "closing separator must fill the whole line",
			input:   "a\n====\nb\n==== \n",
			sep:     sep,
			wantErr: "line 4 looks like \"====\" but has trailing whitespace; remove the spaces or tabs after it",
		},
		{
			name:    "middle separator must fill the whole line",
			input:   "a\n====\t\nb\n====\n",
			sep:     sep,
			wantErr: "line 2 looks like \"====\" but has trailing whitespace; remove the spaces or tabs after it",
		},
		{
			name:    "the first line that looks like a separator is the one named",
			input:   "a\n==== \nb\n==== \n",
			sep:     sep,
			wantErr: "line 2 looks like \"====\" but has trailing whitespace",
		},
		{
			name:    "old form without the closing separator",
			input:   "a\n====\nb\n",
			sep:     sep,
			wantErr: "input ended before the second \"====\" line; if a content line equals the heredoc terminator",
		},
		{
			name:    "old form deletion looks like a closing separator alone",
			input:   "a\n====\n",
			sep:     sep,
			wantErr: "only one \"====\" line; an empty new block still takes two \"====\" lines after the old block",
		},
		{
			name:    "shell cut the heredoc inside the old block",
			input:   "a\nb\n",
			sep:     sep,
			wantErr: "no \"====\" line found; if a content line equals the heredoc terminator",
		},
		{
			name:    "shell cut the heredoc after a quoted separator in the old block",
			input:   "a\n====\nb\nc\n",
			sep:     sep,
			wantErr: "input ended before the second \"====\" line",
		},
		{
			name:    "two separators before the closing one",
			input:   "a\n====\nb\n====\nc\n====\n",
			sep:     sep,
			wantErr: "found 3 \"====\" lines, expected 2; a content line equals \"====\", so pass -d '%%%%' and write %%%% on both separator lines",
		},
		{
			// Two closed old/new pairs in a row, or one pair whose blocks
			// each hold a Markdown setext underline. Either way it is
			// rejected, not read as two pairs.
			name:    "four separators are not read as two pairs",
			input:   "a\n====\nb\n====\nc\n====\nd\n====\n",
			sep:     sep,
			wantErr: "found 4 \"====\" lines, expected 2; a content line equals \"====\", so pass -d '%%%%' and write %%%% on both separator lines",
		},
		{
			name:    "the named separator skips the ones the input already holds",
			input:   "a\n====\n%%%%\n@@@@\n####\nb\n====\nc\n====\n",
			sep:     sep,
			wantErr: "pass -d '=====' and write ===== on both separator lines",
		},
		{
			name:    "the named separator grows until the input does not hold it",
			input:   "a\n====\n%%%%\n@@@@\n####\n=====\nb\n====\nc\n====\n",
			sep:     sep,
			wantErr: "pass -d '======' and write ====== on both separator lines",
		},
		{
			// The separator grows by a whole rune, so a multi-byte one does
			// not come back as a broken encoding of itself.
			name:    "a multi-byte separator grows into valid UTF-8",
			input:   "a\n——\n%%%%\n@@@@\n####\nb\n——\nc\n——\n",
			sep:     []byte("——"),
			wantErr: "pass -d '———' and write ——— on both separator lines",
		},
		{
			name:    "closing separator alone",
			input:   "====\n",
			sep:     sep,
			wantErr: "only one \"====\" line",
		},
		{
			name:    "empty old block",
			input:   "====\nb\n====\n",
			sep:     sep,
			wantErr: "old block is empty; put at least one line before the first \"====\" line",
		},
		{
			name:    "identical blocks",
			input:   "same\n====\nsame\n====\n",
			sep:     sep,
			wantErr: "identical",
		},
		{
			name:    "a line after the closing separator",
			input:   "a\n====\nb\n====\nc\n",
			sep:     sep,
			wantErr: "content after the closing \"====\" line; remove the 1 line after it",
		},
		{
			name:    "a blank line after the closing separator",
			input:   "a\n====\nb\n====\n\n",
			sep:     sep,
			wantErr: "content after the closing \"====\" line; remove the 1 line after it",
		},
		{
			name:    "several lines after the closing separator",
			input:   "a\n====\nb\n====\nc\nd\n",
			sep:     sep,
			wantErr: "content after the closing \"====\" line; remove the 2 lines after it",
		},
		{
			// Past two separators the closing one cannot be picked out, so
			// the count is reported whether or not the input ends with one.
			name:    "three separators and a line after them",
			input:   "a\n====\nb\n====\nc\n====\nd\n",
			sep:     sep,
			wantErr: "found 3 \"====\" lines, expected 2; a content line equals \"====\", so pass -d '%%%%' and write %%%% on both separator lines",
		},
		{
			name:    "empty input",
			input:   "",
			sep:     sep,
			wantErr: "no input on stdin; pass the old and new blocks as a heredoc (see --help)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split([]byte(tt.input), tt.sep)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Split = %q, %v; want an error containing %q", got, err, tt.wantErr)
			}
		})
	}
}

// FuzzSplit checks that any input is either split into a non-empty old
// block or rejected, without panicking. Run it with -fuzz to search beyond
// the seeds.
func FuzzSplit(f *testing.F) {
	f.Add([]byte("a\n====\nb\n====\n"), []byte("===="))
	f.Add([]byte("a\n====\nb\n====\nc\n====\nd\n====\n"), []byte("===="))
	f.Add([]byte("a\n====\n====\n"), []byte("%%%%"))
	f.Fuzz(func(t *testing.T, input, sep []byte) {
		blocks, err := Split(input, sep)
		if err == nil && len(blocks.Old) == 0 {
			t.Fatalf("Split(%q, %q) returned an empty old block", input, sep)
		}
	})
}

// splitTexts are the blocks single pairs are built from: those of
// blockTexts, the blocks the examples above used to spell out (indentation,
// a blank line inside a block, a Markdown underline), and lines equal to
// each separator tried, which only another separator can carry.
var splitTexts = append([]string{
	"foo", "bar", "gone", "  if x:\n    return", "  if x:\n    return None",
	"a\n\nb", "Title\n====", "Title\n----", "====", "%%", "——",
}, blockTexts...)

// A pair written with any separator and any blocks comes back as written,
// with or without the final line break, unless a block holds a line equal to
// the separator, the old block is empty, or the blocks are the same: those
// are refused, for that reason.
func TestSplitReadsBackWhatWasWritten(t *testing.T) {
	read := 0
	for _, sep := range []string{"====", "%%", "——"} {
		for _, o := range splitTexts {
			for _, n := range splitTexts {
				for _, ending := range []string{"\n", ""} {
					input := []byte(o + "\n" + sep + "\n")
					if n != "" {
						input = append(input, n+"\n"...)
					}
					input = append(input, sep+ending...)
					saved := bytes.Clone(input)
					got, err := Split(input, []byte(sep))
					if !bytes.Equal(input, saved) {
						t.Fatalf("%q: input was changed", saved)
					}
					var want string
					switch {
					case holdsLine(o, sep) || holdsLine(n, sep):
						want = "lines, expected 2"
					case o == "":
						want = "old block is empty"
					case o == n:
						want = "identical"
					}
					if want != "" {
						if err == nil || !strings.Contains(err.Error(), want) {
							t.Fatalf("%q: Split = %q, %v; want an error containing %q", input, got, err, want)
						}
						continue
					}
					if err != nil || string(got.Old) != o || string(got.New) != n {
						t.Fatalf("%q: Split = %q, %v; want %q / %q", input, got, err, o, n)
					}
					read++
				}
			}
		}
	}
	if read == 0 {
		t.Fatal("no input was read back")
	}
}

// holdsLine reports whether any line of block is exactly line.
func holdsLine(block, line string) bool {
	for l := range strings.SplitSeq(block, "\n") {
		if l == line {
			return true
		}
	}
	return false
}
