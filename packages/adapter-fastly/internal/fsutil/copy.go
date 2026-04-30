// Package fsutil holds tiny filesystem helpers used by adapter-fastly.
// It is internal by design — the helpers are not part of the adapter's
// public API and may change without notice.
package fsutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// CopyFile copies src to dst with the given permission bits, creating
// any missing parent directories. Errors from closing the source and
// destination files are joined so a flush failure on close is never
// silently dropped.
func CopyFile(src, dst string, perm os.FileMode) (err error) {
	if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
		return mkErr
	}

	in, err := os.Open(src) //nolint:gosec // src is supplied by the adapter, not user input
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, in.Close())
	}()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec // dst is under the adapter's OutputDir
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, out.Close())
	}()

	_, err = io.Copy(out, in)
	return err
}
