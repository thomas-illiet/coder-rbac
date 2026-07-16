// Package main provides the coder-rbac executable entry point.
package main

import (
	"fmt"
	"os"

	"github.com/thomas-illiet/coder-rbac/internal/command"
)

// main executes the coder-rbac command and reports fatal command errors to stderr.
func main() {
	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
