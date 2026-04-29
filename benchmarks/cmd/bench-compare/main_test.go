package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const sampleCSV = `goos: darwin
goarch: arm64
pkg: example/bench
,base.txt,,head.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1.000e-06,1%,1.040e-06,1%,+4.00%,p=0.002 n=6
Bar-8,1.000e-06,1%,1.060e-06,1%,+6.00%,p=0.002 n=6
Baz-8,1.000e-06,1%,1.100e-06,1%,+10.00%,p=0.200 n=6
Qux-8,1.000e-06,1%,1.000e-06,1%,~,p=1.000 n=6

,base.txt,,head.txt,,,
,B/op,CI,B/op,CI,vs base,P
Foo-8,32,0%,33,0%,+3.13%,p=0.002 n=6

,base.txt,,head.txt,,,
,allocs/op,CI,allocs/op,CI,vs base,P
Foo-8,1,0%,2,0%,+100.00%,p=0.002 n=6
Bar-8,1,0%,1,0%,~,p=1.000 n=6
`

func TestParseBenchstatCSV(t *testing.T) {
	rows, err := parseBenchstatCSV([]byte(sampleCSV))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := len(rows), 7; got != want {
		t.Fatalf("rows=%d want=%d", got, want)
	}
	r := rows[0]
	if r.name != "Foo-8" || r.metric != "sec/op" {
		t.Fatalf("row[0] = %+v", r)
	}
	if r.deltaPc < 3.99 || r.deltaPc > 4.01 {
		t.Fatalf("row[0].deltaPc=%v want=4.00", r.deltaPc)
	}
	if !r.hasP || r.pValue > 0.003 {
		t.Fatalf("row[0] p=%v hasP=%v", r.pValue, r.hasP)
	}
	if !rows[3].noDiff {
		t.Fatalf("row[3] expected noDiff for ~")
	}
}

func TestClassifyThreshold(t *testing.T) {
	tests := []struct {
		name string
		row  row
		want verdict
	}{
		{"4pct_below_thresh_warn", row{metric: "sec/op", deltaPc: 4.0, pValue: 0.001, hasP: true}, verdictWarn},
		{"6pct_above_thresh_fail", row{metric: "sec/op", deltaPc: 6.0, pValue: 0.001, hasP: true}, verdictFail},
		{"10pct_high_p_skip", row{metric: "sec/op", deltaPc: 10.0, pValue: 0.20, hasP: true}, verdictSkip},
		{"no_diff_pass", row{metric: "sec/op", noDiff: true}, verdictPass},
		{"improvement_pass", row{metric: "sec/op", deltaPc: -3.0, pValue: 0.001, hasP: true}, verdictPass},
		{"alloc_increase_fail", row{metric: "allocs/op", deltaPc: 100.0, pValue: 0.001, hasP: true}, verdictFail},
		{"alloc_no_diff_pass", row{metric: "allocs/op", noDiff: true}, verdictPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.row, 5, 0); got != tt.want {
				t.Fatalf("classify=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestEvaluateAllowRegression(t *testing.T) {
	rows := []row{
		{name: "Bar-8", metric: "sec/op", deltaPc: 6.0, pValue: 0.001, hasP: true},
	}
	out := evaluate(rows, 5, 0, true)
	if out[0].verdict != verdictAllowed {
		t.Fatalf("verdict=%v want=allowed", out[0].verdict)
	}
}

func TestRunIntegration(t *testing.T) {
	cfg := config{
		base:         "base.txt",
		head:         "head.txt",
		thresholdPct: 5,
		allocsPct:    0,
		lookPath:     func(string) (string, error) { return "/usr/bin/benchstat", nil },
		runBenchstat: func(string, string) ([]byte, error) { return []byte(sampleCSV), nil },
		stdout:       &bytes.Buffer{},
		stderr:       &bytes.Buffer{},
	}
	if got := run(cfg); got != exitFail {
		t.Fatalf("exit=%d want=%d", got, exitFail)
	}

	cfg.allowRegression = true
	cfg.stdout = &bytes.Buffer{}
	if got := run(cfg); got != exitOK {
		t.Fatalf("with allow-regression exit=%d want=%d", got, exitOK)
	}
}

func TestRunMissingBenchstat(t *testing.T) {
	stderr := &bytes.Buffer{}
	cfg := config{
		base:         "base.txt",
		head:         "head.txt",
		thresholdPct: 5,
		lookPath:     func(string) (string, error) { return "", errors.New("not found") },
		runBenchstat: func(string, string) ([]byte, error) { return nil, nil },
		stdout:       &bytes.Buffer{},
		stderr:       stderr,
	}
	if got := run(cfg); got != exitErr {
		t.Fatalf("exit=%d want=%d", got, exitErr)
	}
	if !strings.Contains(stderr.String(), "benchstat not found") {
		t.Fatalf("stderr missing hint: %q", stderr.String())
	}
}

func TestPrintTableContents(t *testing.T) {
	rows := []row{
		{name: "Foo-8", metric: "sec/op", deltaPc: 6, pValue: 0.001, hasP: true, verdict: verdictFail},
		{name: "Bar-8", metric: "sec/op", noDiff: true, verdict: verdictPass},
	}
	buf := &bytes.Buffer{}
	printTable(buf, rows)
	out := buf.String()
	for _, want := range []string{"Foo-8", "+6.00%", "FAIL", "Bar-8", "~", "pass"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q in:\n%s", want, out)
		}
	}
}
