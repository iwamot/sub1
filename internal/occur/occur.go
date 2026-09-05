// Package occur locates a block inside file content and describes where a
// replacement happened, or why it did not.
package occur

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

var newline = []byte("\n")

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
		lines = append(lines, bytes.Count(content[:pos], newline)+1)
		pos += len(old)
	}
}

// Summary is the one-line report printed after a successful replacement.
func Summary(path string, lines []int) string {
	return fmt.Sprintf("%s: replaced at %s", path, lineList(lines))
}

// Mismatch is the one-line report printed when old was found a different
// number of times than expected. The lines are included so that the caller
// can decide which occurrences to widen the block around, or how many to
// expect, without reading the file again.
func Mismatch(path string, lines []int, expected int) string {
	where := ""
	if len(lines) > 0 {
		where = " (" + lineList(lines) + ")"
	}
	return fmt.Sprintf("%s: old block found %s%s, expected %d", path, times(len(lines)), where, expected)
}

// Hint guesses why old, which does not occur in content, was expected to.
// It reports the closest thing it can find, or "" when nothing comes close.
//
// The guess is made in two steps. First, content and old are compared with
// one kind of whitespace difference ignored at a time, and the first kind
// that makes them match is reported along with what the file actually has.
// Second, for a multi-line block, the place where the most leading lines of
// old match in a row is reported. The hint is only a lead: the count in the
// Mismatch line is what decided that nothing was replaced.
func Hint(content, old []byte) string {
	for _, n := range normalizations {
		lines := Lines(n.apply(content), n.apply(old))
		if len(lines) == 0 {
			continue
		}
		return fmt.Sprintf("near %s: %s", lineList(lines), n.describe(content, old, lines[0]))
	}
	return prefixHint(content, old)
}

// A normalization erases one kind of whitespace difference. It must keep the
// number of lines unchanged, so that a line number found in the normalized
// content is valid in the original. describe explains, for the match that
// starts at the given 1-based line of content, what the file has that old
// does not, or the other way round.
type normalization struct {
	apply    func([]byte) []byte
	describe func(content, old []byte, line int) string
}

var normalizations = []normalization{
	{lineEndings, describeLineEndings},
	{trailingWhitespace, describeTrailingWhitespace},
	{leadingWhitespace, describeLeadingWhitespace},
}

func lineEndings(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), newline)
}

func describeLineEndings(content, _ []byte, _ int) string {
	side := "file"
	if !bytes.Contains(content, []byte("\r\n")) {
		side = "old block"
	}
	return "the " + side + " uses CRLF line endings"
}

func trailingWhitespace(b []byte) []byte {
	return mapLines(b, func(line []byte) []byte { return bytes.TrimRight(line, " \t") })
}

// describeTrailingWhitespace names the first line, in old and then in the
// matched region of content, that carries trailing whitespace. One of them
// must, or the blocks would have matched as they are.
func describeTrailingWhitespace(content, old []byte, line int) string {
	oldLines, fileLines := region(content, old, line)
	i := 0
	for i+1 < len(oldLines) && !hasTrailing(oldLines[i]) && !hasTrailing(fileLines[i]) {
		i++
	}
	if hasTrailing(oldLines[i]) {
		return fmt.Sprintf("old block line %d has trailing whitespace", i+1)
	}
	return fmt.Sprintf("file line %d has trailing whitespace", line+i)
}

func hasTrailing(line []byte) bool {
	return len(bytes.TrimRight(line, " \t")) != len(line)
}

func leadingWhitespace(b []byte) []byte {
	return mapLines(b, func(line []byte) []byte { return bytes.TrimLeft(line, " \t") })
}

// describeLeadingWhitespace compares the indentation of old and of the
// matched region of content line by line and reports the first pair that
// differs. One must, or the blocks would have matched as they are.
func describeLeadingWhitespace(content, old []byte, line int) string {
	oldLines, fileLines := region(content, old, line)
	i := 0
	for i+1 < len(oldLines) && bytes.Equal(indent(fileLines[i]), indent(oldLines[i])) {
		i++
	}
	return fmt.Sprintf("file line %d starts with %s, old block line %d with %s",
		line+i, describeIndent(indent(fileLines[i])), i+1, describeIndent(indent(oldLines[i])))
}

func indent(line []byte) []byte {
	return line[:len(line)-len(bytes.TrimLeft(line, " \t"))]
}

func describeIndent(ws []byte) string {
	tabs := bytes.Count(ws, []byte("\t"))
	spaces := len(ws) - tabs
	switch {
	case tabs == 0 && spaces == 0:
		return "no indentation"
	case tabs > 0 && spaces > 0:
		return "mixed tabs and spaces"
	case tabs > 0:
		return plural(tabs, "tab")
	default:
		return plural(spaces, "space")
	}
}

// region returns the lines of old and the same number of lines of content
// starting at the given 1-based line, which is where a normalized match
// begins. Both blocks are split the same way, and since normalizations keep
// line counts, the file lines are the ones the match covers.
func region(content, old []byte, line int) (oldLines, fileLines [][]byte) {
	oldLines = bytes.Split(old, newline)
	fileLines = bytes.Split(content, newline)[line-1:]
	return oldLines, fileLines[:len(oldLines)]
}

// mapLines applies f to each line of b, leaving the line breaks as they are.
func mapLines(b []byte, f func([]byte) []byte) []byte {
	lines := bytes.Split(b, newline)
	for i, line := range lines {
		lines[i] = f(line)
	}
	return bytes.Join(lines, newline)
}

// prefixHint finds the place in content where the most leading lines of old
// match in a row. A single matching line says nothing, since a brace or a
// blank line matches anywhere, so at least two lines must match to report.
// The run never reaches the last line of old: if it did, old as a whole
// would occur in content.
func prefixHint(content, old []byte) string {
	oldLines := bytes.Split(old, newline)
	if len(oldLines) < 2 {
		return ""
	}
	fileLines := bytes.Split(content, newline)
	first := append(append([]byte{}, oldLines[0]...), '\n')
	best, bestLine := 0, 0
	for pos := 0; ; {
		i := bytes.Index(content[pos:], first)
		if i < 0 {
			break
		}
		pos += i
		line := bytes.Count(content[:pos], newline) + 1
		m := 1
		for m < len(oldLines) && line-1+m < len(fileLines) && bytes.Equal(fileLines[line-1+m], oldLines[m]) {
			m++
		}
		if m > best {
			best, bestLine = m, line
		}
		pos += len(first)
	}
	if best < 2 {
		return ""
	}
	return fmt.Sprintf("near line %d: first %d of %d lines match; line %d differs", bestLine, best, len(oldLines), best+1)
}

func lineList(lines []int) string {
	nums := make([]string, len(lines))
	for i, n := range lines {
		nums[i] = strconv.Itoa(n)
	}
	noun := "line"
	if len(lines) != 1 {
		noun = "lines"
	}
	return noun + " " + strings.Join(nums, ", ")
}

func times(n int) string {
	if n == 1 {
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
