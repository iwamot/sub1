// Package block splits sub1's stdin into the OLD and NEW text blocks.
//
// The input is the body of a heredoc: OLD lines, one line holding only the
// separator, then NEW lines. The heredoc ends every line with '\n', including
// the last one, so the final newline of each block is dropped to make the
// blocks match the way they appear in the file.
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

// Split parses input at the single line equal to sep. It rejects input with
// zero or several separator lines, an empty OLD block, and identical blocks,
// since none of those can describe a replacement.
func Split(input, sep []byte) (Blocks, error) {
	lines := bytes.Split(input, newline)
	at, hits := -1, 0
	for i, line := range lines {
		if bytes.Equal(line, sep) {
			at, hits = i, hits+1
		}
	}
	if hits != 1 {
		return Blocks{}, fmt.Errorf("expected exactly one separator line %q on stdin, found %d", sep, hits)
	}
	oldText := bytes.Join(lines[:at], newline)
	newText := bytes.TrimSuffix(bytes.Join(lines[at+1:], newline), newline)
	if len(oldText) == 0 {
		return Blocks{}, fmt.Errorf("old block is empty")
	}
	if bytes.Equal(oldText, newText) {
		return Blocks{}, fmt.Errorf("old and new blocks are identical")
	}
	return Blocks{Old: oldText, New: newText}, nil
}
