package aitemplates

import (
	"io/fs"
	"path"
)

// stripPrefixFS exposes inner under the empty root by transparently
// prepending "files/" to every requested name. It implements fs.FS so
// fs.ReadFile(FS, "AGENTS.md") resolves to inner.ReadFile("files/AGENTS.md").
type stripPrefixFS struct {
	inner fs.FS
}

// Open implements fs.FS, mapping the empty root to "files".
func (s stripPrefixFS) Open(name string) (fs.File, error) {
	if name == "." {
		return s.inner.Open("files")
	}
	return s.inner.Open(path.Join("files", name))
}

// ReadFile lets callers use fs.ReadFile(FS, name) without going through
// the fs.File round trip. fs.ReadFile prefers an interface match here.
func (s stripPrefixFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(s.inner, path.Join("files", name))
}
