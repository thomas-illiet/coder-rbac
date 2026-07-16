// Package buildinfo exposes build metadata injected through Go linker flags.
package buildinfo

var (
	// Version is the semantic version injected when the binary is built.
	Version = "dev"
	// Commit is the source revision injected when the binary is built.
	Commit = "unknown"
	// Date is the build timestamp injected when the binary is built.
	Date = "unknown"
)
