package mcp

import (
	"os"
	"path/filepath"
)

// Config locates the on-disk sources the server reads. All fields are
// absolute or repo-relative paths; empty strings are filled by
// WithDefaults.
type Config struct {
	Root           string
	DocsDir        string
	KitDir         string
	PlaygroundsDir string
}

// WithDefaults fills missing fields by inferring them from Root, then
// from the working directory walk if Root is empty. The returned Config
// always has Root populated when a sveltego repo can be located.
func (c Config) WithDefaults() Config {
	if c.Root == "" {
		c.Root = findRepoRoot()
	}
	if c.Root != "" {
		if c.DocsDir == "" {
			c.DocsDir = filepath.Join(c.Root, "documentation", "docs")
		}
		if c.KitDir == "" {
			c.KitDir = filepath.Join(c.Root, "packages", "sveltego", "exports", "kit")
		}
		if c.PlaygroundsDir == "" {
			c.PlaygroundsDir = filepath.Join(c.Root, "playgrounds")
		}
	}
	return c
}

func findRepoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for i := 0; i < 12; i++ {
		if hasRepoMarkers(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

func hasRepoMarkers(dir string) bool {
	_, err1 := os.Stat(filepath.Join(dir, "go.work"))
	_, err2 := os.Stat(filepath.Join(dir, "packages", "sveltego"))
	return err1 == nil && err2 == nil
}
