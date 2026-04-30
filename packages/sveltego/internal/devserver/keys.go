package devserver

// Structured log attribute keys. Centralized so sloglint sees a single
// definition site and so the keys stay consistent with the rest of the
// codebase (see internal/codegen and server packages).
const (
	logKeyComponent = "component"
	logKeyRoot      = "root"
	logKeyPort      = "port"
	logKeyURL       = "url"
	logKeyPath      = "path"
	logKeyTarget    = "target"
	logKeyKind      = "kind"
	logKeyFiles     = "files"
	logKeyError     = "err"
	logKeyElapsed   = "elapsed"
)
