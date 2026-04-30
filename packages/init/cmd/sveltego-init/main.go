// Command sveltego-init scaffolds a fresh sveltego project on disk.
//
// Usage:
//
//	sveltego-init [--ai] [--force] [--non-interactive] [--module path] <target-dir>
//
// Without --ai, the AI-assistant templates copy is prompted on a TTY and
// defaulted to false on a piped stdin or with --non-interactive.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/binsarjr/sveltego/init/internal/scaffold"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "sveltego-init: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sveltego-init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: sveltego-init [--ai] [--force] [--non-interactive] [--module path] <target-dir>")
		fs.PrintDefaults()
	}
	var (
		ai       = fs.Bool("ai", false, "copy AI-assistant templates (AGENTS.md, CLAUDE.md, .cursorrules, copilot)")
		force    = fs.Bool("force", false, "overwrite existing files")
		nonInter = fs.Bool("non-interactive", false, "never prompt; default --ai to false if unset")
		module   = fs.String("module", "", "Go module path for the generated go.mod (defaults to target dir base name)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("exactly one target directory required")
	}
	target := fs.Arg(0)

	aiFlagSet := flagWasSet(fs, "ai")
	wantAI := *ai
	if !aiFlagSet {
		if *nonInter || !isTerminal(stdin) {
			wantAI = false
		} else {
			yes, err := promptYesNo(stdin, stdout, "Copy AI-assistant templates (AGENTS.md, CLAUDE.md, .cursorrules, copilot)? [y/N]: ")
			if err != nil {
				return fmt.Errorf("prompt: %w", err)
			}
			wantAI = yes
		}
	}

	res, err := scaffold.Run(scaffold.Options{
		Dir:    target,
		Module: *module,
		AI:     wantAI,
		Force:  *force,
	})
	if err != nil {
		return err
	}

	for _, w := range res.Written {
		fmt.Fprintf(stdout, "wrote   %s\n", w)
	}
	for _, s := range res.Skipped {
		fmt.Fprintf(stdout, "skipped %s (use --force to overwrite)\n", s)
	}
	if len(res.Skipped) > 0 && !*force {
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "%d file(s) skipped because they already exist. Re-run with --force to overwrite.\n", len(res.Skipped))
	}
	return nil
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// isTerminal reports whether r refers to a terminal device. The check is
// deliberately conservative: only *os.File with an IsTerminal-style Stat
// mode counts. Piped or wrapped readers are treated as non-interactive.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func promptYesNo(stdin io.Reader, stdout io.Writer, msg string) (bool, error) {
	fmt.Fprint(stdout, msg)
	br := bufio.NewReader(stdin)
	line, err := br.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
