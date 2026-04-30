package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	c := NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	c.SetOut(&outBuf)
	c.SetErr(&errBuf)
	c.SetArgs(args)
	err = c.Execute()
	return outBuf.String(), errBuf.String(), err
}

func resetLoggerOnCleanup(t *testing.T) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})
}

func TestRoot_Help(t *testing.T) {
	resetLoggerOnCleanup(t)
	stdout, _, err := runCmd(t)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("expected help text to contain Usage:, got %q", stdout)
	}
	for _, sub := range []string{"build", "compile", "dev", "check", "routes", "version"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected help to list %q subcommand, got %q", sub, stdout)
		}
	}
}

func TestVersion(t *testing.T) {
	resetLoggerOnCleanup(t)
	stdout, _, err := runCmd(t, "version")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	re := regexp.MustCompile(`^sveltego v\S+ \(go\d+\.\d+(?:\.\d+)?, \S+/\S+\)\n$`)
	if !re.MatchString(stdout) {
		t.Errorf("version output %q did not match expected format", stdout)
	}
}

func TestBuildHelp(t *testing.T) {
	resetLoggerOnCleanup(t)
	stdout, _, err := runCmd(t, "build", "--help")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	for _, sub := range []string{"--out", "--main"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected build help to mention %q, got %q", sub, stdout)
		}
	}
}

func TestCompileHelp(t *testing.T) {
	resetLoggerOnCleanup(t)
	stdout, _, err := runCmd(t, "compile", "--help")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stdout, "Compile") {
		t.Errorf("expected compile help to mention Compile, got %q", stdout)
	}
}

func TestDevStub(t *testing.T) {
	resetLoggerOnCleanup(t)
	_, stderr, err := runCmd(t, "dev")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stderr, "v0.3") || !strings.Contains(stderr, "42") {
		t.Errorf("unexpected dev stub message: %q", stderr)
	}
}

func TestCheckStub(t *testing.T) {
	resetLoggerOnCleanup(t)
	_, stderr, err := runCmd(t, "check")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(stderr, "not implemented") {
		t.Errorf("unexpected check stub message: %q", stderr)
	}
}

func TestVerboseFlag(t *testing.T) {
	resetLoggerOnCleanup(t)

	// Silence the side-effect log writes during the test by routing the
	// configured logger to a discarded buffer-equivalent. The CLI sends to
	// os.Stderr by default; tests just observe slog.Default()'s Enabled.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open /dev/null: %v", err)
	}
	t.Cleanup(func() { _ = devNull.Close() })

	_, _, err = runCmd(t, "-vv", "version")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("expected DEBUG level enabled after -vv, but it is not")
	}
}

func TestUnknownCommand(t *testing.T) {
	resetLoggerOnCleanup(t)
	_, _, err := runCmd(t, "nope")
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
}
