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
  sub1 [-n N] [-d SEP] [--] FILE <<'SUB1'
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

  sub1 handlers.py <<'SUB1'         # an empty new block deletes the old one
  import os
  ====
  ====
  SUB1

Options:
  -n N            expect OLD N times (default 1)
  -d SEP          line that ends OLD and ends NEW (default "====")
  -h, --help      show this help
  -v, --version   show the version
  --instructions  print the paragraph for the agent's instruction file

Exit codes:
  0  replaced
  1  OLD found a different number of times than expected
  2  input error: the flags or the blocks on stdin
  3  file error: stdin or FILE could not be read, or FILE could not be written
`

// instructionsText is the paragraph a coding agent needs in order to use
// sub1: that it exists, when to reach for it, and the shape of the call.
//
// What to do when a call fails is not here. Every message sub1 prints ends
// with the way out, so the same advice in the instruction file would be a
// second copy to keep in step, read on every call to cover the calls that
// fail. The one thing left is that the file is only rewritten on an exact
// match, which never shows up in a message because it is what happens when
// nothing goes wrong.
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
	"The file is rewritten only when the old block occurs exactly once. Otherwise nothing is written and the message says what to do next. See `sub1 --help` for options.\n"

type cliArgs struct {
	showHelp         bool
	showVersion      bool
	showInstructions bool
	expected         int
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
					return cliArgs{}, fmt.Errorf("multiple files given")
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
		default:
			if strings.HasPrefix(arg, "-") {
				return cliArgs{}, fmt.Errorf("unknown flag: %s", arg)
			}
			if a.path != "" {
				return cliArgs{}, fmt.Errorf("multiple files given")
			}
			a.path = arg
		}
	}
	if a.path == "" && !a.showHelp && !a.showVersion && !a.showInstructions {
		return cliArgs{}, fmt.Errorf("no file given")
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
//  3. injected (devVersion) as the final fallback.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != devVersion {
		return injected
	}
	if info != nil {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return v
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
// given on the command line. The os errors name a path of their own — the
// symlink target, or the directory the temporary file goes in — and lead
// with the operation that failed ("open", "lstat"), which says more about
// how sub1 is built than about what the caller has to fix.
func fileError(path string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return fmt.Errorf("%s: %w", path, err)
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
	blocks, err := block.Split(input, []byte(a.separator))
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}

	content, err := os.ReadFile(a.path)
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", fileError(a.path, err))
		return exitFile
	}
	// The blocks come from a heredoc and end their lines with LF. A file
	// that ends every line with CRLF is edited as CRLF: both blocks are
	// converted before matching, and the hint, if one is needed, compares
	// the LF forms so that line endings do not drown out the real difference.
	old, new := blocks.Old, blocks.New
	asCRLF := crlf.Uniform(content) && !bytes.ContainsRune(old, '\r') && !bytes.ContainsRune(new, '\r')
	if asCRLF {
		old, new = crlf.ToCRLF(old), crlf.ToCRLF(new)
	}
	lines := occur.Lines(content, old)
	if len(lines) != a.expected {
		// The hint is worked out first: when there is one, it takes the
		// place of the general suggestion the count line would otherwise
		// end with.
		hint := ""
		if len(lines) == 0 {
			hintContent := content
			if asCRLF {
				hintContent = crlf.ToLF(content)
			}
			hint = occur.Hint(hintContent, blocks.Old)
		}
		fmt.Fprintln(stderr, "sub1:", occur.Mismatch(a.path, lines, a.expected, hint != ""))
		if hint != "" {
			fmt.Fprintf(stderr, "  hint: %s\n", hint)
		}
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
