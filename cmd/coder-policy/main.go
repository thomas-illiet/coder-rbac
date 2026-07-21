// Package main provides the coder-policy executable entry point.
package main

import (
	"fmt"
	"os"

	"coder-policy/internal/command"
)

// main executes the coder-policy command and reports fatal command errors to stderr.
func main() {
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
