package kit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// CronTask declares a scheduled task the server runs in the background.
// Spec must be one of the supported shorthand forms: @every <duration>,
// @hourly, @daily, or @weekly. Fn receives a context that is cancelled
// when the server shuts down; errors are logged but do not stop the loop.
// Name is optional and used only in log output.
type CronTask struct {
	Name string
	Spec string
	Fn   func(ctx context.Context) error
}

// ParseSchedule converts a spec string into the interval between runs.
// Supported forms:
//
//	@every <duration>  — any duration parseable by time.ParseDuration
//	@hourly            — 1 hour
//	@daily             — 24 hours
//	@weekly            — 7 days
//
// Full crontab syntax is not supported; use @every for arbitrary intervals.
func ParseSchedule(spec string) (time.Duration, error) {
	spec = strings.TrimSpace(spec)
	switch spec {
	case "@hourly":
		return time.Hour, nil
	case "@daily":
		return 24 * time.Hour, nil
	case "@weekly":
		return 7 * 24 * time.Hour, nil
	}
	after, ok := strings.CutPrefix(spec, "@every ")
	if !ok {
		return 0, errors.New("kit: unsupported cron spec " + strconv.Quote(spec) + "; use @every <duration>, @hourly, @daily, or @weekly")
	}
	// Allow both pure duration strings ("5s") and unit-only shorthand ("5m").
	// time.ParseDuration handles those already; attempt an integer-only value
	// for the rare case where users omit units, which we reject with a clear message.
	d, err := parseDurationOrBare(strings.TrimSpace(after))
	if err != nil {
		return 0, errors.New("kit: invalid @every duration " + strconv.Quote(after) + ": " + err.Error())
	}
	if d <= 0 {
		return 0, errors.New("kit: @every duration must be positive, got " + strconv.Quote(after))
	}
	return d, nil
}

// parseDurationOrBare wraps time.ParseDuration and tries to surface a clean
// error for a bare integer (missing unit) rather than a cryptic parse failure.
func parseDurationOrBare(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		// If the string looks like a plain integer, hint the user.
		if _, intErr := strconv.ParseInt(s, 10, 64); intErr == nil {
			return 0, errors.New(err.Error() + " (did you mean " + s + "s?)")
		}
		return 0, err
	}
	return d, nil
}
