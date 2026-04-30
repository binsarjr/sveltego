// Package fsutil holds tiny filesystem helpers used by adapter-lambda.
// It is internal by design — the helpers are not part of the adapter's
// public API and may change without notice.
package fsutil

import (
	"errors"
	"io"
	"os"
)

// WriteFile writes content to path with the given permission bits,
// truncating any existing file. The error returned from closing the
// destination is joined with any write error so a flush failure on
// close is never silently dropped (the bug behind issue #192).
func WriteFile(path, content string, perm os.FileMode) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm) //nolint:gosec // path is OutputDir/<known name>
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	_, err = io.WriteString(f, content)
	return err
}
