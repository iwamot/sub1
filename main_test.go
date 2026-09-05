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
		{"count before file", []string{"-n", "3", "f.txt"}, with(func(a *cliArgs) { a.expected = 3; a.path = "f.txt" }), ""},
		{"count after file", []string{"f.txt", "-n", "3"}, with(func(a *cliArgs) { a.expected = 3; a.path = "f.txt" }), ""},
		{"separator", []string{"-d", "%%", "f.txt"}, with(func(a *cliArgs) { a.separator = "%%"; a.path = "f.txt" }), ""},
		{"both flags", []string{"f.txt", "-d", "%%", "-n", "2"}, with(func(a *cliArgs) { a.separator = "%%"; a.expected = 2; a.path = "f.txt" }), ""},
		{"help short", []string{"-h"}, with(func(a *cliArgs) { a.showHelp = true }), ""},
		{"help long", []string{"--help"}, with(func(a *cliArgs) { a.showHelp = true }), ""},
		{"version short", []string{"-v"}, with(func(a *cliArgs) { a.showVersion = true }), ""},
		{"version long", []string{"--version"}, with(func(a *cliArgs) { a.showVersion = true }), ""},
		{"no file", nil, cliArgs{}, "no file given"},
		{"unknown flag", []string{"--bogus", "f.txt"}, cliArgs{}, "unknown flag"},
		{"two files", []string{"a", "b"}, cliArgs{}, "multiple files"},
		{"count missing value", []string{"f.txt", "-n"}, cliArgs{}, "-n needs a value"},
		{"count zero", []string{"-n", "0", "f.txt"}, cliArgs{}, "positive integer"},
		{"count negative", []string{"-n", "-1", "f.txt"}, cliArgs{}, "positive integer"},
		{"count not a number", []string{"-n", "x", "f.txt"}, cliArgs{}, "positive integer"},
		{"separator missing value", []string{"f.txt", "-d"}, cliArgs{}, "-d needs a value"},
		{"separator empty", []string{"-d", "", "f.txt"}, cliArgs{}, "non-empty"},
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
		{"dev falls back to build info Main.Version", devVersion, &debug.BuildInfo{Main: debug.Module{Version: "v0.0.3"}}, "v0.0.3"},
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
	r := runWith([]string{path}, "  foo $x `y` \\n \"q\"\n  bar\n====\n  FOO\n  BAR\n  BAZ\n")
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
	if r := runWith([]string{path}, "x\n====\ny\n"); r.code != exitOK {
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

func TestRun_deletesWhenNewIsEmpty(t *testing.T) {
	path := writeTemp(t, "a\nb\nc\n")
	if r := runWith([]string{path}, "b\n\n====\n"); r.code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", r.code, r.stderr)
	}
	if got, want := readBack(t, path), "a\nc\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestRun_noTrailingNewlineInFile(t *testing.T) {
	path := writeTemp(t, "abc")
	if r := runWith([]string{path}, "b\n====\nB\n"); r.code != exitOK {
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
		{"absent", nil, "old block found 0 times, expected 1"},
		{"duplicate", nil, "old block found 2 times, expected 1"},
		{"fewer than -n", []string{"-n", "3"}, "old block found 2 times, expected 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := "x\nx\n"
			old := "x"
			if tt.name == "absent" {
				old = "nope"
			}
			path := writeTemp(t, original)
			r := runWith(append([]string{path}, tt.argv...), old+"\n====\ny\n")
			if r.code != exitMismatch {
				t.Fatalf("exit = %d, want 1 (stderr: %q)", r.code, r.stderr)
			}
			if r.stdout != "" {
				t.Errorf("stdout = %q, want empty", r.stdout)
			}
			if !strings.Contains(r.stderr, tt.wantLine) {
				t.Errorf("stderr = %q, want containing %q", r.stderr, tt.wantLine)
			}
			if got := readBack(t, path); got != original {
				t.Errorf("file changed to %q", got)
			}
		})
	}
}

func TestRun_countMatchesReplacesAll(t *testing.T) {
	path := writeTemp(t, "x(1)\ny\nx(2)\nx(3)\n")
	r := runWith([]string{"-n", "3", path}, "x(\n====\nz(\n")
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
	r := runWith([]string{"-d", "%%", path}, "Title\n====\n%%\nTitle\n----\n")
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
	r := runWith([]string{path}, "x\n====\ny\n====\nz\n")
	if r.code != exitUsage {
		t.Fatalf("exit = %d, want 2", r.code)
	}
	if !strings.Contains(r.stderr, "found 2") {
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
	r := runWith([]string{path}, "x\n====\ny\n")
	if r.code != exitUsage {
		t.Errorf("exit = %d, want 2", r.code)
	}
	if r.stderr == "" {
		t.Error("expected an error on stderr")
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
	r := runWith([]string{path}, "x\n====\ny\n")
	if r.code != exitUsage {
		t.Errorf("exit = %d, want 2 (stderr: %q)", r.code, r.stderr)
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
		if n := strings.Count(r.stdout, "\n"); n > 20 {
			t.Errorf("%s: help is %d lines, want at most 20", flag, n)
		}
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
