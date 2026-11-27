package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	def := cliArgs{expected: 1, separator: defaultSeparator}
	with := func(f func(*cliArgs)) cliArgs { a := def; f(&a); return a }
	tests := []struct {
		name    string
		argv    []string
		want    cliArgs
		wantErr string
	}{
		{"file only", []string{"f.txt"}, with(func(a *cliArgs) { a.path = "f.txt" }), ""},
		{"count before file", []string{"-n", "3", "f.txt"}, with(func(a *cliArgs) { a.expected = 3; a.countGiven = true; a.path = "f.txt" }), ""},
		{"count after file", []string{"f.txt", "-n", "3"}, with(func(a *cliArgs) { a.expected = 3; a.countGiven = true; a.path = "f.txt" }), ""},
		{"separator", []string{"-d", "%%", "f.txt"}, with(func(a *cliArgs) { a.separator = "%%"; a.path = "f.txt" }), ""},
		{"both flags", []string{"f.txt", "-d", "%%", "-n", "2"}, with(func(a *cliArgs) { a.separator = "%%"; a.expected = 2; a.countGiven = true; a.path = "f.txt" }), ""},
		{"help short", []string{"-h"}, with(func(a *cliArgs) { a.showHelp = true }), ""},
		{"help long", []string{"--help"}, with(func(a *cliArgs) { a.showHelp = true }), ""},
		{"version short", []string{"-v"}, with(func(a *cliArgs) { a.showVersion = true }), ""},
		{"version long", []string{"--version"}, with(func(a *cliArgs) { a.showVersion = true }), ""},
		{"instructions", []string{"--instructions"}, with(func(a *cliArgs) { a.showInstructions = true }), ""},
		{"dash-dash before file", []string{"-n", "2", "--", "-f.txt"}, with(func(a *cliArgs) { a.expected = 2; a.countGiven = true; a.path = "-f.txt" }), ""},
		{"dash-dash after file", []string{"f.txt", "--"}, with(func(a *cliArgs) { a.path = "f.txt" }), ""},
		{"dash-dash alone", []string{"--"}, cliArgs{}, "no file given"},
		{"dash-dash then two files", []string{"--", "a", "b"}, cliArgs{}, "multiple files"},
		{"file then dash-dash then file", []string{"a", "--", "b"}, cliArgs{}, "multiple files"},
		{"flag after dash-dash is a file", []string{"--", "-n"}, with(func(a *cliArgs) { a.path = "-n" }), ""},
		{"no file", nil, cliArgs{}, "no file given"},
		{"unknown flag", []string{"--bogus", "f.txt"}, cliArgs{}, "unknown flag"},
		{"two files", []string{"a", "b"}, cliArgs{}, "multiple files"},
		{"count missing value", []string{"f.txt", "-n"}, cliArgs{}, "-n needs a value"},
		{"count zero", []string{"-n", "0", "f.txt"}, cliArgs{}, "positive integer"},
		{"count negative", []string{"-n", "-1", "f.txt"}, cliArgs{}, "positive integer"},
		{"count not a number", []string{"-n", "x", "f.txt"}, cliArgs{}, "positive integer"},
		{"separator missing value", []string{"f.txt", "-d"}, cliArgs{}, "-d needs a value"},
		{"separator empty", []string{"-d", "", "f.txt"}, cliArgs{}, "non-empty"},
		{"pairs", []string{"--pairs", "f.txt"}, with(func(a *cliArgs) { a.pairs = true; a.path = "f.txt" }), ""},
		{"pairs with separator", []string{"f.txt", "--pairs", "-d", "%%"}, with(func(a *cliArgs) { a.pairs = true; a.separator = "%%"; a.path = "f.txt" }), ""},
		{"pairs with count", []string{"--pairs", "-n", "2", "f.txt"}, cliArgs{}, "-n cannot be used with --pairs"},
		{"count with pairs", []string{"-n", "1", "f.txt", "--pairs"}, cliArgs{}, "-n cannot be used with --pairs"},
		{"range", []string{"--range", "f.txt"}, with(func(a *cliArgs) { a.rangeMode = true; a.path = "f.txt" }), ""},
		{"range with count", []string{"--range", "-n", "2", "f.txt"}, cliArgs{}, "-n cannot be used with --range"},
		{"range with pairs", []string{"--range", "--pairs", "f.txt"}, cliArgs{}, "--pairs cannot be used with --range; replace each stretch in a call of its own"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.argv)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseArgs err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs err = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseArgs = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		want     string
	}{
		{"injected non-dev wins over build info", "1.2.3", &debug.BuildInfo{Main: debug.Module{Version: "9.9.9"}}, "1.2.3"},
		{"injected non-dev wins with no build info", "1.2.3", nil, "1.2.3"},
		{"dev falls back to build info Main.Version, without the v", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "v0.0.3"}}, "0.0.3"},
		{"only the leading v is dropped", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0-dev"}}, "1.0.0-dev"},
		{"dev with (devel) build info falls through to dev", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, devVersion},
		{"dev with empty build info version falls through to dev", devVersion, &debug.BuildInfo{}, devVersion},
		{"dev with nil build info falls through to dev", devVersion, nil, devVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.injected, tt.info); got != tt.want {
				t.Errorf("resolveVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readBack(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

func runWith(argv []string, stdin string) runResult {
	var so, se bytes.Buffer
	code := run(argv, strings.NewReader(stdin), &so, &se)
	return runResult{code, so.String(), se.String()}
}

func TestRun_replacesExactlyOnce(t *testing.T) {
	path := writeTemp(t, "a\n  foo $x `y` \\n \"q\"\n  bar\nend\n")
	r := runWith([]string{path}, "  foo $x `y` \\n \"q\"\n  bar\n====\n  FOO\n  BAR\n  BAZ\n====\n")
	if r.code != exitOK || r.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": replaced at line 2\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "a\n  FOO\n  BAR\n  BAZ\nend\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_keepsFileMode(t *testing.T) {
	path := writeTemp(t, "x\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if r := runWith([]string{path}, "x\n====\ny\n====\n"); r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 640", info.Mode().Perm())
	}
}

func TestRun_deletesTheLineWhenNewIsEmpty(t *testing.T) {
	path := writeTemp(t, "a\nb\nc\n")
	r := runWith([]string{path}, "b\n====\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "a\nc\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if got, want := r.stdout, path+": replaced at line 2\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRun_deletesWhenNewIsEmptyAndOldEndsWithABlankLine(t *testing.T) {
	path := writeTemp(t, "a\nb\n\nc\n")
	if r := runWith([]string{path}, "b\n\n====\n====\n"); r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "a\n\nc\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_deletesTheLineWhenNewIsEmptyInCRLFFile(t *testing.T) {
	path := writeTemp(t, "a\r\nb\r\nc\r\n")
	r := runWith([]string{path}, "b\n====\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "a\r\nc\r\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if got, want := r.stdout, path+": replaced at line 2 (CRLF)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRun_deletesEachOccurrenceOnItsOwnWhenNewIsEmpty(t *testing.T) {
	path := writeTemp(t, "b\nab\nb\n")
	r := runWith([]string{"-n", "3", path}, "b\n====\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "a\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if got, want := r.stdout, path+": replaced at lines 1, 2, 3 (1 match starts inside an identifier)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRun_notesFollowCRLFInOneParenthesis(t *testing.T) {
	path := writeTemp(t, "one\r\nmax = 1\r\n")
	r := runWith([]string{path}, "x = 1\n====\nx = 2\n====\n")
	if r.code != exitOK || r.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": replaced at line 2 (CRLF; 1 match starts inside an identifier)\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "one\r\nmax = 2\r\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_secondRunOfAnInsertionIsNoted(t *testing.T) {
	path := writeTemp(t, "import os\n")
	stdin := "import os\n====\nimport os\nimport sys\n====\n"
	first := runWith([]string{path}, stdin)
	if first.code != exitOK || first.stdout != path+": replaced at line 1\n" {
		t.Fatalf("first run: exit = %d, stdout = %q, stderr = %q", first.code, first.stdout, first.stderr)
	}
	second := runWith([]string{path}, stdin)
	if second.code != exitOK || second.stderr != "" {
		t.Fatalf("second run: exit = %d, stderr = %q", second.code, second.stderr)
	}
	if want := path + ": replaced at line 1 (the file already contained the new block)\n"; second.stdout != want {
		t.Errorf("stdout = %q, want %q", second.stdout, want)
	}
	if got, want := readBack(t, path), "import os\nimport sys\nimport sys\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_fileNameStartingWithDash(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("-n", []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runWith([]string{"--", "-n"}, "x\n====\ny\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got := readBack(t, "-n"); got != "y\n" {
		t.Errorf("file = %q, want %q", got, "y\n")
	}
}

func TestRun_noTrailingNewlineInFile(t *testing.T) {
	path := writeTemp(t, "abc")
	if r := runWith([]string{path}, "b\n====\nB\n====\n"); r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got := readBack(t, path); got != "aBc" {
		t.Errorf("file = %q, want %q", got, "aBc")
	}
}

func TestRun_countMismatchLeavesFileUntouched(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		wantLine string
	}{
		{"absent", nil, "old block found 0 times, expected 1; no similar text found, read the file again\n"},
		{"duplicate", nil, "old block found 2 times (lines 1, 2), expected 1; widen the old block to match only 1, or pass -n 2 to replace all 2\n  hint: line 1 starts the file, line 2 follows \"x\" (near lines 1, 2)\n"},
		{"fewer than -n", []string{"-n", "3"}, "old block found 2 times (lines 1, 2), expected 3; pass -n 2, or read the file again\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := "x\nx\n"
			old := "x"
			if tt.name == "absent" {
				old = "nope"
			}
			path := writeTemp(t, original)
			r := runWith(append([]string{path}, tt.argv...), old+"\n====\ny\n====\n")
			if r.code != exitMismatch {
				t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
			}
			if r.stdout != "" {
				t.Errorf("stdout = %q, want empty", r.stdout)
			}
			if want := "sub1: " + path + ": " + tt.wantLine; r.stderr != want {
				t.Errorf("stderr = %q, want %q", r.stderr, want)
			}
			if got := readBack(t, path); got != original {
				t.Errorf("file changed to %q", got)
			}
		})
	}
}

func TestRun_absentBlockGetsHint(t *testing.T) {
	path := writeTemp(t, "a\n\tb\n")
	r := runWith([]string{path}, "a\n  b\n====\nc\n====\n")
	if r.code != exitMismatch {
		t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": old block found 0 times, expected 1\n" +
		"  hint: file line 2 starts with 1 tab, old block line 2 with 2 spaces (near line 1)\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

// A count that fell short leaves something to look for, and the hint
// describes it while the count line keeps -n and drops the suggestion to
// read the file again.
//
// The occurrence that matched sits after the one that did not, so that a
// mask taken in the wrong space would land on the wrong bytes: the offsets
// are the file's own, and a CRLF file is folded only afterwards.
func TestRun_fewerThanExpectedGetsHint(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{"LF", "a\n\tb\na\n  b\n", "file line 2 starts with 1 tab, old block line 2 with 2 spaces (near line 1)"},
		{"CRLF throughout", "a\r\n\tb\r\na\r\n  b\r\n", "file line 2 starts with 1 tab, old block line 2 with 2 spaces (near line 1)"},
		{"mixed line endings", "a\r\n\tb\r\na\n  b\n", "file line 1 ends with CRLF, old block line 1 with LF; file line 2 starts with 1 tab, old block line 2 with 2 spaces (near line 1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.file)
			r := runWith([]string{path, "-n", "2"}, "a\n  b\n====\nc\n====\n")
			if r.code != exitMismatch {
				t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
			}
			want := "sub1: " + path + ": old block found once (line 3), expected 2; pass -n 1\n" +
				"  hint: " + tt.want + "\n"
			if r.stderr != want {
				t.Errorf("stderr = %q, want %q", r.stderr, want)
			}
			if got := readBack(t, path); got != tt.file {
				t.Errorf("file changed to %q", got)
			}
		})
	}
}

// The two halves of the hint work together: what was found is masked out,
// and what is left to find is the line that carries most of the block. This
// is the shape the failures take most often — a block of one line, mistyped.
func TestRun_fewerThanExpectedFindsTheMistypedLine(t *testing.T) {
	original := "value := compute(alpha)\nvalue := compute(alpah)\n"
	path := writeTemp(t, original)
	r := runWith([]string{path, "-n", "2"}, "value := compute(alpha)\n====\nvalue := compute(beta)\n====\n")
	if r.code != exitMismatch {
		t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": old block found once (line 1), expected 2; pass -n 1\n" +
		"  hint: the closest line is file line 2: \"value := compute(alpah)\" (near line 2)\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

// More occurrences than expected leave nothing missing to look for, and what
// is worth saying is what tells the places apart. Here the two places read
// the same, which is what says that widening the block will not separate
// them and -n is the way out.
func TestRun_moreThanExpectedNamesThePlaces(t *testing.T) {
	original := "def handle(x):\n    return _tool_chunks(x)\ndef retry(x):\n    return _tool_chunks(x)\n"
	path := writeTemp(t, original)
	r := runWith([]string{path}, "_tool_chunks(\n====\ntool_chunks(\n====\n")
	if r.code != exitMismatch {
		t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": old block found 2 times (lines 2, 4), expected 1; widen the old block to match only 1, or pass -n 2 to replace all 2\n" +
		"  hint: lines 2, 4 sit in \"    return _tool_chunks(x)\" (near lines 2, 4)\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if got := readBack(t, path); got != original {
		t.Errorf("file changed to %q", got)
	}
}

func TestRun_crlfFileIsEditedAsCRLF(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		stdin string
		want  string
	}{
		{"multi-line blocks", "one\r\ntwo\r\nthree\r\n", "two\nthree\n====\ntwo\nfour\n====\n", "one\r\ntwo\r\nfour\r\n"},
		{"one-line old, multi-line new", "one\r\ntwo\r\nthree\r\n", "two\n====\ntwo\ntwo-and-a-half\n====\n", "one\r\ntwo\r\ntwo-and-a-half\r\nthree\r\n"},
		{"no final line break", "one\r\ntwo", "two\n====\n2\n====\n", "one\r\n2"},
		{"deletion keeps the line breaks it was shown", "a\r\nb\r\nc\r\n", "a\nb\n====\na\n====\n", "a\r\nc\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.file)
			r := runWith([]string{path}, tt.stdin)
			if r.code != exitOK || r.stderr != "" {
				t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
			}
			if !strings.HasSuffix(r.stdout, " (CRLF)\n") {
				t.Errorf("stdout = %q, want it to end with (CRLF)", r.stdout)
			}
			if got := readBack(t, path); got != tt.want {
				t.Errorf("file = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRun_crlfFileHintIgnoresLineEndings(t *testing.T) {
	path := writeTemp(t, "a\r\n\tb\r\n")
	r := runWith([]string{path}, "a\n  b\n====\nc\n====\n")
	if r.code != exitMismatch {
		t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": old block found 0 times, expected 1\n" +
		"  hint: file line 2 starts with 1 tab, old block line 2 with 2 spaces (near line 1)\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

func TestRun_mixedLineEndingsAreMatchedAsIs(t *testing.T) {
	original := "a\r\nb\nc\r\n"
	path := writeTemp(t, original)
	r := runWith([]string{path}, "a\nb\nc\n====\nx\n====\n")
	if r.code != exitMismatch {
		t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": old block found 0 times, expected 1\n" +
		"  hint: file line 1 ends with CRLF, old block line 1 with LF (near line 1)\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
	if got := readBack(t, path); got != original {
		t.Errorf("file changed to %q", got)
	}
}

func TestRun_fileWithoutLineBreakStaysLF(t *testing.T) {
	path := writeTemp(t, "abc")
	r := runWith([]string{path}, "b\n====\nx\ny\n====\n")
	if r.code != exitOK || r.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": replaced at line 1 (1 match starts inside an identifier; 1 match ends inside an identifier; 1 match starts mid-line with a multi-line new block)\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got := readBack(t, path); got != "ax\nyc" {
		t.Errorf("file = %q, want %q", got, "ax\nyc")
	}
}

func TestRun_blocksWithCRAreTakenLiterally(t *testing.T) {
	original := "a\r\nb\r\n"
	path := writeTemp(t, original)
	r := runWith([]string{path}, "a\r\nb\n====\nA\r\nB\n====\n")
	if r.code != exitOK || r.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": replaced at line 1\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got := readBack(t, path); got != "A\r\nB\r\n" {
		t.Errorf("file = %q", got)
	}
}

func TestRun_countMatchesReplacesAll(t *testing.T) {
	path := writeTemp(t, "x(1)\ny\nx(2)\nx(3)\n")
	r := runWith([]string{"-n", "3", path}, "x(\n====\nz(\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": replaced at lines 1, 3, 4\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "z(1)\ny\nz(2)\nz(3)\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_customSeparator(t *testing.T) {
	path := writeTemp(t, "Title\n====\nbody\n")
	r := runWith([]string{"-d", "%%", path}, "Title\n====\n%%\nTitle\n----\n%%\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "Title\n----\nbody\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_badStdinIsUsageError(t *testing.T) {
	original := "x\n"
	path := writeTemp(t, original)
	r := runWith([]string{path}, "x\n====\ny\n====\nz\n====\n")
	if r.code != exitUsage {
		t.Fatalf("exit = %d, want 2", r.code)
	}
	if !strings.Contains(r.stderr, "found 3 \"====\" lines, expected 2") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if got := readBack(t, path); got != original {
		t.Errorf("file changed to %q", got)
	}
}

func TestRun_usageError(t *testing.T) {
	r := runWith([]string{"--bogus"}, "")
	if r.code != exitUsage {
		t.Errorf("exit = %d, want 2", r.code)
	}
	if r.stdout != "" || !strings.Contains(r.stderr, "unknown flag") {
		t.Errorf("stdout = %q, stderr = %q", r.stdout, r.stderr)
	}
}

func TestRun_missingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.txt")
	r := runWith([]string{path}, "x\n====\ny\n====\n")
	if r.code != exitFile {
		t.Errorf("exit = %d, want 3", r.code)
	}
	// The path is the one that was passed, with no "open" in front of it.
	if want := "sub1: " + path + ": no such file or directory\n"; r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

// A file that cannot be read and one that cannot be written fail with the
// same words from the OS, so each names its own operation.
func TestRun_unreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	r := runWith([]string{path}, "x\n====\ny\n====\n")
	if r.code != exitFile {
		t.Errorf("exit = %d, want 3 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": cannot read the file: permission denied\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

func TestRun_unwritableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	r := runWith([]string{path}, "x\n====\ny\n====\n")
	if r.code != exitFile {
		t.Errorf("exit = %d, want 3 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": cannot write the file: permission denied; make it writable, or edit a copy\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

// A file error names the file even when the thing that failed is the
// directory around it, so that every exit 3 line reads the same way.
func TestRun_unwritableDirNamesTheFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	r := runWith([]string{path}, "x\n====\ny\n====\n")
	if r.code != exitFile {
		t.Errorf("exit = %d, want 3 (stderr: %q)", r.code, r.stderr)
	}
	want := "sub1: " + path + ": cannot create a temporary file in " + dir + ": permission denied; make the directory writable\n"
	if r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

func TestIsTerminal(t *testing.T) {
	if isTerminal(strings.NewReader("")) {
		t.Error("a strings.Reader is not a terminal")
	}
	f, err := os.Open(writeTemp(t, "x\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("a regular file is not a terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if isTerminal(null) {
		t.Error("the null device is not a terminal")
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Skip("no controlling terminal")
	}
	defer tty.Close()
	if !isTerminal(tty) {
		t.Error("/dev/tty should be a terminal")
	}
}

func TestRun_help(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		r := runWith([]string{flag}, "")
		if r.code != exitOK || r.stderr != "" {
			t.Errorf("%s: exit = %d, stderr = %q", flag, r.code, r.stderr)
		}
		if !strings.HasPrefix(r.stdout, "sub1 — ") || !strings.Contains(r.stdout, "Usage:") {
			t.Errorf("%s: stdout = %q", flag, r.stdout)
		}
		if !strings.Contains(r.stdout, "Examples:") {
			t.Errorf("%s: help has no examples", flag)
		}
		if n := strings.Count(r.stdout, "\n"); n > 60 {
			t.Errorf("%s: help is %d lines, want at most 60", flag, n)
		}
	}
}

func TestRun_instructions(t *testing.T) {
	r := runWith([]string{"--instructions"}, "")
	if r.code != exitOK || r.stderr != "" {
		t.Errorf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if r.stdout != instructionsText {
		t.Errorf("stdout = %q", r.stdout)
	}
	if !strings.HasSuffix(r.stdout, "\n") {
		t.Error("instructions must end in a newline")
	}
	// The paragraph is pasted into an instruction file, so it carries the
	// call it describes as an indented block and nothing else.
	if !strings.Contains(r.stdout, "\n    sub1 FILE <<'SUB1'\n") {
		t.Errorf("instructions have no example call: %q", r.stdout)
	}
	if strings.Contains(r.stdout, "exit 1") || strings.Contains(r.stdout, "-n N") {
		t.Errorf("instructions repeat what the output already says: %q", r.stdout)
	}
}

func TestRun_version(t *testing.T) {
	for _, flag := range []string{"-v", "--version"} {
		r := runWith([]string{flag}, "")
		if r.code != exitOK || r.stderr != "" {
			t.Errorf("%s: exit = %d, stderr = %q", flag, r.code, r.stderr)
		}
		if strings.TrimSpace(r.stdout) == "" {
			t.Errorf("%s: version output empty", flag)
		}
	}
}

func TestRun_pairsReplaceEveryPairInOneWrite(t *testing.T) {
	path := writeTemp(t, "a\nb\nc\nd\n")
	r := runWith([]string{"--pairs", path}, "a\n====\nA\n====\nc\n====\nC1\nC2\n====\n")
	if r.code != exitOK || r.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": pair 1: replaced at line 1\n" + path + ": pair 2: replaced at line 3\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "A\nb\nC1\nC2\nd\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_pairsReportLinesInTheFileAsGiven(t *testing.T) {
	// Pair 1 adds lines above pair 2's match, and pair 2's new block holds
	// pair 1's old block: both are judged against the file as it was.
	path := writeTemp(t, "x\ny\n")
	r := runWith([]string{"--pairs", path}, "x\n====\nx1\nx2\nx3\n====\ny\n====\nx\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": pair 1: replaced at line 1\n" + path + ": pair 2: replaced at line 2\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "x1\nx2\nx3\nx\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_pairsDeleteWholeLines(t *testing.T) {
	path := writeTemp(t, "a\nb\nc\n")
	r := runWith([]string{"--pairs", path}, "a\n====\n====\nb\n====\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "c\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_pairsInCRLFFile(t *testing.T) {
	path := writeTemp(t, "a\r\nb\r\nc\r\n")
	r := runWith([]string{"--pairs", path}, "a\n====\nA\n====\nb\nc\n====\nB\n====\n")
	if r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": pair 1: replaced at line 1 (CRLF)\n" + path + ": pair 2: replaced at line 2 (CRLF)\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "A\r\nB\r\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_pairsReportEveryFailingPairAndWriteNothing(t *testing.T) {
	content := "keep\nfoo\nfoo\nbar\n"
	path := writeTemp(t, content)
	r := runWith([]string{"--pairs", path}, "keep\n====\nKEEP\n====\nfoo\n====\nFOO\n====\nmissing\n====\nM\n====\n")
	if r.code != exitMismatch || r.stdout != "" {
		t.Fatalf("exit = %d, stdout = %q", r.code, r.stdout)
	}
	for _, want := range []string{
		"sub1: " + path + ": pair 2: old block found 2 times (lines 2, 3), expected 1; widen the old block to match only 1, or replace all 2 in a call of its own with -n 2\n",
		"sub1: " + path + ": pair 3: old block found 0 times, expected 1",
	} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr = %q, want containing %q", r.stderr, want)
		}
	}
	if strings.Contains(r.stderr, "pair 1") || strings.Contains(r.stderr, "-n 2 to replace") {
		t.Errorf("stderr = %q", r.stderr)
	}
	if got := readBack(t, path); got != content {
		t.Errorf("file = %q, want it untouched", got)
	}
}

func TestRun_pairsThatOverlapWriteNothing(t *testing.T) {
	content := "one\ntwo three\nfour\n"
	path := writeTemp(t, content)
	r := runWith([]string{"--pairs", path}, "three\nfour\n====\nx\n====\ntwo three\n====\ny\n====\n")
	want := "sub1: " + path + ": pair 2 overlaps pair 1 at line 2; shorten one of the two old blocks so they do not share text\n"
	if r.code != exitMismatch || r.stderr != want {
		t.Fatalf("exit = %d, stderr = %q, want %q", r.code, r.stderr, want)
	}
	if got := readBack(t, path); got != content {
		t.Errorf("file = %q, want it untouched", got)
	}
}

func TestRun_pairsInputErrorsNameNoFile(t *testing.T) {
	path := writeTemp(t, "a\nb\n")
	for _, tt := range []struct {
		argv        []string
		stdin, want string
	}{
		{[]string{"--pairs", path}, "a\n====\nA\n====\na\n====\nB\n====\n", "sub1: pair 1 and pair 2 have the same old block; remove one of them\n"},
		{[]string{"--pairs", path}, "a\n====\nA\n====\nb\n====\n", "sub1: found 3 \"====\" lines, expected an even number with --pairs, two for each pair; if a content line equals \"====\", pass -d '%%%%' and write %%%% on every separator line\n"},
		{[]string{"--pairs", "-n", "2", path}, "a\n====\nA\n====\n", "sub1: -n cannot be used with --pairs; replace a block that occurs more than once in a call of its own with -n\n"},
	} {
		r := runWith(tt.argv, tt.stdin)
		if r.code != exitUsage || r.stderr != tt.want {
			t.Errorf("%q: exit = %d, stderr = %q, want %q", tt.stdin, r.code, r.stderr, tt.want)
		}
	}
	if got := readBack(t, path); got != "a\nb\n" {
		t.Errorf("file = %q, want it untouched", got)
	}
}

func TestRun_withoutPairsSeveralPairsAreStillRefused(t *testing.T) {
	path := writeTemp(t, "a\nb\n")
	r := runWith([]string{path}, "a\n====\nA\n====\nb\n====\nB\n====\n")
	if r.code != exitUsage || !strings.Contains(r.stderr, `found 4 "====" lines, expected 2`) {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
}

func TestRun_rangeReplacesFromStartThroughEnd(t *testing.T) {
	path := writeTemp(t, "# T\n\n## Old\na\nb\n\n## Next\nc\n")
	r := runWith([]string{"--range", path}, "## Old\n====\n## Next\n====\n## New\nx\n\n## Next\n====\n")
	if r.code != exitOK || r.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if want := path + ": replaced lines 3-7 (5 lines)\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if got, want := readBack(t, path), "# T\n\n## New\nx\n\n## Next\nc\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_rangeWithEmptyNewDeletesTheLines(t *testing.T) {
	path := writeTemp(t, "keep\nfrom\nmid\nto\nkeep too\n")
	r := runWith([]string{"--range", path}, "from\n====\nto\n====\n====\n")
	if r.code != exitOK || r.stdout != path+": replaced lines 2-4 (3 lines)\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", r.code, r.stdout, r.stderr)
	}
	if got, want := readBack(t, path), "keep\nkeep too\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_rangeInCRLFFile(t *testing.T) {
	path := writeTemp(t, "a\r\nb\r\nc\r\nd\r\n")
	r := runWith([]string{"--range", path}, "b\n====\nc\n====\nB\n====\n")
	if r.code != exitOK || r.stdout != path+": replaced lines 2-3 (2 lines; CRLF)\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", r.code, r.stdout, r.stderr)
	}
	if got, want := readBack(t, path), "a\r\nB\r\nd\r\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_rangeMismatchesWriteNothing(t *testing.T) {
	content := "x\nfrom\ny\nfrom\nend\nlast\n"
	for _, tt := range []struct{ stdin, want string }{
		{"from\n====\nend\n====\nz\n====\n", ": start block found 2 times (lines 2, 4), expected 1; widen the start block to match only 1\n"},
		{"missing\n====\nend\n====\nz\n====\n", ": start block found 0 times, expected 1; no similar text found, read the file again\n"},
		{"last\n====\nx\n====\nz\n====\n", ": end block found 0 times after the start block (line 6); it occurs before the start block, at line 1\n"},
		{"y\n====\nmissing\n====\nz\n====\n", ": end block found 0 times after the start block (line 3); no similar text found, read the file again\n"},
	} {
		path := writeTemp(t, content)
		r := runWith([]string{"--range", path}, tt.stdin)
		if r.code != exitMismatch || r.stdout != "" || !strings.HasPrefix(r.stderr, "sub1: "+path+tt.want) {
			t.Errorf("%q: exit = %d, stdout = %q, stderr = %q, want %q", tt.stdin, r.code, r.stdout, r.stderr, tt.want)
		}
		if got := readBack(t, path); got != content {
			t.Errorf("%q: file = %q, want it untouched", tt.stdin, got)
		}
	}
}

func TestRun_rangeInputErrorsNameNoFile(t *testing.T) {
	path := writeTemp(t, "a\nb\n")
	r := runWith([]string{"--range", path}, "a\n====\nb\n====\n")
	want := "sub1: found 2 \"====\" lines, expected 3 with --range: one after the start block, one after the end block, and one after the new block, even an empty one\n"
	if r.code != exitUsage || r.stderr != want {
		t.Fatalf("exit = %d, stderr = %q, want %q", r.code, r.stderr, want)
	}
}

func TestRun_rangeHintsNameTheBlockAndLookAfterTheStart(t *testing.T) {
	// "from x" on line 1 is what the mistyped end blocks come closest to,
	// but it is above the start block, so only the line after it is named.
	content := "from x\nstart\nmid\nfrom y\n"
	for _, tt := range []struct{ stdin, want string }{
		{"  start\n====\nmid\n====\nz\n====\n", "  hint: file line 2 starts with no indentation, start block line 1 with 2 spaces (near line 2)\n"},
		{"start\n====\nfrom  y\n====\nz\n====\n", "  hint: file line 4 has 1 space where the end block has 2 spaces (near line 4)\n"},
		{"start\n====\nfrom  x\n====\nz\n====\n", "(line 2); no similar text found, read the file again\n"},
	} {
		path := writeTemp(t, content)
		r := runWith([]string{"--range", path}, tt.stdin)
		if r.code != exitMismatch || !strings.HasSuffix(r.stderr, tt.want) {
			t.Errorf("%q: exit = %d, stderr = %q, want ending with %q", tt.stdin, r.code, r.stderr, tt.want)
		}
	}
}
