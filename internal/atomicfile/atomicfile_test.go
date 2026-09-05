package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

// assertNoTempFiles fails if a temporary file was left behind in dir.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(dir, ".sub1-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestWriteFileKeepsMode(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o755, 0o755 | os.ModeSetgid} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "f.txt")
			if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}

			if err := WriteFile(path, []byte("new\n")); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "new\n" {
				t.Errorf("content = %q, want %q", got, "new\n")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&modeBits != mode {
				t.Errorf("mode = %v, want %v", info.Mode()&modeBits, mode)
			}
			assertNoTempFiles(t, dir)
		})
	}
}

func TestWriteFileFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(link, []byte("new\n")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("target content = %q, want %q", got, "new\n")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link was replaced by a regular file")
	}
	assertNoTempFiles(t, dir)
}

func TestWriteFileUnwritableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("directory permissions do not apply to root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	err := WriteFile(path, []byte("new\n"))
	if err == nil {
		t.Fatal("WriteFile succeeded in an unwritable directory")
	}
	want := path + ": cannot create a temporary file in " + dir + ": permission denied"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Errorf("content = %q, want the original %q", got, "old\n")
	}
	assertNoTempFiles(t, dir)
}

func TestWriteFileReadOnlyFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("file permissions do not apply to root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o400); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, []byte("new\n")); err == nil {
		t.Fatal("WriteFile replaced a read-only file")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Errorf("content = %q, want the original %q", got, "old\n")
	}
	assertNoTempFiles(t, dir)
}
