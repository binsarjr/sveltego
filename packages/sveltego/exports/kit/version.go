package kit

import "time"

// DefaultVersionPollInterval is the cadence the client poller uses when
// VersionPollConfig.PollInterval is the zero value. Mirrors the figure
// most SvelteKit apps land on once they enable polling — short enough
// to surface a deploy within a working session, long enough that an
// idle tab does not generate meaningful traffic.
const DefaultVersionPollInterval = 60 * time.Second

// MinVersionPollInterval is the floor enforced when normalizing a
// caller-provided interval. Anything smaller — including negative or
// sub-second values — is clamped to this floor so a config typo cannot
// hammer the endpoint.
const MinVersionPollInterval = time.Second

// VersionPollConfig configures the client-side version poller. The
// poller fetches /_app/version.json on PollInterval, compares the
// returned hash to the build version baked into the hydration payload,
// and flips the `updated.current` rune to true on drift. Disabled true
// suppresses the poller entirely, matching SvelteKit's pollInterval=0
// default.
type VersionPollConfig struct {
	// Disabled turns the poller off. The /_app/version.json endpoint
	// is still served so user code calling `updated.check()` works.
	Disabled bool
	// PollInterval is the cadence between background polls. Zero
	// resolves to DefaultVersionPollInterval; values below
	// MinVersionPollInterval are clamped up.
	PollInterval time.Duration
}

// Resolve returns a copy of c with zero/sub-floor values normalized to
// the package defaults. Always returns a usable config.
func (c VersionPollConfig) Resolve() VersionPollConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultVersionPollInterval
	}
	if c.PollInterval < MinVersionPollInterval {
		c.PollInterval = MinVersionPollInterval
	}
	return c
}
