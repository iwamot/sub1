package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/iwamot/sub1/internal/atomicfile"
	"github.com/iwamot/sub1/internal/block"
	"github.com/iwamot/sub1/internal/crlf"
	"github.com/iwamot/sub1/internal/occur"
)

// The exit codes separate the three things that can go wrong, because each
// asks the caller for a different fix: the old block or -n, the command, or
// the paths and permissions around it. Code 3 covers every failure to read
// or write, stdin included, as against code 2, where the input arrived and
// sub1 refused it.
const (
	exitOK       = 0
	exitMismatch = 1
	exitUsage    = 2
	exitFile     = 3
)

const devVersion = "0.0.0-dev"

var version = devVersion

const defaultSeparator = "===="

const helpText = `sub1 — replace a literal text block in a file, exactly once.

Usage:
  sub1 [-n N | --pairs] [-d SEP] [--] FILE <<'SUB1'
  old text (one or more lines)
  ====
  new text (zero or more lines)
  ====
  SUB1

OLD and NEW come from stdin, each followed by a line equal to SEP. The final
newline of each block is dropped. FILE is rewritten only when OLD occurs
exactly N times; otherwise it is left untouched.

Examples:
  sub1 app.py <<'SUB1'              # replace the one occurrence
  old line
  ====
  new line
  ====
  SUB1

  sub1 -n 3 handlers.py <<'SUB1'    # replace 3, and fail if there are not 3
  _tool_chunks(
  ====
  tool_chunks(
  ====
  SUB1

  sub1 -d @@ README.md <<'SUB1'     # a block holds a "====" line
  Title
  ====
  @@
  Title
  -----
  @@
  SUB1

  sub1 handlers.py <<'SUB1'         # an empty new block deletes the whole line
  import os
  ====
  ====
  SUB1

Options:
  -n N            expect OLD N times and replace all N (default 1)
  -d SEP          line that ends OLD and ends NEW (default "====")
  --pairs         read several OLD/NEW pairs in a row, each OLD exactly once
  -h, --help      show this help
  -v, --version   show the version
  --instructions  print the paragraph for the agent's instruction file

Notes:
  "(...)" after "replaced at line N" is a note, not an error: check the edit.

Exit codes:
  0  replaced
  1  OLD found a different number of times than expected, or pairs overlap
  2  input error: the flags or the blocks on stdin
  3  file error: stdin or FILE could not be read, or FILE could not be written
`

// instructionsText is the paragraph a coding agent needs in order to use
// sub1: that it exists, when to reach for it, and the shape of the call.
//
// What to do when a call fails is not here. Wherever there is a way out,
// the message that reports the failure carries it, so the same advice in the
// instruction file would be a second copy to keep in step, read on every
// call to cover the calls that fail. A message with no such tail is one
// nothing can be suggested for — a file that is not there. The one thing
// left is that the file is only rewritten on an exact match, which never
// shows up in a message because it is what happens when nothing goes wrong.
//
// README.md quotes this paragraph verbatim.
const instructionsText = "To replace part of a file from the shell, use `sub1` instead of sed or an ad-hoc script:\n" +
	"\n" +
	"    sub1 FILE <<'SUB1'\n" +
	"    old lines\n" +
	"    ====\n" +
	"    new lines\n" +
	"    ====\n" +
	"    SUB1\n" +
	"\n" +
	"The file is rewritten only when the old block occurs exactly once. Otherwise nothing is written and the message says what to do next. To change several places in one file, pass `--pairs` and write the old/new pairs one after another. See `sub1 --help` for options.\n"

// errMultipleFiles is worded once because parseArgs reaches it from both
// sides of "--".
var errMultipleFiles = fmt.Errorf("multiple files given; sub1 edits one file per call")

type cliArgs struct {
	showHelp         bool
	showVersion      bool
	showInstructions bool
	expected         int
	countGiven       bool
	pairs            bool
	separator        string
	path             string
}

