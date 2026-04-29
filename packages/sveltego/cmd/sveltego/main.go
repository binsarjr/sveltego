// Command sveltego is the CLI entry point for the sveltego framework.
package main

import (
	"fmt"
	"os"
)

func main() {
	exitCode := 0
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "sveltego: internal error: %v\n", r)
			os.Exit(2)
		}
		os.Exit(exitCode)
	}()

	if err := Execute(); err != nil {
		exitCode = 1
	}
}
