package scenarios

import "log/slog"

// quietLogger returns a slog logger that discards all output. The bench
// pipeline emits debug/info events that would otherwise pollute results.
func quietLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
