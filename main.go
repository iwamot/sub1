package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/iwamot/sub1/internal/atomicfile"
	"github.com/iwamot/sub1/internal/block"
	"github.com/iwamot/sub1/internal/occur"
)

const (
	exitOK       = 0
	exitMismatch = 1
	exitUsage    = 2
)

const devVersion = "0.0.0-dev"

var version = devVersion

const defaultSeparator = "===="

const helpText = `sub1 — replace a literal text block in a file, exactly once.

Usage:
  sub1 [-n N] [-d SEP] FILE <<'SUB1'
  old text (one or more lines)
  ====
  new text (zero or more lines)
  SUB1
  sub1 -h, --help
  sub1 -v, --version

OLD and NEW come from stdin, split at the single line equal to SEP (default
"===="). The final newline of each block is dropped. FILE is rewritten only
when OLD occurs exactly N times (default 1); otherwise it is left untouched.

Exit codes:
  0  replaced
  1  OLD found a different number of times than expected
  2  usage error, or FILE could not be read or written
`

type cliArgs struct {
	showHelp    bool
	showVersion bool
	expected    int
	separator   string
	path        string
}

func parseArgs(argv []string) (cliArgs, error) {
	a := cliArgs{expected: 1, separator: defaultSeparator}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-h", "--help":
			a.showHelp = true
		case "-v", "--version":
			a.showVersion = true
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
	if a.path == "" && !a.showHelp && !a.showVersion {
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

	if isTerminal(stdin) {
		fmt.Fprintln(stderr, "sub1: stdin is a terminal; pass the old and new blocks in a heredoc (see --help)")
		return exitUsage
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}
	blocks, err := block.Split(input, []byte(a.separator))
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}

	content, err := os.ReadFile(a.path)
	if err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}
	lines := occur.Lines(content, blocks.Old)
	if len(lines) != a.expected {
		fmt.Fprintln(stderr, "sub1:", occur.Mismatch(a.path, lines, a.expected))
		if len(lines) == 0 {
			if hint := occur.Hint(content, blocks.Old); hint != "" {
				fmt.Fprintf(stderr, "  %s\n", hint)
			}
		}
		return exitMismatch
	}
	if err := atomicfile.WriteFile(a.path, bytes.ReplaceAll(content, blocks.Old, blocks.New)); err != nil {
		fmt.Fprintln(stderr, "sub1:", err)
		return exitUsage
	}
	fmt.Fprintln(stdout, occur.Summary(a.path, lines))
	return exitOK
}
