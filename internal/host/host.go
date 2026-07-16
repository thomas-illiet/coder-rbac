// Package host provides canonical host normalization shared by configuration and routing.
package host

import (
	"net"
	"strings"
)

// Normalize lowercases a host and removes surrounding whitespace and a trailing dot.
func Normalize(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return strings.TrimSuffix(raw, ".")
}

// WithoutPort normalizes a host and removes its port when one is present.
func WithoutPort(raw string) string {
	raw = Normalize(raw)
	if hostname, _, err := net.SplitHostPort(raw); err == nil {
		return Normalize(hostname)
	}
	return raw
}
