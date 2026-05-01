// CLI-level codegen + go-build integration test. Default `go test` runs
// the compile half (cheap); the `integration` build tag gates the full
// `sveltego build` invocation that subprocesses `go build`. Run with
// `go test -tags=integration -run TestBuildCmdIntegration ./cmd/sveltego/...`.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyTree mirrors src into dst, replacing the literal __SVELTEGO__ token
// in any .template file with replacement and renaming the file to drop
// the .template suffix.
func copyTree(t *testing.T, src, dst, replacement string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(target, ".template") {
			raw = []byte(strings.ReplaceAll(string(raw), "__SVELTEGO__", replacement))
			target = strings.TrimSuffix(target, ".template")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

// stageExample copies cmd/sveltego/testdata/example into a fresh temp
// directory and rewrites go.mod with a replace directive pointing at the
// real sveltego module path so isolated-mode builds can resolve imports.
func stageExample(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	src := filepath.Join(wd, "testdata", "example")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("fixture missing at %s: %v", src, err)
	}
	sveltego, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatalf("abs sveltego module: %v", err)
	}

	dst := t.TempDir()
	copyTree(t, src, dst, sveltego)
	return dst
}

// withCwd swaps the process working directory for the duration of the
// test. The caller's cleanup restores the original CWD.
func withCwd(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}

func TestCompileCmd_FixtureProject(t *testing.T) {
	resetLoggerOnCleanup(t)
	root := stageExample(t)
	withCwd(t, root)

	stdout, stderr, err := runCmd(t, "compile")
	if err != nil {
		t.Fatalf("compile: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	for _, p := range []string{
		filepath.Join(root, ".gen", "routes", "page.gen.go"),
		filepath.Join(root, ".gen", "manifest.gen.go"),
		filepath.Join(root, ".gen", "links", "links.go"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist after compile: %v", p, err)
		}
	}
	if !strings.Contains(stdout, "compiled:") {
		t.Errorf("expected compile success line, got %q", stdout)
	}
}

func TestBuildCmd_NoGoMod(t *testing.T) {
	resetLoggerOnCleanup(t)
	dir := t.TempDir()
	withCwd(t, dir)
	_, _, err := runCmd(t, "compile")
	if err == nil {
		t.Fatal("expected error when no go.mod exists in cwd ancestry")
	}
}

// TestHasRequire pins the go.mod parsing used by ensureSveltegoRequire
// to decide whether `go get @latest` needs to run on first build. The
// rule matters because the fresh-scaffold (#110) deliberately omits the
// require line and we must not run `go get` on already-tidy projects.
func TestHasRequire(t *testing.T) {
	t.Parallel()
	const target = "github.com/binsarjr/sveltego/packages/sveltego"

	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "bare module clause (fresh scaffold)",
			body: "module example.com/hello\n\ngo 1.23\n",
			want: false,
		},
		{
			name: "single-line require",
			body: "module example.com/hello\n\ngo 1.23\n\nrequire " + target + " v0.0.0-bootstrap.0.20260501100517-d57b1b5d2445\n",
			want: true,
		},
		{
			name: "block-form require",
			body: "module example.com/hello\n\ngo 1.23\n\nrequire (\n\t" + target + " v0.0.0-bootstrap.0.20260501100517-d57b1b5d2445\n)\n",
			want: true,
		},
		{
			name: "different module required",
			body: "module example.com/hello\n\ngo 1.23\n\nrequire github.com/spf13/cobra v1.0.0\n",
			want: false,
		},
		{
			name: "comment line referencing module",
			body: "module example.com/hello\n\ngo 1.23\n\n// " + target + " is the framework\n",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasRequire([]byte(tc.body), target)
			if got != tc.want {
				t.Errorf("hasRequire(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
