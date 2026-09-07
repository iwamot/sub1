// Package occur locates a block inside file content and describes where a
// replacement happened, or why it did not.
package occur

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/iwamot/sub1/internal/crlf"
)

var newline = []byte("\n")

// Lines returns the 1-based line number at which each non-overlapping
// occurrence of old starts, in order. The occurrences are the same ones
// Replace rewrites, so len(Lines(...)) is the count that decides whether the
// file is rewritten.
func Lines(content, old []byte) []int {
	var lines []int
	for _, pos := range offsets(content, old) {
		lines = append(lines, bytes.Count(content[:pos], newline)+1)
	}
	return lines
}

// Replace returns content with every non-overlapping occurrence of old
// replaced by new.
//
// An empty new block deletes the old block, and it deletes the line break
// after it as well when the old block is a whole line or a run of whole
// lines: the occurrence starts a line (it is at the start of the file or
// follows a "\n"), old does not itself end with "\n", and a line break comes
// right after it. That line break is the one the heredoc dropped from the
// last line of the old block, so the rule is what "delete the old block"
// means when the block was written as lines. An old block that ends with a
// blank line already carries its own line break and takes nothing more. The
// line break is deleted as it is in the file, CRLF or LF, so the rule needs
// no knowledge of the file's line endings. Each occurrence is judged on its
// own, against the original content.
func Replace(content, old, new []byte) []byte {
	var out []byte
	pos := 0
	for _, i := range offsets(content, old) {
		out = append(out, content[pos:i]...)
		out = append(out, new...)
		pos = i + len(old)
		if len(new) == 0 && (i == 0 || content[i-1] == '\n') && !bytes.HasSuffix(old, newline) {
			pos += lineBreakLen(content[pos:])
		}
	}
	return append(out, content[pos:]...)
}

// lineBreakLen returns the length of the line break b starts with, or 0.
func lineBreakLen(b []byte) int {
	switch {
	case bytes.HasPrefix(b, []byte("\r\n")):
		return 2
	case bytes.HasPrefix(b, newline):
		return 1
	}
	return 0
}

// offsets returns the byte offset of each non-overlapping occurrence of old
// in content, in order. It is the one scan behind Lines and Replace, so the
// occurrences that are counted are the ones that are rewritten.
func offsets(content, old []byte) []int {
	var found []int
	for pos := 0; ; {
		i := bytes.Index(content[pos:], old)
		if i < 0 {
			return found
		}
		pos += i
		found = append(found, pos)
		pos += len(old)
	}
}

// Summary is the one-line report printed after a successful replacement.
// The notes, if any, follow in parentheses.
func Summary(path string, lines []int, notes []string) string {
	s := fmt.Sprintf("%s: replaced at %s", path, lineList(lines))
	if len(notes) > 0 {
		s += " (" + strings.Join(notes, "; ") + ")"
	}
	return s
}