func parseArgs(argv []string) (cliArgs, error) {
	a := cliArgs{expected: 1, separator: defaultSeparator}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			// Everything after "--" is a file name, even one that starts
			// with "-".
			for _, arg := range argv[i+1:] {
				if a.path != "" {
					return cliArgs{}, errMultipleFiles
				}
				a.path = arg
			}
			break
		}
		switch arg {
		case "-h", "--help":
			a.showHelp = true
		case "-v", "--version":
			a.showVersion = true
		case "--instructions":
			a.showInstructions = true
		case "--pairs":
			a.pairs = true
		case "-n", "-d":
			if i+1 >= len(argv) {
				return cliArgs{}, fmt.Errorf("%s needs a value", arg)
			}
			i++
			value := argv[i]
			if arg == "-d" {
				if value == "" {
					return cliArgs{}, fmt.Errorf("-d needs a non-empty separator")
				}
				a.separator = value
				continue
			}
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return cliArgs{}, fmt.Errorf("-n needs a positive integer, got %q", value)
			}
			a.expected = n
			a.countGiven = true
		default:
			if strings.HasPrefix(arg, "-") {
				return cliArgs{}, fmt.Errorf("unknown flag: %s; see --help for the flags", arg)
			}
			if a.path != "" {
				return cliArgs{}, errMultipleFiles
			}
			a.path = arg
		}
	}
	if a.path == "" && !a.showHelp && !a.showVersion && !a.showInstructions {
		return cliArgs{}, fmt.Errorf("no file given; pass the file to edit as the last argument")
	}
	// Each pair is expected once. One -n for all of them would almost never
	// be right, since the pairs that need it rarely need the same count, and
	// refusing the two together leaves room to give each pair its own count
	// later without changing what a call means now.
	if a.pairs && a.countGiven {
		return cliArgs{}, fmt.Errorf("-n cannot be used with --pairs; replace a block that occurs more than once in a call of its own with -n")
	}
	return a, nil
}

// resolveVersion picks the most authoritative version string available.
//
// Priority:
//  1. injected (set via `-ldflags '-X main.version=...'` during a GoReleaser
//     build) when it differs from devVersion.
//  2. info.Main.Version when present and not "(devel)" or "" — this is what
//     `go install module@vX.Y.Z` records, even though ldflags don't apply.
//     Its leading "v" is dropped, so every way of installing prints the
//     version as GoReleaser and the Homebrew cask spell it.
//  3. injected (devVersion) as the final fallback.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != devVersion {
		return injected
	}
	if info != nil {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return injected
}

// isTerminal reports whether r is an interactive terminal, where waiting for
// a heredoc that will never come would look like a hang.
//
// The null device is a character device too, but reading it returns at once,
// so it is excluded. That also covers a closed stdin, which the Go runtime
// reopens on the null device at startup.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(info, null) {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// fileError words a file error as "FILE: what", with FILE the path as it was
// given on the command line.
func fileError(path string, err error) error {
	return fmt.Errorf("%s: %w", path, bare(err))
}

// bare strips the wrapping the os errors come in. They name a path of their
// own — the symlink target, or the directory the temporary file goes in —
// and lead with the operation that failed ("open", "lstat"), which says more
// about how sub1 is built than about what the caller has to fix.
func bare(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
}

// readTarget reads the file to edit. A file that is there but cannot be read
// fails with the same words as one that cannot be written, so this one names
// the operation. The other read failures — no such file, a directory — say
// what is wrong on their own, and naming the operation would only pad them.
func readTarget(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			err = fmt.Errorf("cannot read the file: %w", bare(err))
		}
		return nil, fileError(path, err)
	}
	return content, nil
}

// mismatchLines reports one count that is not the expected one: the count
// line that count builds, and the hint that may follow it.
func mismatchLines(count func(hinted bool) string, content, old, lfOld []byte, lines []int, expected int, asCRLF bool) []string {
	// The hint is worked out first: a count line that is followed by one
	// leaves out the suggestion to read the file that it would otherwise
	// end with.
	hint := ""
	if len(lines) < expected {
		// Mask takes the occurrences that were found out of the way, before
		// the fold, so that the hint describes what is missing rather than a
		// match that is already there.
		hintContent := occur.Mask(content, old)
		if asCRLF {
			hintContent = crlf.ToLF(hintContent)
		}
		hint = occur.Hint(hintContent, lfOld)
	} else {
		// Nothing is missing here. What helps instead is telling the places
		// that were found apart, to widen the block around one.
		hint = occur.Surroundings(content, old)
	}
	out := []string{"sub1: " + count(hint != "")}
	if hint != "" {
		out = append(out, "  hint: "+hint)
	}
	return out
}

