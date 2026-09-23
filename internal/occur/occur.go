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

const (
	// quoteMax is where a quoted file line is cut. It is wide enough for an
	// ordinary line of code and narrow enough that the hint stays one line
	// on a terminal.
	quoteMax = 120

	// minOverlap is how much of a one-line old block a file line has to
	// carry before it can be called the closest one. Half of the block is
	// asked for as well, so this is the floor under that half: without it a
	// short block — "}" or "return" — would name whichever line happens to
	// share a couple of bytes with it.
	minOverlap = 8

	// gapSlack is how much longer what lies between the head and the tail
	// of the old block may be in the file than in the block itself. A typo
	// barely moves that distance, so allowing a little keeps the lines a
	// typo produces while it rules out a long line that carries the head in
	// one place and the tail somewhere else entirely.
	gapSlack = 16
)

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
// own, against the original content. A line break that the next occurrence
// starts with belongs to that occurrence and is deleted with it, so every
// occurrence that was counted is deleted whole.
func Replace(content, old, new []byte) []byte {
	var out []byte
	pos := 0
	found := offsets(content, old)
	for k, i := range found {
		out = append(out, content[pos:i]...)
		out = append(out, new...)
		pos = i + len(old)
		if len(new) == 0 && (i == 0 || content[i-1] == '\n') && !bytes.HasSuffix(old, newline) {
			brk := lineBreakLen(content[pos:])
			if k+1 == len(found) || found[k+1] >= pos+brk {
				pos += brk
			}
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

// Mask returns a copy of content with every occurrence of old blanked out,
// so that a hint asked for afterwards looks for what is missing instead of
// settling for an occurrence that is already there.
//
// The byte blanked in is NUL, which an old block written as text does not
// carry and which nothing a hint does can turn back into a match: the
// normalizations only take spaces and tabs away, and the scoring that looks
// for the closest line compares bytes. Line breaks inside an occurrence are
// left alone, so the masked content has the same lines as the original and a
// line number found in it is the line number in the file.
//
// The offsets are the ones in content as it was given, so a caller that
// folds CRLF away before asking for a hint masks first and folds afterwards.
func Mask(content, old []byte) []byte {
	masked := bytes.Clone(content)
	for _, pos := range offsets(content, old) {
		for i := pos; i < pos+len(old); i++ {
			if masked[i] != '\n' && masked[i] != '\r' {
				masked[i] = 0
			}
		}
	}
	return masked
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
//
// It ends with what to do next. A hint line may follow, and where one does
// the tail gives up its suggestion to read the file, the hint saying more
// about what is missing than the suggestion could. Nothing else in the tail
// moves, so the way out of a count that is too high reads the same whether a
// hint follows it or not.
func Mismatch(path string, lines []int, expected int, hinted bool) string {
	where := ""
	if len(lines) > 0 {
		where = " (" + lineList(lines) + ")"
	}
	return fmt.Sprintf("%s: old block found %s%s, expected %d%s",
		path, times(len(lines)), where, expected, remedy(len(lines), expected, hinted))
}

// remedy names the way out of a count mismatch, with the count that was
// found filled in so that the caller can use it as it stands. More
// occurrences than expected are cut down by adding context to the old block;
// fewer are not, so there the count is the thing to accept or the block the
// thing to fix. A block that was not found at all has nothing to widen, and
// is either described by the hint that follows or read again from the file.
//
// A hint takes the place of reading the file again, which is the round trip
// it exists to save. With nothing found, that was the whole tail and the
// tail goes; with fewer found than expected, -n is a way out of its own and
// stays. More than expected never asked for a read, and reads the same
// either way.
func remedy(found, expected int, hinted bool) string {
	switch {
	case found == 0 && hinted:
		return ""
	case found == 0:
		return "; no similar text found, read the file again"
	case found > expected:
		return fmt.Sprintf("; widen the old block, or pass -n %d", found)
	case hinted:
		return fmt.Sprintf("; pass -n %d", found)
	default:
		return fmt.Sprintf("; pass -n %d, or read the file again", found)
	}
}

// Surroundings tells the places an old block was found apart from one
// another, so that a block found more times than expected can be widened
// until it is unique, or given up on in favour of -n, without the file being
// read.
//
// What is named for each place is what a wider block would have to take in
// to reach it: the line before it, where the block begins a line, and the
// line it sits in, where the block begins inside one. Places whose line
// reads the same are named together, which is how it shows that widening by
// a line will not tell them apart.
//
// Occurrences that share a line are named once, and where that leaves a
// single place there is nothing to tell apart and nothing is said. The
// offsets are the ones in content as it was given, so the places are the
// same ones the count reported.
func Surroundings(content, old []byte) string {
	fileLines := bytes.Split(content, newline)
	type place struct {
		one, many string
		lines     []int
	}
	var places []place
	where := map[string]int{}
	seen := map[int]bool{}
	var found []int
	for _, pos := range offsets(content, old) {
		line := bytes.Count(content[:pos], newline) + 1
		if seen[line] {
			continue
		}
		seen[line] = true
		found = append(found, line)
		var one, many string
		switch {
		case pos > 0 && content[pos-1] != '\n':
			q := quoteLine(withoutLineEnding(fileLines[line-1]))
			one, many = "sits in "+q, "sit in "+q
		case line == 1:
			// Only one place can start the file, so this one has no plural.
			one, many = "starts the file", "starts the file"
		default:
			q := quoteLine(withoutLineEnding(fileLines[line-2]))
			one, many = "follows "+q, "follow "+q
		}
		i, ok := where[one]
		if !ok {
			i = len(places)
			where[one] = i
			places = append(places, place{one: one, many: many})
		}
		places[i].lines = append(places[i].lines, line)
	}
	if len(found) < 2 {
		return ""
	}
	parts := make([]string, len(places))
	for i, p := range places {
		phrase := p.one
		if len(p.lines) > 1 {
			phrase = p.many
		}
		parts[i] = lineList(p.lines) + " " + phrase
	}
	return fmt.Sprintf("%s (near %s)", strings.Join(parts, ", "), lineList(found))
}

// withoutLineEnding drops the "\r" that a CRLF file leaves at the end of a
// line. Surroundings quotes from the file as it was read, so that this byte
// is still there, and it is the line ending rather than anything a block
// would be written to match.
func withoutLineEnding(line []byte) []byte {
	return bytes.TrimSuffix(line, []byte("\r"))
}

// Hint guesses why old, which does not occur in content, was expected to.
// It reports the closest thing it can find, or "" when nothing comes close.
//
// The guess is made in three steps, and the first one that finds something
// is the answer. First, content and old are compared with one kind of
// whitespace difference ignored at a time, and the first kind that makes
// them match is reported along with what the file actually has. Second, for
// a multi-line block, the longest run of its lines that appears in a row in
// the file is reported, along with the line next to that run that must
// differ, quoted from the file where one line is all that differs. Third,
// for a block of one line, the line that carries most of it is reported and
// quoted. The steps divide the work by what went wrong: how the block is
// spaced, then which of its several lines says something else, then what a
// block of one line says. The hint is only a lead: the count in the
// Mismatch line is what decided that nothing was replaced.
//
// The caller prints it under a "hint:" label. What to change comes first and
// where it is goes in a trailing "(near ...)", because that is the order the
// caller reads in, and because a list of line numbers in front of the text
// runs into it: "near lines 1, 3, file line 1 starts with" hides where the
// list ends.
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
		// An old block of spaces and tabs alone normalizes to nothing,
		// which occurs everywhere and says nothing about what differs.
		normOld := n.apply(old)
		if len(normOld) == 0 {
			continue
		}
		// A description compares old with the file line by line from the
		// start of each line, so it only holds for a match that starts a
		// line. One that starts mid-line is left to the hints below, which
		// quote the line instead of describing it.
		normContent := n.apply(content)
		var lines []int
		for _, pos := range offsets(normContent, normOld) {
			if n.describe != nil && pos > 0 && normContent[pos-1] != '\n' {
				continue
			}
			lines = append(lines, bytes.Count(normContent[:pos], newline)+1)
		}
		if len(lines) == 0 {
			continue
		}
		if n.describe != nil {
			notes = append(notes, n.describe(content, old, lines[0]))
		}
		// The identity normalization has nothing of its own to say, and it
		// only matches at all when the line endings were folded above, which
		// leaves a note behind: old does not occur in content as it stands.
		return fmt.Sprintf("%s (near %s)", strings.Join(notes, "; "), lineList(lines))
	}
	line, note := runHint(content, old)
	if line == 0 {
		line, note = closestLine(content, old)
	}
	if line == 0 {
		return ""
	}
	return fmt.Sprintf("%s (near line %d)", strings.Join(append(notes, note), "; "), line)
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

// describeTrailingWhitespace compares what old and the matched region of
// content carry at the end of each line and reports the first pair that
// differs. One pair must, or the blocks would have matched as they are.
//
// Both sides are named, as the leading and inner descriptions do. Stopping
// at the first line that merely has trailing whitespace would name a line
// that carries the same whitespace on both sides and so matches, and naming
// one side alone reads as if the other end were bare when both carry
// whitespace and only the amount differs.
func describeTrailingWhitespace(content, old []byte, line int) string {
	oldLines, fileLines := region(content, old, line)
	i := 0
	for i+1 < len(oldLines) && bytes.Equal(trailing(fileLines[i]), trailing(oldLines[i])) {
		i++
	}
	return fmt.Sprintf("file line %d ends with %s, old block line %d with %s",
		line+i, describeTrailing(trailing(fileLines[i])), i+1, describeTrailing(trailing(oldLines[i])))
}

// trailing returns the run of spaces and tabs at the end of line.
func trailing(line []byte) []byte {
	return line[len(bytes.TrimRight(line, " \t")):]
}

func describeTrailing(ws []byte) string {
	if len(ws) == 0 {
		return "no trailing whitespace"
	}
	return describeRun(ws)
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
// it covers and the line next to it that must differ, quoted from the file
// where there is one such line, or 0 when there is no run to report. Lines
// are compared whole: a line of old ending a longer
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
	note := differLines(differs)
	// Where one line of old is the only one known to differ, the file line
	// facing it is the one to compare it against, and quoting it saves the
	// reader the trip back to the file. Where two differ, one before the run
	// and one after, neither is the line to fix and quoting one of them
	// would point at the wrong end as often as the right one.
	if len(differs) == 1 {
		facing := bestFile - 1
		if differs[0] > bestOld {
			facing = bestFile + best
		}
		if text, ok := fileLine(fileLines, facing); ok {
			note += fmt.Sprintf(": file line %d is %s", facing+1, quoteLine(text))
		}
	}
	return bestFile + 1, fmt.Sprintf("lines %d-%d of the old block match file lines %d-%d; %s",
		bestOld+1, bestOld+best, bestFile+1, bestFile+best, note)
}

// fileLine returns the line of the file at the 0-based index, and whether
// there is one there. Splitting a file that ends with a line break leaves an
// empty last element that is not a line of the file, and quoting it would
// report an empty line where the file simply ends.
func fileLine(lines [][]byte, i int) ([]byte, bool) {
	if i < 0 || i >= len(lines) || (i == len(lines)-1 && len(lines[i]) == 0) {
		return nil, false
	}
	return lines[i], true
}

// closestLine finds the file line that carries most of a one-line old block
// and reports it with the line quoted, or 0 when no line carries enough of
// it. It is the last thing a hint tries: the normalizations have found no
// whitespace difference that explains the block, and runHint works on two
// lines or more, so a block of one line that differs in what it says has
// nothing to report without this.
//
// A line has to carry minOverlap bytes of the block, and half of it, before
// it is named. What is being scored is a lead for a reader who will look at
// the line, and a line that shares less than half of a block is one the
// reader would not recognize. On a tie the earliest line in the file wins,
// as it does in runHint.
//
// A block of several lines is left to runHint. Scoring one against a single
// line of the file would be comparing things of different shapes, and the
// line that came out of it would be named on the strength of whichever of
// the block's lines it resembled.
func closestLine(content, old []byte) (int, string) {
	if bytes.Contains(old, newline) || len(old) < minOverlap {
		return 0, ""
	}
	best, bestLine := 0, 0
	var bestText []byte
	for i, line := range bytes.Split(content, newline) {
		if score := overlap(line, old); score > best {
			best, bestLine, bestText = score, i+1, line
		}
	}
	if best < max(minOverlap, len(old)/2) {
		return 0, ""
	}
	return bestLine, fmt.Sprintf("the closest line is file line %d: %s", bestLine, quoteLine(bestText))
}

// overlap scores how much of old a line carries: the longest run of bytes
// old begins with that appears somewhere in the line, plus the longest run
// it ends with that appears after that run, the two chosen together.
//
// Choosing them together is what makes the score follow a typo. Taking the
// longest head first and looking for a tail behind it spends the head on
// whatever short run repeats earliest in the line, and the tail is then
// hunted in what little is left; a block whose middle word was mistyped
// scores near nothing that way, and near all of itself this way.
//
// The two runs must sit apart in the line by no more than gapSlack bytes
// more than they do in old, which is what stops a long line from scoring by
// carrying the head at one end and the tail at the other. A run of zero is
// allowed on either side, and is what a block whose beginning or end was
// mistyped falls back to; with one run there is no distance to check.
func overlap(line, old []byte) int {
	suffixes := make([]int, len(line)+1)
	for k := range suffixes {
		suffixes[k] = commonSuffix(old, line[:k])
	}
	best := 0
	for _, s := range suffixes {
		best = max(best, s)
	}
	for i := 0; i <= len(line); i++ {
		p := commonPrefix(old, line[i:])
		if p == 0 {
			continue
		}
		best = max(best, p)
		// The bound on k is the rule about the distance between the two
		// runs. Asking that the line hold no more than gapSlack bytes more
		// between them than old does is, once the runs themselves cancel
		// from both sides, the same as asking that the tail end no further
		// than the whole of old plus the slack past where the head began.
		for k := i + p; k <= len(line) && k <= i+len(old)+gapSlack; k++ {
			s := min(suffixes[k], len(old)-p)
			if s == 0 || k-s < i+p {
				continue
			}
			best = max(best, p+s)
		}
	}
	return best
}

// commonPrefix returns the number of bytes a and b begin with in common.
func commonPrefix(a, b []byte) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// commonSuffix returns the number of bytes a and b end with in common.
func commonSuffix(a, b []byte) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}

// differLines words the lines of old, next to the matched run, that are
// known not to match: one before it, one after it, or both.
func differLines(lines []int) string {
	if len(lines) == 1 {
		return fmt.Sprintf("line %d differs", lines[0])
	}
	return fmt.Sprintf("lines %d and %d differ", lines[0], lines[1])
}

// quoteLine renders a line of the file so that a hint can carry what the
// file actually says, not only where it is. It is quoted the way Go quotes a
// string, which makes tabs and control characters visible: an old block that
// was typed with the wrong whitespace differs in exactly those bytes, and a
// bare copy of the line would hide the difference it was printed to show.
//
// Only one line is ever quoted, and a long one is cut to quoteMax bytes with
// an ellipsis after the closing quote. The cut is by bytes, so it can fall
// inside a multi-byte character; the quoting then shows that byte as an
// escape, which is honest about what is there and is followed by the
// ellipsis that says the line goes on.
//
// Whatever the line holds is shown, a "\r" at its end included: a hint that
// quotes from content whose line endings were folded away is quoting one the
// file really has there. Surroundings, which works on the file as it was
// read, takes the line ending off itself.
func quoteLine(line []byte) string {
	if len(line) > quoteMax {
		return strconv.Quote(string(line[:quoteMax])) + "..."
	}
	return strconv.Quote(string(line))
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