// Notes states what about a replacement that is about to be made could be
// other than what a block written as lines was meant to do. Matching is
// byte-exact, so an occurrence can start or end inside a line, and each
// note is a fact about where the occurrences sit, not a verdict: renaming
// an identifier at its call sites matches inside lines on purpose. The
// notes are printed so that a caller can check the edit, and so that how
// often each kind is meant can be counted from the output. Every
// occurrence is judged against the original content, as Replace does.
//
// The notes, in the order they are given:
//
//   - An occurrence starts inside an identifier: the byte before it and the
//     first byte of old are both identifier characters (ASCII letters,
//     digits, and underscore), as when old "x = 1" is found in "max = 1".
//   - An occurrence ends inside an identifier: the byte after it and the
//     last byte of old are both identifier characters, as when old
//     "return" is found in "returned".
//   - An occurrence starts in the middle of a line and new has several
//     lines: the lines of new after the first are inserted with the
//     indentation they were written with, not the one the file has at the
//     occurrence, as when old "return 1" without indentation is found in
//     an indented line.
//   - The file already contained new: new contains old and content contains
//     new, so the occurrence sits inside a copy of new that is already
//     there, and replacing it repeats the rest of new. This is what a second
//     run of the same edit looks like.
func Notes(content, old, new []byte) []string {
	startsInside, endsInside, midLine := 0, 0, 0
	multiLine := bytes.Contains(new, newline)
	for _, i := range offsets(content, old) {
		end := i + len(old)
		if i > 0 && isIdent(content[i-1]) && isIdent(old[0]) {
			startsInside++
		}
		if end < len(content) && isIdent(content[end]) && isIdent(old[len(old)-1]) {
			endsInside++
		}
		if i > 0 && content[i-1] != '\n' && multiLine {
			midLine++
		}
	}
	var notes []string
	if startsInside > 0 {
		notes = append(notes, matches(startsInside, "starts", "start")+" inside an identifier")
	}
	if endsInside > 0 {
		notes = append(notes, matches(endsInside, "ends", "end")+" inside an identifier")
	}
	if midLine > 0 {
		notes = append(notes, matches(midLine, "starts", "start")+" mid-line with a multi-line new block")
	}
	if bytes.Contains(new, old) && bytes.Contains(content, new) {
		notes = append(notes, "the file already contained the new block")
	}
	return notes
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// matches counts occurrences with a verb: "1 match starts", "2 matches start".
func matches(n int, singular, plural string) string {
	if n == 1 {
		return "1 match " + singular
	}
	return fmt.Sprintf("%d matches %s", n, plural)
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
// Second, for a multi-line block, the longest run of its lines that appears
// in a row in the file is reported, along with the line next to that run
// that must differ. The hint is only a lead: the count in the Mismatch line
// is what decided that nothing was replaced.
func Hint(content, old []byte) string {
	// CRLF line endings are folded away on the file side only, and stated
	// first when present. A file with CRLF usually differs in something else
	// as well, and the "\r" would otherwise hide a trailing-whitespace
	// difference. The old block comes from a heredoc and has no "\r" to fold.
	// The caller folds a file that is CRLF throughout before asking for a
	// hint, so CRLF seen here normally means the file mixes line endings.
	var notes []string
	if bytes.Contains(content, []byte("\r\n")) {
		note := "the file has mixed line endings"
		if crlf.Uniform(content) {
			note = "the file uses CRLF line endings"
		}
		content = crlf.ToLF(content)
		notes = append(notes, note)
	}
	for _, n := range normalizations {
		lines := Lines(n.apply(content), n.apply(old))
		if len(lines) == 0 {
			continue
		}
		if n.describe != nil {
			notes = append(notes, n.describe(content, old, lines[0]))
		}
		return fmt.Sprintf("near %s: %s", lineList(lines), strings.Join(notes, "; "))
	}
	line, note := runHint(content, old)
	if line == 0 {
		return ""
	}
	return fmt.Sprintf("near line %d: %s", line, strings.Join(append(notes, note), "; "))
}

// A normalization erases one kind of whitespace difference. It must keep the
// number of lines unchanged, so that a line number found in the normalized
// content is valid in the original. describe explains, for the match that
// starts at the given 1-based line of content, what the file has that old
// does not, or the other way round. The first normalization is the identity,
// which catches a file that differs in line endings alone.
type normalization struct {
	apply    func([]byte) []byte
	describe func(content, old []byte, line int) string
}

var normalizations = []normalization{
	{func(b []byte) []byte { return b }, nil},
	{trailingWhitespace, describeTrailingWhitespace},
	{leadingWhitespace, describeLeadingWhitespace},
	{innerWhitespace, describeInnerWhitespace},
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
	if len(ws) == 0 {
		return "no indentation"
	}
	return describeRun(ws)
}

// describeRun names a non-empty run of spaces and tabs.
func describeRun(ws []byte) string {
	tabs := bytes.Count(ws, []byte("\t"))
	spaces := len(ws) - tabs
	switch {
	case tabs > 0 && spaces > 0:
		return "mixed tabs and spaces"
	case tabs > 0:
		return plural(tabs, "tab")
	default:
		return plural(spaces, "space")
	}
}

// innerWhitespace collapses every run of spaces and tabs in a line into a
// single space, wherever it is. It comes after the trailing and leading
// normalizations, so a difference it alone erases lies inside a line, or is
// spread over more than one of the three places.
func innerWhitespace(b []byte) []byte {
	return mapLines(b, func(line []byte) []byte {
		out := make([]byte, 0, len(line))
		for i := 0; i < len(line); {
			if !isBlank(line[i]) {
				out = append(out, line[i])
				i++
				continue
			}
			out = append(out, ' ')
			for i < len(line) && isBlank(line[i]) {
				i++
			}
		}
		return out
	})
}

// describeInnerWhitespace walks the first line of the matched region that
// differs from old as written and reports the first run of whitespace whose
// two versions disagree. The lines are equal once every run is collapsed to
// one space, so the runs pair up one to one.
func describeInnerWhitespace(content, old []byte, line int) string {
	oldLines, fileLines := region(content, old, line)
	i := 0
	for i+1 < len(oldLines) && bytes.Equal(oldLines[i], fileLines[i]) {
		i++
	}
	fileRun, oldRun := differingRuns(fileLines[i], oldLines[i])
	return fmt.Sprintf("file line %d has %s where the old block has %s", line+i, describeRun(fileRun), describeRun(oldRun))
}

// differingRuns returns the first pair of whitespace runs, at the same place
// in a and b, that are not the same bytes. a and b must be equal once their
// runs are collapsed, and must not be identical.
func differingRuns(a, b []byte) (runA, runB []byte) {
	i, j := 0, 0
	for {
		if isBlank(a[i]) {
			ra, rb := blankRun(a[i:]), blankRun(b[j:])
			if !bytes.Equal(ra, rb) {
				return ra, rb
			}
			i, j = i+len(ra), j+len(rb)
			continue
		}
		i++
		j++
	}
}

// blankRun returns the run of spaces and tabs that b starts with.
func blankRun(b []byte) []byte {
	n := 0
	for n < len(b) && isBlank(b[n]) {
		n++
	}
	return b[:n]
}

func isBlank(c byte) bool {
	return c == ' ' || c == '\t'
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

// runHint finds the longest run of consecutive lines of old that appears,
// whole and in order, in content, at any offset into old. It returns the
// 1-based file line where the run starts and a note naming the lines of old
// it covers and the line next to it that must differ, or 0 when there is no
// run to report. Lines are compared whole: a line of old ending a longer
// file line is not a match. A single matching line says nothing, since a
// brace or a blank line matches anywhere, so at least two lines must match.
// The run never covers all of old: if it did, old as a whole would occur in
// content. On a tie the earliest place in the file wins, and within it the
// earliest place in old.
func runHint(content, old []byte) (int, string) {
	oldLines := bytes.Split(old, newline)
	if len(oldLines) < 2 {
		return 0, ""
	}
	fileLines := bytes.Split(content, newline)
	best, bestFile, bestOld := 0, 0, 0
	for j := range fileLines {
		for i := range oldLines {
			if j > 0 && i > 0 && bytes.Equal(fileLines[j-1], oldLines[i-1]) {
				continue // inside a run already counted from where it starts
			}
			m := 0
			for i+m < len(oldLines) && j+m < len(fileLines) && bytes.Equal(fileLines[j+m], oldLines[i+m]) {
				m++
			}
			if m > best {
				best, bestFile, bestOld = m, j, i
			}
		}
	}
	if best < 2 {
		return 0, ""
	}
	var differs []int
	if bestOld > 0 {
		differs = append(differs, bestOld)
	}
	if bestOld+best < len(oldLines) {
		differs = append(differs, bestOld+best+1)
	}
	return bestFile + 1, fmt.Sprintf("lines %d-%d of the old block match file lines %d-%d; %s",
		bestOld+1, bestOld+best, bestFile+1, bestFile+best, differLines(differs))
}

// differLines words the lines of old, next to the matched run, that are
// known not to match: one before it, one after it, or both.
func differLines(lines []int) string {
	if len(lines) == 1 {
		return fmt.Sprintf("line %d differs", lines[0])
	}
	return fmt.Sprintf("lines %d and %d differ", lines[0], lines[1])
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
