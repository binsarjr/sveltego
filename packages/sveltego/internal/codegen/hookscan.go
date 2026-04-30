package codegen

import (
	"errors"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

// HookSet records which optional hook functions a user-authored
// hooks.server.go declares. The build pipeline consults this to emit a
// hooks adapter that fills missing entries with identity defaults.
type HookSet struct {
	// SourcePath is the absolute path to src/hooks.server.go. Empty when
	// no user file exists.
	SourcePath  string
	Handle      bool
	HandleError bool
	HandleFetch bool
	Reroute     bool
	Init        bool
}

// Present reports whether SourcePath points at a user-authored file.
func (h HookSet) Present() bool { return h.SourcePath != "" }

// Any reports whether any optional hook function was discovered. Used
// by the emitter to skip mirror generation entirely when the file
// exists but declares nothing exported.
func (h HookSet) Any() bool {
	return h.Handle || h.HandleError || h.HandleFetch || h.Reroute || h.Init
}

// scanHooksServer reads <projectRoot>/src/hooks.server.go (when present)
// and reports which optional hook functions the file declares. Missing
// file is not an error; signature mismatches are surfaced via err so
// the build fails fast instead of emitting a non-compiling adapter.
func scanHooksServer(projectRoot string) (HookSet, error) {
	path := filepath.Join(projectRoot, "src", "hooks.server.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return HookSet{}, nil
		}
		return HookSet{}, fmt.Errorf("codegen: stat %s: %w", path, err)
	}
	if info.IsDir() {
		return HookSet{}, fmt.Errorf("codegen: %s is a directory", path)
	}

	src, err := os.ReadFile(path) //nolint:gosec // path is project-rooted
	if err != nil {
		return HookSet{}, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	stripped := stripBuildConstraint(src)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, stripped, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return HookSet{}, fmt.Errorf("codegen: parse %s: %w", path, err)
	}

	out := HookSet{SourcePath: path}
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil {
			continue
		}
		switch fn.Name.Name {
		case "Handle":
			if err := validateHandleSig(fn); err != nil {
				return HookSet{}, fmt.Errorf("codegen: %s: %w", path, err)
			}
			out.Handle = true
		case "HandleError":
			if err := validateHandleErrorSig(fn); err != nil {
				return HookSet{}, fmt.Errorf("codegen: %s: %w", path, err)
			}
			out.HandleError = true
		case "HandleFetch":
			if err := validateHandleFetchSig(fn); err != nil {
				return HookSet{}, fmt.Errorf("codegen: %s: %w", path, err)
			}
			out.HandleFetch = true
		case "Reroute":
			if err := validateRerouteSig(fn); err != nil {
				return HookSet{}, fmt.Errorf("codegen: %s: %w", path, err)
			}
			out.Reroute = true
		case "Init":
			if err := validateInitSig(fn); err != nil {
				return HookSet{}, fmt.Errorf("codegen: %s: %w", path, err)
			}
			out.Init = true
		}
	}
	return out, nil
}

// paramCount returns the parameter count of fn, treating a nil Type or
// Params as zero. Multi-name groups (e.g. `a, b int`) count as separate
// parameters so signature checks match call-site arity.
func paramCount(fn *goast.FuncDecl) int {
	if fn == nil || fn.Type == nil || fn.Type.Params == nil {
		return 0
	}
	n := 0
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			n++
			continue
		}
		n += len(field.Names)
	}
	return n
}

// resultCount returns the result count of fn.
func resultCount(fn *goast.FuncDecl) int {
	if fn == nil || fn.Type == nil || fn.Type.Results == nil {
		return 0
	}
	n := 0
	for _, field := range fn.Type.Results.List {
		if len(field.Names) == 0 {
			n++
			continue
		}
		n += len(field.Names)
	}
	return n
}

// errBadHandleSig and friends carry the canonical signature mismatch
// strings. Sentinel errors keep tests stable across formatting changes.
var (
	errBadHandleSig      = errors.New("Handle must have signature func(*kit.RequestEvent, kit.ResolveFn) (*kit.Response, error)")
	errBadHandleErrorSig = errors.New("HandleError must have signature func(*kit.RequestEvent, error) kit.SafeError")
	errBadHandleFetchSig = errors.New("HandleFetch must have signature func(*kit.RequestEvent, *http.Request) (*http.Response, error)")
	errBadRerouteSig     = errors.New("Reroute must have signature func(*url.URL) string")
	errBadInitSig        = errors.New("Init must have signature func(context.Context) error")
)

// validateHandleSig checks for `func Handle(*kit.RequestEvent, kit.ResolveFn)
// (*kit.Response, error)` by parameter and result arity. The full type
// identity is enforced at `go build` time on the generated adapter.
func validateHandleSig(fn *goast.FuncDecl) error {
	if paramCount(fn) != 2 || resultCount(fn) != 2 {
		return errBadHandleSig
	}
	return nil
}

func validateHandleErrorSig(fn *goast.FuncDecl) error {
	if paramCount(fn) != 2 || resultCount(fn) != 1 {
		return errBadHandleErrorSig
	}
	return nil
}

func validateHandleFetchSig(fn *goast.FuncDecl) error {
	if paramCount(fn) != 2 || resultCount(fn) != 2 {
		return errBadHandleFetchSig
	}
	return nil
}

func validateRerouteSig(fn *goast.FuncDecl) error {
	if paramCount(fn) != 1 || resultCount(fn) != 1 {
		return errBadRerouteSig
	}
	return nil
}

func validateInitSig(fn *goast.FuncDecl) error {
	if paramCount(fn) != 1 || resultCount(fn) != 1 {
		return errBadInitSig
	}
	return nil
}
