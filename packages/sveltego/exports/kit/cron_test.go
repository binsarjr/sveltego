package kit_test

import (
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
)

func TestParseSchedule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spec    string
		want    time.Duration
		wantErr bool
	}{
		{"@hourly", time.Hour, false},
		{"@daily", 24 * time.Hour, false},
		{"@weekly", 7 * 24 * time.Hour, false},
		{"@every 5s", 5 * time.Second, false},
		{"@every 30m", 30 * time.Minute, false},
		{"@every 2h", 2 * time.Hour, false},
		{"@every 1h30m", 90 * time.Minute, false},
		// whitespace tolerance
		{"  @daily  ", 24 * time.Hour, false},
		{"@every  10s", 10 * time.Second, false},
		// errors
		{"@every 0s", 0, true},
		{"@every -5s", 0, true},
		{"@every 42", 0, true},   // bare integer, no unit
		{"@monthly", 0, true},    // unsupported shorthand
		{"*/5 * * * *", 0, true}, // full crontab not supported
		{"", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			got, err := kit.ParseSchedule(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSchedule(%q) = %v, want error", tc.spec, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSchedule(%q) error: %v", tc.spec, err)
			}
			if got != tc.want {
				t.Fatalf("ParseSchedule(%q) = %v, want %v", tc.spec, got, tc.want)
			}
		})
	}
}
