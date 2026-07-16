package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	hostutil "github.com/thomas-illiet/coder-rbac/internal/host"
)

// HostKind identifies how a request host is allowed to use the proxy.
type HostKind uint8

const (
	// HostUnknown represents a host that is not declared in the configuration.
	HostUnknown HostKind = iota
	// HostMain represents the public Coder access host.
	HostMain
	// HostInternal represents an internal proxy Service host.
	HostInternal
	// HostWildcard represents a workspace application subdomain.
	HostWildcard
)

// String returns the stable Prometheus label for a host kind.
func (k HostKind) String() string {
	switch k {
	case HostMain:
		return "main"
	case HostInternal:
		return "internal"
	case HostWildcard:
		return "wildcard"
	default:
		return "unknown"
	}
}

// HostClassifier classifies normalized request hosts against configured endpoints.
type HostClassifier struct {
	publicHost     string
	publicHostname string
	internalHosts  map[string]struct{}
	wildcardSuffix string
}

// NewHostClassifier creates a classifier for the public, wildcard, and internal hosts.
func NewHostClassifier(publicURL *url.URL, wildcard string, internal []string) *HostClassifier {
	internalHosts := make(map[string]struct{}, len(internal)*2)
	for _, rawHost := range internal {
		normalized := hostutil.Normalize(rawHost)
		internalHosts[normalized] = struct{}{}
		internalHosts[hostutil.WithoutPort(normalized)] = struct{}{}
	}
	return &HostClassifier{
		publicHost:     hostutil.Normalize(publicURL.Host),
		publicHostname: hostutil.WithoutPort(publicURL.Host),
		internalHosts:  internalHosts,
		wildcardSuffix: strings.TrimPrefix(hostutil.Normalize(wildcard), "*"),
	}
}

// Classify returns the configured category of rawHost or HostUnknown.
func (c *HostClassifier) Classify(rawHost string) HostKind {
	host := hostutil.Normalize(rawHost)
	hostname := hostutil.WithoutPort(host)
	if host == c.publicHost || hostname == c.publicHostname {
		return HostMain
	}
	if _, ok := c.internalHosts[host]; ok {
		return HostInternal
	}
	if _, ok := c.internalHosts[hostname]; ok {
		return HostInternal
	}
	if strings.HasSuffix(hostname, c.wildcardSuffix) && len(hostname) > len(c.wildcardSuffix) {
		return HostWildcard
	}
	return HostUnknown
}

// NormalizePath repeatedly decodes and cleans a request path to close routing bypasses.
func NormalizePath(request *http.Request) (string, error) {
	decoded := request.URL.EscapedPath()
	if decoded == "" {
		decoded = "/"
	}
	for range 4 {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			break
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	if strings.ContainsRune(decoded, '\x00') {
		return "", fmt.Errorf("path contains a null byte")
	}
	decoded = strings.ReplaceAll(decoded, "\\", "/")
	if !strings.HasPrefix(decoded, "/") {
		decoded = "/" + decoded
	}
	normalized := path.Clean(decoded)
	if normalized == "." {
		normalized = "/"
	}
	return normalized, nil
}

// wildcardControlPath reports whether a path exposes a control-plane endpoint on an app host.
func wildcardControlPath(normalizedPath string) bool {
	return normalizedPath == "/api/v2" || strings.HasPrefix(normalizedPath, "/api/v2/") ||
		normalizedPath == "/cli-auth" || strings.HasPrefix(normalizedPath, "/cli-auth/") ||
		normalizedPath == "/setup" || strings.HasPrefix(normalizedPath, "/setup/")
}
