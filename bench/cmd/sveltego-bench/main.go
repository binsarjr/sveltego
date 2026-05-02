// Command sveltego-bench is a thin convenience driver around the bench
// scenarios. It runs each scenario for a fixed duration via httptest and
// reports rps and per-request latency, mirroring what `oha`/`wrk` would
// give over the wire — but without external tooling. The output is
// human-readable; CI uses `go test -bench=.` plus benchstat for the
// regression gate (see .github/workflows/bench.yml).
//
// Usage:
//
//	go run ./cmd/sveltego-bench -duration 5s
//	go run ./cmd/sveltego-bench -scenario hello -duration 10s
//
// adapter-bun comparison is documented at scripts/adapter-bun-compare.sh
// and remains out of scope for the MVP gate.
package main

import (
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"sort"
	"time"

	"github.com/binsarjr/sveltego/bench/scenarios"
)

const (
	exitOK  = 0
	exitErr = 1
)

type config struct {
	duration time.Duration
	scenario string
	mode     string
	stdout   *os.File
	stderr   *os.File
}

func main() {
	cfg := config{stdout: os.Stdout, stderr: os.Stderr}
	fs := flag.NewFlagSet("sveltego-bench", flag.ContinueOnError)
	fs.SetOutput(cfg.stderr)
	fs.DurationVar(&cfg.duration, "duration", 3*time.Second, "wall-clock duration per scenario")
	fs.StringVar(&cfg.scenario, "scenario", "", "single scenario to run by name (e.g. hello, ssg-serve); empty runs all")
	fs.StringVar(&cfg.mode, "mode", "all", "render mode subset: ssr | ssg | spa | static | all")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(exitErr)
	}
	os.Exit(run(cfg))
}

func run(cfg config) int {
	all, err := scenarios.All()
	if err != nil {
		_, _ = fmt.Fprintf(cfg.stderr, "sveltego-bench: build scenarios: %v\n", err)
		return exitErr
	}

	selected := all
	if cfg.scenario != "" {
		filtered := make([]scenarios.Scenario, 0, 1)
		for _, sc := range all {
			if sc.Name == cfg.scenario {
				filtered = append(filtered, sc)
			}
		}
		if len(filtered) == 0 {
			_, _ = fmt.Fprintf(cfg.stderr, "sveltego-bench: unknown scenario %q\n", cfg.scenario)
			return exitErr
		}
		selected = filtered
	} else if cfg.mode != "" && cfg.mode != "all" {
		want, ok := parseMode(cfg.mode)
		if !ok {
			_, _ = fmt.Fprintf(cfg.stderr, "sveltego-bench: unknown mode %q (want ssr|ssg|spa|static|all)\n", cfg.mode)
			return exitErr
		}
		filtered := make([]scenarios.Scenario, 0, len(all))
		for _, sc := range all {
			if sc.Mode == want {
				filtered = append(filtered, sc)
			}
		}
		if len(filtered) == 0 {
			_, _ = fmt.Fprintf(cfg.stderr, "sveltego-bench: no scenarios for mode %q\n", cfg.mode)
			return exitErr
		}
		selected = filtered
	}

	_, _ = fmt.Fprintf(cfg.stdout, "mode\tscenario\tn\trps\tp50\tp99\n")
	for _, sc := range selected {
		r := measure(sc, cfg.duration)
		_, _ = fmt.Fprintf(cfg.stdout, "%s\t%s\t%d\t%.0f\t%s\t%s\n",
			sc.Mode, sc.Name, r.n, r.rps, r.p50, r.p99)
	}
	return exitOK
}

func parseMode(s string) (scenarios.Mode, bool) {
	switch scenarios.Mode(s) {
	case scenarios.ModeSSR, scenarios.ModeSSG, scenarios.ModeSPA, scenarios.ModeStatic:
		return scenarios.Mode(s), true
	}
	return "", false
}

type result struct {
	n        int
	rps      float64
	p50, p99 time.Duration
}

func measure(sc scenarios.Scenario, dur time.Duration) result {
	rec := httptest.NewRecorder()
	deadline := time.Now().Add(dur)
	samples := make([]time.Duration, 0, 1<<14)
	start := time.Now()
	for time.Now().Before(deadline) {
		t := time.Now()
		_ = sc.Run(rec)
		samples = append(samples, time.Since(t))
	}
	elapsed := time.Since(start)
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return result{
		n:   len(samples),
		rps: float64(len(samples)) / elapsed.Seconds(),
		p50: percentile(samples, 0.50),
		p99: percentile(samples, 0.99),
	}
}

func percentile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * q)
	return sorted[idx]
}
