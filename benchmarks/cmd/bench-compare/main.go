// Command bench-compare gates pull requests on benchmark regressions.
//
// It invokes benchstat on two files of `go test -bench` output and exits
// non-zero when any benchmark regresses beyond the configured thresholds.
package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	exitOK   = 0
	exitFail = 1
	exitErr  = 2
)

type verdict string

const (
	verdictPass    verdict = "pass"
	verdictWarn    verdict = "warn"
	verdictFail    verdict = "FAIL"
	verdictAllowed verdict = "allowed"
	verdictSkip    verdict = "skip"
)

type row struct {
	name    string
	metric  string
	deltaPc float64
	pValue  float64
	hasP    bool
	noDiff  bool
	verdict verdict
}

type config struct {
	base            string
	head            string
	thresholdPct    float64
	allocsPct       float64
	allowRegression bool
	lookPath        func(string) (string, error)
	runBenchstat    func(base, head string) ([]byte, error)
	stdout          io.Writer
	stderr          io.Writer
}

func main() {
	cfg := config{
		lookPath:     exec.LookPath,
		runBenchstat: runBenchstat,
		stdout:       os.Stdout,
		stderr:       os.Stderr,
	}

	fs := flag.NewFlagSet("bench-compare", flag.ContinueOnError)
	fs.SetOutput(cfg.stderr)
	fs.StringVar(&cfg.base, "base", "", "baseline benchmark output file")
	fs.StringVar(&cfg.head, "head", "", "candidate benchmark output file")
	fs.Float64Var(&cfg.thresholdPct, "threshold-pct", 5, "max allowed sec/op regression in percent")
	fs.Float64Var(&cfg.allocsPct, "allocs-pct", 0, "max allowed allocs/op regression in percent")
	fs.BoolVar(&cfg.allowRegression, "allow-regression", false, "skip the gate, report only")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(exitErr)
	}

	if cfg.base == "" || cfg.head == "" {
		fmt.Fprintln(cfg.stderr, "bench-compare: -base and -head are required")
		os.Exit(exitErr)
	}

	os.Exit(run(cfg))
}

func run(cfg config) int {
	if _, err := cfg.lookPath("benchstat"); err != nil {
		fmt.Fprintln(cfg.stderr, "bench-compare: benchstat not found on PATH")
		fmt.Fprintln(cfg.stderr, "install: go install golang.org/x/perf/cmd/benchstat@latest")
		return exitErr
	}

	out, err := cfg.runBenchstat(cfg.base, cfg.head)
	if err != nil {
		fmt.Fprintf(cfg.stderr, "bench-compare: benchstat failed: %v\n", err)
		return exitErr
	}

	rows, err := parseBenchstatCSV(out)
	if err != nil {
		fmt.Fprintf(cfg.stderr, "bench-compare: parse: %v\n", err)
		return exitErr
	}

	rows = evaluate(rows, cfg.thresholdPct, cfg.allocsPct, cfg.allowRegression)
	printTable(cfg.stdout, rows)

	for _, r := range rows {
		if r.verdict == verdictFail {
			return exitFail
		}
	}
	return exitOK
}

func runBenchstat(base, head string) ([]byte, error) {
	cmd := exec.Command("benchstat", "-format=csv", base, head)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// parseBenchstatCSV parses the multi-section CSV emitted by `benchstat -format=csv`.
//
// Sections are separated by blank lines. Each section starts with two header
// lines (filenames row, then column-names row), followed by data rows. The
// metric is taken from the second column of the column-names row (sec/op,
// B/op, allocs/op).
func parseBenchstatCSV(data []byte) ([]row, error) {
	var rows []row
	sections := splitSections(data)
	for _, sec := range sections {
		if len(sec) < 3 {
			continue
		}
		r := csv.NewReader(strings.NewReader(strings.Join(sec, "\n")))
		r.FieldsPerRecord = -1
		records, err := r.ReadAll()
		if err != nil {
			return nil, err
		}
		if len(records) < 3 {
			continue
		}
		metric := ""
		if len(records[1]) > 1 {
			metric = strings.TrimSpace(records[1][1])
		}
		if metric == "" {
			continue
		}
		for _, rec := range records[2:] {
			if len(rec) < 7 {
				continue
			}
			name := strings.TrimSpace(rec[0])
			if name == "" || name == "geomean" {
				continue
			}
			vsBase := strings.TrimSpace(rec[5])
			pField := strings.TrimSpace(rec[6])
			row := row{name: name, metric: metric}
			if vsBase == "~" {
				row.noDiff = true
			} else {
				delta, err := parseDelta(vsBase)
				if err != nil {
					return nil, fmt.Errorf("row %q metric %q: %w", name, metric, err)
				}
				row.deltaPc = delta
			}
			if p, ok := parseP(pField); ok {
				row.pValue = p
				row.hasP = true
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func splitSections(data []byte) [][]string {
	lines := strings.Split(string(data), "\n")
	var sections [][]string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			sections = append(sections, cur)
			cur = nil
		}
	}
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if trim == "" {
			flush()
			continue
		}
		if !strings.Contains(ln, ",") {
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return sections
}

func parseDelta(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0, errors.New("empty delta")
	}
	return strconv.ParseFloat(s, 64)
}

func parseP(s string) (float64, bool) {
	for _, tok := range strings.Fields(s) {
		if v, ok := strings.CutPrefix(tok, "p="); ok {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return 0, false
			}
			return f, true
		}
	}
	return 0, false
}

func evaluate(rows []row, thresholdPct, allocsPct float64, allow bool) []row {
	out := make([]row, len(rows))
	for i, r := range rows {
		r.verdict = classify(r, thresholdPct, allocsPct)
		if allow && r.verdict == verdictFail {
			r.verdict = verdictAllowed
		}
		out[i] = r
	}
	return out
}

func classify(r row, thresholdPct, allocsPct float64) verdict {
	if r.noDiff {
		return verdictPass
	}
	if r.hasP && r.pValue >= 0.05 {
		return verdictSkip
	}
	limit := thresholdPct
	if r.metric == "allocs/op" {
		limit = allocsPct
	}
	if r.deltaPc <= 0 {
		return verdictPass
	}
	if r.deltaPc > limit {
		return verdictFail
	}
	return verdictWarn
}

func printTable(w io.Writer, rows []row) {
	fmt.Fprintln(w, "name\tmetric\tdelta\tp\tverdict")
	for _, r := range rows {
		delta := "~"
		if !r.noDiff {
			delta = fmt.Sprintf("%+.2f%%", r.deltaPc)
		}
		p := "-"
		if r.hasP {
			p = fmt.Sprintf("%.3f", r.pValue)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.name, r.metric, delta, p, r.verdict)
	}
}
