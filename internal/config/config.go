// Package config loads and validates coder-rbac runtime settings.
package config

import (
	"net/url"
	"time"
)

// EnvPrefix is the Viper environment-variable prefix used by coder-rbac.
const EnvPrefix = "CODER_RBAC"

// Config contains a validated snapshot of the proxy runtime configuration.
type Config struct {
	UpstreamURL            *url.URL
	PublicURL              *url.URL
	WildcardAccessHost     string
	InternalHosts          []string
	AllowedUsernames       map[string]struct{}
	ExpectedVersionPrefix  string
	ListenAddress          string
	HealthListenAddress    string
	MetricsListenAddress   string
	VersionCheckInterval   time.Duration
	UpstreamTimeout        time.Duration
	ShutdownTimeout        time.Duration
	MaxInspectionBodyBytes int64
}
