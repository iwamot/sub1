// Package block splits sub1's stdin into the OLD and NEW text blocks.
//
// The input is the body of a heredoc: OLD lines, one line holding only the
// separator, NEW lines, and a closing line holding only the separator. The
// heredoc ends every line with '\n', including the last one, so the final
// newline of each block is dropped to make the blocks match the way they
// appear in the file.
//
// The closing separator tells a complete input from one the shell cut short.
// A heredoc ends at the first line equal to its terminator, so a terminator
// written on the separator line by mistake, or one that also occurs inside a
// block, ends the heredoc early, and what reaches sub1 stops before the
// closing separator.
package block

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// Blocks holds the text to find and the text to put in its place.
type Blocks struct {
	Old []byte
	New []byte
}

var newline = []byte("\n")

// Split parses input as OLD, sep, NEW, sep. It rejects input whose last line
// is not sep, input with zero or several sep lines before that, an empty OLD
// block, and identical blocks, since none of those can describe a
// replacement.
//
// A rejection names what to do next whenever there is something to name. The
// caller is usually an agent that wrote the heredoc it just sent, so the
// message says which of its lines to change rather than only which rule was
// broken. The separators are counted in full, closing line included, because
// that is the number the caller can count in what it wrote; a count that
// does not say where the input was cut is left out.
//
// Several separators are refused rather than divided up. Which of them the
// caller meant as text cannot be told, and a count that happens to divide
// evenly would still be a guess at where the blocks end. Naming a separator
// the input does not hold is a different matter: that needs the lines, not
// a split.
func Split(input, sep []byte) (Blocks, error) {
	if len(input) == 0 {
		return Blocks{}, fmt.Errorf("no input on stdin; pass the old and new blocks as a heredoc (see --help)")
	}
	lines := bytes.Split(bytes.TrimSuffix(input, newline), newline)
	last := len(lines) - 1
	at, hits := -1, 0
	for i, line := range lines[:last] {
		if bytes.Equal(line, sep) {
			at, hits = i, hits+1
		}
	}
	if !bytes.Equal(lines[last], sep) {
		if n := nearlySep(lines, sep); n > 0 {
			return Blocks{}, trailingWhitespace(n, sep)
		}
		// Fewer than two separators means the shell cut the heredoc short,
		// and which ones did arrive says where. Exactly two is the other
		// story: the closing separator is there and text follows it, which
		// a cut input cannot look like, since a cut takes lines off the end.
		switch hits {
		case 0:
			return Blocks{}, fmt.Errorf("no %q line found; if a content line equals the heredoc terminator, use another terminator", sep)
		case 1:
			return Blocks{}, fmt.Errorf("input ended before the second %q line; if a content line equals the heredoc terminator, use another terminator", sep)
		case 2:
			return Blocks{}, fmt.Errorf("content after the closing %q line; remove the %s after it", sep, plural(last-at, "line"))
		}
		// Past two, which separator was the closing one cannot be told, so
		// the count is what to report, the same as for an input that does
		// end with one.
		return Blocks{}, tooManySeparators(hits, lines, sep)
	}
	switch {
	case hits == 0:
		if n := nearlySep(lines[:last], sep); n > 0 {
			return Blocks{}, trailingWhitespace(n, sep)
		}
		return Blocks{}, fmt.Errorf("only one %q line; an empty new block still takes two %q lines after the old block", sep, sep)
	case hits > 1:
		return Blocks{}, tooManySeparators(hits+1, lines, sep)
	}
	oldText := bytes.Join(lines[:at], newline)
	newText := bytes.Join(lines[at+1:last], newline)
	if len(oldText) == 0 {
		return Blocks{}, fmt.Errorf("old block is empty; put at least one line before the first %q line", sep)
	}
	if bytes.Equal(oldText, newText) {
		return Blocks{}, fmt.Errorf("old and new blocks are identical")
	}
	return Blocks{Old: oldText, New: newText}, nil
}

// trailingWhitespace names a line that was meant to be a separator. The line
// number is what the caller needs; the rest of the input is described by
// whichever check called this.
func trailingWhitespace(n int, sep []byte) error {
	return fmt.Errorf("line %d looks like %q but has trailing whitespace; remove the spaces or tabs after it", n, sep)
}

// tooManySeparators reports an input holding more separator lines than the
// grammar has places for. The count is n, the closing line included when the
// input has one. The separator to move to is named outright, since the
// caller would otherwise have to pick one and check it against its own text.
func tooManySeparators(n int, lines [][]byte, sep []byte) error {
	free := freeSeparator(lines, sep)
	return fmt.Errorf("found %d %q lines, expected 2; a content line equals %q, so pass -d '%s' and write %s on both separator lines", n, sep, sep, free, free)
}

// freeSeparator returns a separator that no line of input equals, so the
// caller can paste it into -d without checking anything first. Every line is
// a candidate to collide with, the current separator included: the blocks
// are not split at this point, and cannot be, which is why the input was
// rejected in the first place.
//
// The three fixed candidates come first because they read as separators. If
// the input holds all of them, the current separator grows by its own last
// character, which ends because no line can equal one longer than itself.
// The character is taken as a rune, so that a separator such as "——" grows
// into valid UTF-8 rather than into a broken encoding of it.
func freeSeparator(lines [][]byte, sep []byte) []byte {
	for _, c := range [][]byte{[]byte("%%%%"), []byte("@@@@"), []byte("####")} {
		if !holds(lines, c) {
			return c
		}
	}
	_, size := utf8.DecodeLastRune(sep)
	tail := sep[len(sep)-size:]
	for n := 1; ; n++ {
		grown := append(append([]byte{}, sep...), bytes.Repeat(tail, n)...)
		if !holds(lines, grown) {
			return grown
		}
	}
}

// holds reports whether any line is exactly s.
func holds(lines [][]byte, s []byte) bool {
	for _, line := range lines {
		if bytes.Equal(line, s) {
			return true
		}
	}
	return false
}

// plural writes a count with its noun, as "1 line" or "3 lines".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// nearlySep returns the 1-based number of the first line that is sep with
// trailing spaces or tabs, or 0. Such a line is a separator that was meant
// to be one, and the caller names it rather than reporting the separator as
// missing.
func nearlySep(lines [][]byte, sep []byte) int {
	for i, line := range lines {
		if !bytes.Equal(line, sep) && bytes.Equal(bytes.TrimRight(line, " \t"), sep) {
			return i + 1
		}
	}
	return 0
}
