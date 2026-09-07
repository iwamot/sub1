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
		// The shell cut the heredoc short. Which separators did arrive says
		// where it was cut, and the terminator is the thing to change.
		switch hits {
		case 0:
			return Blocks{}, fmt.Errorf("no %q line found; if a content line equals the heredoc terminator, use another terminator", sep)
		case 1:
			return Blocks{}, fmt.Errorf("input ended before the second %q line; if a content line equals the heredoc terminator, use another terminator", sep)
		}
		return Blocks{}, fmt.Errorf("input ended before the closing %q line; if a content line equals the heredoc terminator, use another terminator", sep)
	}
	switch {
	case hits == 0:
		if n := nearlySep(lines[:last], sep); n > 0 {
			return Blocks{}, trailingWhitespace(n, sep)
		}
		return Blocks{}, fmt.Errorf("only one %q line; an empty new block still takes two %q lines after the old block", sep, sep)
	case hits > 1:
		return Blocks{}, fmt.Errorf("found %d %q lines, expected 2; if a content line equals %q, pass -d SEP and use SEP as the separator", hits+1, sep, sep)
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
