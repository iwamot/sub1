// Package occur locates a block inside file content and describes where a
// replacement happened.
package occur

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Lines returns the 1-based line number at which each non-overlapping
// occurrence of old starts, in order. The occurrences are the same ones
// bytes.Count and bytes.ReplaceAll see, so len(Lines(...)) is the count that
// decides whether the file is rewritten.
func Lines(content, old []byte) []int {
	var lines []int
	for pos := 0; ; {
		i := bytes.Index(content[pos:], old)
		if i < 0 {
			return lines
		}
		pos += i
		lines = append(lines, bytes.Count(content[:pos], []byte("\n"))+1)
		pos += len(old)
	}
}

// Summary is the one-line report printed after a successful replacement.
func Summary(path string, lines []int) string {
	nums := make([]string, len(lines))
	for i, n := range lines {
		nums[i] = strconv.Itoa(n)
	}
	noun := "line"
	if len(lines) != 1 {
		noun = "lines"
	}
	return fmt.Sprintf("%s: replaced at %s %s", path, noun, strings.Join(nums, ", "))
}