// runPairs is run for --pairs: every pair is checked against the file as it
// is, and the file is written once, only when every old block occurs exactly
// once and no two of them share text. A pair that fails is reported with its
// number and the report a single replacement would give, and every failing
// pair is reported, so that one call says all there is to fix.
func runPairs(a cliArgs, input []byte, stdout, stderr io.Writer) int {
	pairs, err := block.SplitPairs(input, []byte(a.separator))
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}
	content, err := readTarget(a.path)
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitFile
	}
	asCRLF := crlf.Uniform(content)
	for _, p := range pairs {
		if bytes.ContainsRune(p.Old, '\r') || bytes.ContainsRune(p.New, '\r') {
			asCRLF = false
		}
	}
	type pair struct {
		label    string
		old, new []byte
		lines    []int
	}
	checked := make([]pair, len(pairs))
	var report []string
	for i, p := range pairs {
		old, new := p.Old, p.New
		if asCRLF {
			old, new = crlf.ToCRLF(old), crlf.ToCRLF(new)
		}
		label := fmt.Sprintf("%s: pair %d", a.path, i+1)
		lines := occur.Lines(content, old)
		checked[i] = pair{label, old, new, lines}
		if len(lines) != 1 {
			count := func(hinted bool) string { return occur.PairMismatch(label, lines, hinted) }
			report = append(report, mismatchLines(count, content, old, p.Old, lines, 1, asCRLF)...)
		}
	}
	if len(report) > 0 {
		fmt.Fprintln(stderr, strings.Join(report, "\n"))
		return exitMismatch
	}
	sets := make([][]occur.Edit, len(checked))
	for i, p := range checked {
		sets[i] = occur.Edits(content, p.old, p.new)
	}
	edits, overlap := occur.Merge(content, sets)
	if overlap != nil {
		fmt.Fprintf(stderr, "sub1: %s: pair %d overlaps pair %d at line %d; shorten one of the two old blocks so they do not share text\n",
			a.path, overlap.Second+1, overlap.First+1, overlap.Line)
		return exitMismatch
	}
	if err := atomicfile.WriteFile(a.path, occur.Apply(content, edits)); err != nil {
		fmt.Fprintln(stderr, "sub1:", fileError(a.path, err))
		return exitFile
	}
	for _, p := range checked {
		var notes []string
		if asCRLF {
			notes = append(notes, "CRLF")
		}
		notes = append(notes, occur.Notes(content, p.old, p.new)...)
		fmt.Fprintln(stdout, occur.Summary(p.label, p.lines, notes))
	}
	return exitOK
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	a, err := parseArgs(argv)
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}
	if a.showHelp {
		fmt.Fprint(stdout, helpText)
		return exitOK
	}
	if a.showVersion {
		info, _ := debug.ReadBuildInfo()
		fmt.Fprintln(stdout, resolveVersion(version, info))
		return exitOK
	}
	if a.showInstructions {
		fmt.Fprint(stdout, instructionsText)
		return exitOK
	}

	if isTerminal(stdin) {
		fmt.Fprintln(stderr, "sub1: stdin is a terminal; pass the old and new blocks in a heredoc (see --help)")
		return exitUsage
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		// Reading stdin failed, which is the other side of exit 3: not an
		// input sub1 refuses, but one it could not take in. "stdin" stands
		// where a path would.
		fmt.Fprintln(stderr, "sub1:", fileError("stdin", err))
		return exitFile
	}
	if a.pairs {
		return runPairs(a, input, stdout, stderr)
	}
	blocks, err := block.Split(input, []byte(a.separator))
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}

	content, err := readTarget(a.path)
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitFile
	}
	// The blocks come from a heredoc and end their lines with LF. A file
	// that ends every line with CRLF is edited as CRLF: both blocks are
	// converted before matching, and the hint that has to compare them, the
	// one for a count that fell short, is handed the LF forms instead so
	// that line endings do not drown out the real difference.
	old, new := blocks.Old, blocks.New
	asCRLF := crlf.Uniform(content) && !bytes.ContainsRune(old, '\r') && !bytes.ContainsRune(new, '\r')
	if asCRLF {
		old, new = crlf.ToCRLF(old), crlf.ToCRLF(new)
	}
	lines := occur.Lines(content, old)
	if len(lines) != a.expected {
		count := func(hinted bool) string { return occur.Mismatch(a.path, lines, a.expected, hinted) }
		fmt.Fprintln(stderr, strings.Join(mismatchLines(count, content, old, blocks.Old, lines, a.expected, asCRLF), "\n"))
		return exitMismatch
	}
	if err := atomicfile.WriteFile(a.path, occur.Replace(content, old, new)); err != nil {
		fmt.Fprintln(stderr, "sub1:", fileError(a.path, err))
		return exitFile
	}
	var notes []string
	if asCRLF {
		notes = append(notes, "CRLF")
	}
	notes = append(notes, occur.Notes(content, old, new)...)
	fmt.Fprintln(stdout, occur.Summary(a.path, lines, notes))
	return exitOK
}
