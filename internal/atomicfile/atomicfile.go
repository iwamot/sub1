// Package atomicfile replaces a file's contents without leaving a partially
// written file behind if the write is interrupted.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// modeBits is what os.Chmod can carry over from the original file: the
// permission bits plus setuid, setgid, and sticky.
const modeBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

// WriteFile writes data to path by way of a temporary file in the same
// directory, then renames it into place. Either the old contents or the new
// contents are visible at path; never a mix. The file's mode is preserved.
// If path is a symlink, the target is replaced, not the link. A file the
// caller cannot open for writing is left untouched.
func WriteFile(path string, data []byte) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(real)
	if err != nil {
		return err
	}
	// Rename only needs write permission on the directory, so a read-only
	// file would otherwise be replaced without complaint. Ask the OS for
	// write access to the file itself first, so a file the caller cannot
	// write stays as it is.
	f, err := os.OpenFile(real, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	f.Close()
	dir := filepath.Dir(real)

	tmp, err := os.CreateTemp(dir, ".sub1-*")
	if err != nil {
		// The temporary file's name means nothing to the caller; point at
		// the directory, which is what needs write permission.
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return fmt.Errorf("%s: cannot create a temporary file in %s: %w", path, dir, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Flush to disk before the rename so a crash after the rename cannot
	// leave an empty file at path. Killing the process alone does not lose
	// buffered data; this guards against power loss and OS crashes.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, info.Mode()&modeBits); err != nil {
		return err
	}
	if err := os.Rename(tmpName, real); err != nil {
		return err
	}
	committed = true

	// Persist the rename itself. Best effort: some filesystems reject
	// fsync on a directory, and the rename has already happened.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}
