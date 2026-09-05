// Package e2e exercises the compiled sub1 binary as a subprocess. Unit tests
// in the main package call run() directly; these tests build the actual
// artifact and verify exit codes, stdin handling, stdout/stderr routing, and
// on-disk effects through a real os.Exec boundary.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "sub1-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "TestMain: mkdir:", err)
		os.Exit(2)
	}
	binPath = filepath.Join(tmp, "sub1")
	out, buildErr := exec.Command("go", "build", "-o", binPath, "..").CombinedOutput()
	if buildErr != nil {
		fmt.Fprintf(os.Stderr, "TestMain: go build failed: %v\n%s", buildErr, out)
		os.RemoveAll(tmp)
		os.Exit(2)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

func runBin(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return runCmd(t, cmd)
}

func runCmd(t *testing.T, cmd *exec.Cmd) result {
	t.Helper()
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	if err == nil {
		return result{stdout: so.String(), stderr: se.String(), exitCode: 0}
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return result{stdout: so.String(), stderr: se.String(), exitCode: ee.ExitCode()}
	}
	t.Fatalf("run %v: %v", cmd.Args, err)
	return result{}
}

func tempFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestE2E_help(t *testing.T) {
	r := runBin(t, "", "--help")
	if r.exitCode != 0 || r.stderr != "" {
		t.Errorf("exit = %d, stderr = %q", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "Usage:") {
		t.Errorf("stdout missing Usage:\n%s", r.stdout)
	}
}

func TestE2E_version(t *testing.T) {
	r := runBin(t, "", "--version")
	if r.exitCode != 0 || r.stderr != "" {
		t.Errorf("exit = %d, stderr = %q", r.exitCode, r.stderr)
	}
	if strings.TrimSpace(r.stdout) == "" {
		t.Error("version output empty")
	}
}

func TestE2E_replace(t *testing.T) {
	path := tempFile(t, "alpha\nbeta\ngamma\n")
	r := runBin(t, "beta\n====\nBETA\n", path)
	if r.exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", r.exitCode, r.stderr)
	}
	if want := path + ": replaced at line 2\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "alpha\nBETA\ngamma\n" {
		t.Errorf("file = %q", got)
	}
}

func TestE2E_mismatchLeavesFileUntouched(t *testing.T) {
	path := tempFile(t, "x\nx\n")
	r := runBin(t, "x\n====\ny\n", path)
	if r.exitCode != 1 {
		t.Fatalf("exit = %d, want 1 (stderr: %q)", r.exitCode, r.stderr)
	}
	if r.stdout != "" || !strings.Contains(r.stderr, "found 2 times, expected 1") {
		t.Errorf("stdout = %q, stderr = %q", r.stdout, r.stderr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x\nx\n" {
		t.Errorf("file changed to %q", got)
	}
}

func TestE2E_countFlag(t *testing.T) {
	path := tempFile(t, "x\nx\n")
	r := runBin(t, "x\n====\ny\n", "-n", "2", path)
	if r.exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q", r.exitCode, r.stderr)
	}
	if want := path + ": replaced at lines 1, 2\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestE2E_usageError(t *testing.T) {
	r := runBin(t, "", "--bogus")
	if r.exitCode != 2 || r.stdout != "" || r.stderr == "" {
		t.Errorf("exit = %d, stdout = %q, stderr = %q", r.exitCode, r.stdout, r.stderr)
	}
}

// stdin on the null device, or closed and reopened there by the Go runtime,
// is what sandboxes and CI hand to a process. It is not a terminal, so the
// empty input reaches the block parser and is reported as such.
func TestE2E_emptyStdinIsNotATerminal(t *testing.T) {
	path := tempFile(t, "x\n")
	devNull := exec.Command(binPath, path)
	closed := exec.Command("bash", "-c", `exec "$0" "$1" <&-`, binPath, path)
	for name, cmd := range map[string]*exec.Cmd{"null device": devNull, "closed": closed} {
		t.Run(name, func(t *testing.T) {
			r := runCmd(t, cmd)
			if r.exitCode != 2 || r.stdout != "" {
				t.Errorf("exit = %d, stdout = %q", r.exitCode, r.stdout)
			}
			if !strings.Contains(r.stderr, "found 0") || strings.Contains(r.stderr, "terminal") {
				t.Errorf("stderr = %q, want the separator count, not the terminal message", r.stderr)
			}
		})
	}
}

// The README quotes --help verbatim; keep the two from drifting apart.
func TestE2E_readmeQuotesHelp(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	const marker = "```\n$ sub1 --help\n"
	start := strings.Index(string(readme), marker)
	if start < 0 {
		t.Fatalf("README.md has no %q block", marker)
	}
	rest := string(readme)[start+len(marker):]
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Fatal("README.md help block is not closed")
	}
	quoted := rest[:end]
	if got := runBin(t, "", "--help").stdout; got != quoted {
		t.Errorf("README help block differs from --help output\n--- README ---\n%s\n--- --help ---\n%s", quoted, got)
	}
}
