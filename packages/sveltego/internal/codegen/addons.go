package codegen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/vite"
)

// detectAddons reads <projectRoot>/package.json and returns the list of
// vite addons inferred from devDependencies. Missing package.json is not
// an error; it returns the zero set.
//
// Detection rules:
//   - "@tailwindcss/vite" in dependencies/devDependencies → AddonTailwindV4
//   - else "tailwindcss" present → AddonTailwindV3
func detectAddons(projectRoot string) ([]vite.Addon, error) {
	path := filepath.Join(projectRoot, "package.json")
	data, err := os.ReadFile(path) //nolint:gosec // path is projectRoot-rooted
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("codegen: read package.json: %w", err)
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("codegen: parse package.json: %w", err)
	}
	has := func(name string) bool {
		if _, ok := pkg.Dependencies[name]; ok {
			return true
		}
		_, ok := pkg.DevDependencies[name]
		return ok
	}
	switch {
	case has("@tailwindcss/vite"):
		return []vite.Addon{vite.AddonTailwindV4}, nil
	case has("tailwindcss"):
		return []vite.Addon{vite.AddonTailwindV3}, nil
	}
	return nil, nil
}

// resolveCSSEntry returns the path (relative to projectRoot) of the CSS
// entry to feed Vite when an addon needs one. Currently it looks for
// src/app.css. Returns "" when not found.
func resolveCSSEntry(projectRoot string) string {
	rel := filepath.Join("src", "app.css")
	if _, err := os.Stat(filepath.Join(projectRoot, rel)); err == nil {
		return filepath.ToSlash(rel)
	}
	return ""
}
