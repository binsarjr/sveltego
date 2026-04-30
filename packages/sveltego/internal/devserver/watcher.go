// Package devserver implements `sveltego dev`: a coordinated process group
// that watches the project sources, re-runs codegen, restarts the user's
// Go HTTP server, and proxies the browser-facing port to a Vite dev
// server for client HMR.
package devserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ChangeKind classifies a file change so the supervisor knows whether a
// codegen-only refresh suffices or whether the Go server must be rebuilt
// and restarted.
type ChangeKind int

const (
	// ChangeSvelte signals one or more `.svelte` files changed; codegen
	// re-runs and the existing Go server picks up the new `.gen/` output.
	ChangeSvelte ChangeKind = iota
	// ChangeGo signals a user `.go` file under `src/routes/`,
	// `src/params/`, or `hooks.server.go` changed. Codegen re-runs and
	// the Go server is rebuilt and restarted.
	ChangeGo
)

// Change is one debounced batch of filesystem events the watcher emits.
// Paths is the set of unique absolute paths touched in the window.
type Change struct {
	Kind  ChangeKind
	Paths []string
}

// debounceWindow is how long the watcher coalesces events. 80ms covers
// editor atomic-write patterns (tmpfile + rename) without making the
// inner-loop feel sluggish.
const debounceWindow = 80 * time.Millisecond

// Watcher recursively watches src/ for `.svelte` and `.go` changes and
// emits debounced batches on Events.
type Watcher struct {
	root    string
	logger  *slog.Logger
	fsw     *fsnotify.Watcher
	events  chan Change
	srcDir  string
	hookDir string
}

// NewWatcher creates a Watcher rooted at projectRoot. The caller must
// call Run to start the loop. projectRoot must be absolute.
func NewWatcher(projectRoot string, logger *slog.Logger) (*Watcher, error) {
	if logger == nil {
		logger = slog.Default()
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("devserver: fsnotify: %w", err)
	}
	w := &Watcher{
		root:    projectRoot,
		logger:  logger,
		fsw:     fsw,
		events:  make(chan Change, 16),
		srcDir:  filepath.Join(projectRoot, "src"),
		hookDir: projectRoot,
	}
	if err := w.addRecursive(w.srcDir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	// Watch the project root non-recursively so hooks.server.go edits surface.
	if err := fsw.Add(w.hookDir); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("devserver: watch root %s: %w", w.hookDir, err)
	}
	return w, nil
}

// Events returns the channel that emits one Change per debounced batch.
// The channel closes when Run returns.
func (w *Watcher) Events() <-chan Change { return w.events }

// Run blocks until ctx is cancelled, draining fsnotify events and
// forwarding debounced batches to Events. The fsnotify watcher is closed
// before Run returns.
func (w *Watcher) Run(ctx context.Context) error {
	defer close(w.events)
	defer w.fsw.Close()

	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		pending = make(map[string]struct{})
		hasGo   bool
	)
	emit := func() {
		if len(pending) == 0 {
			return
		}
		paths := make([]string, 0, len(pending))
		for p := range pending {
			paths = append(paths, p)
		}
		kind := ChangeSvelte
		if hasGo {
			kind = ChangeGo
		}
		// Reset before send so a slow consumer doesn't double-emit.
		pending = make(map[string]struct{})
		hasGo = false
		select {
		case w.events <- Change{Kind: kind, Paths: paths}:
		case <-ctx.Done():
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if !w.relevant(ev) {
				continue
			}
			// New directories under src/ must be added recursively so the
			// watcher catches edits inside freshly-created route folders.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if err := w.addRecursive(ev.Name); err != nil {
						w.logger.Warn("devserver: watch new dir",
							logKeyPath, ev.Name,
							logKeyError, err,
						)
					}
					continue
				}
			}
			switch strings.ToLower(filepath.Ext(ev.Name)) {
			case ".svelte":
				// classified as ChangeSvelte by default
			case ".go":
				hasGo = true
			default:
				continue
			}
			pending[ev.Name] = struct{}{}
			if timer == nil {
				timer = time.NewTimer(debounceWindow)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(debounceWindow)
			}
			timerC = timer.C
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				w.logger.Warn("devserver: fsnotify error", logKeyError, err)
			}
		case <-timerC:
			emit()
			timer = nil
			timerC = nil
		}
	}
}

// relevant filters fsnotify events to the subset that should trigger a
// rebuild. Edits inside `.gen/` (codegen output) and `node_modules/` are
// ignored to avoid feedback loops.
func (w *Watcher) relevant(ev fsnotify.Event) bool {
	// Drop the chmod-only churn macOS emits on save.
	if ev.Op == fsnotify.Chmod {
		return false
	}
	rel, err := filepath.Rel(w.root, ev.Name)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return false
	}
	if strings.HasPrefix(rel, ".gen/") || rel == ".gen" {
		return false
	}
	if strings.HasPrefix(rel, "node_modules/") || strings.Contains(rel, "/node_modules/") {
		return false
	}
	if strings.HasPrefix(rel, "build/") || rel == "build" {
		return false
	}
	// At the project root we only care about hooks.server.go. Other root
	// files (go.mod, package.json, vite.config.js, app.html…) don't
	// trigger the watcher in this MVP — restart manually if needed.
	if !strings.Contains(rel, "/") {
		return filepath.Base(rel) == "hooks.server.go"
	}
	// Anywhere under src/ is fair game.
	if strings.HasPrefix(rel, "src/") {
		return true
	}
	return false
}

// addRecursive registers dir and every nested directory with the fsnotify
// watcher. Missing dir is not an error — `src/` may not exist yet on
// fresh scaffolds.
func (w *Watcher) addRecursive(dir string) error {
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".gen" || name == "node_modules" || name == "build" {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(path); err != nil {
			return fmt.Errorf("devserver: watch %s: %w", path, err)
		}
		return nil
	})
}
