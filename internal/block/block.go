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
func Split(input, sep []byte) (Blocks, error) {
	lines := bytes.Split(bytes.TrimSuffix(input, newline), newline)
	last := len(lines) - 1
	if !bytes.Equal(lines[last], sep) {
		return Blocks{}, fmt.Errorf("closing %q line is missing after the new block", sep)
	}
	at, hits := -1, 0
	for i, line := range lines[:last] {
		if bytes.Equal(line, sep) {
			at, hits = i, hits+1
		}
	}
	switch {
	case hits == 0:
		return Blocks{}, fmt.Errorf("found the closing %q line but no %q line between the old and new blocks (an empty new block still takes two %q lines after the old block)", sep, sep, sep)
	case hits > 1:
		return Blocks{}, fmt.Errorf("expected one %q line between the old and new blocks, found %d", sep, hits)
	}
	oldText := bytes.Join(lines[:at], newline)
	newText := bytes.Join(lines[at+1:last], newline)
	if len(oldText) == 0 {
		return Blocks{}, fmt.Errorf("old block is empty")
	}
	if bytes.Equal(oldText, newText) {
		return Blocks{}, fmt.Errorf("old and new blocks are identical")
	}
	return Blocks{Old: oldText, New: newText}, nil
}
