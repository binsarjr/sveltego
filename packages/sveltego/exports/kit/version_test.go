package kit

import (
	"testing"
	"time"
)

// TestVersionPollConfig_ResolveZero pins the default interval — a
// caller that omits the field gets 60s polling, matching the Issue
// #461 acceptance criterion.
func TestVersionPollConfig_ResolveZero(t *testing.T) {
	t.Parallel()

	got := VersionPollConfig{}.Resolve()
	if got.PollInterval != DefaultVersionPollInterval {
		t.Errorf("zero interval resolved to %v, want %v", got.PollInterval, DefaultVersionPollInterval)
	}
	if got.Disabled {
		t.Errorf("Disabled flipped on resolve, want false")
	}
}

// TestVersionPollConfig_ResolveClampsBelowFloor ensures a misconfigured
// 100ms interval cannot escape onto the wire — anything below the
// 1-second floor is clamped up so a typo does not hammer the endpoint.
func TestVersionPollConfig_ResolveClampsBelowFloor(t *testing.T) {
	t.Parallel()

	got := VersionPollConfig{PollInterval: 100 * time.Millisecond}.Resolve()
	if got.PollInterval != MinVersionPollInterval {
		t.Errorf("100ms resolved to %v, want %v", got.PollInterval, MinVersionPollInterval)
	}
}

// TestVersionPollConfig_ResolvePreservesValid checks the pass-through:
// a sane interval resolves to itself unchanged.
func TestVersionPollConfig_ResolvePreservesValid(t *testing.T) {
	t.Parallel()

	in := VersionPollConfig{PollInterval: 30 * time.Second, Disabled: true}
	got := in.Resolve()
	if got.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want 30s", got.PollInterval)
	}
	if !got.Disabled {
		t.Errorf("Disabled lost on resolve, want true")
	}
}
