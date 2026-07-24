// Package config loads and validates coder-policy runtime settings.
package config

import (
	"net/url"
	"time"
)

// EnvPrefix is the Viper environment-variable prefix used by coder-policy.
const EnvPrefix = "CODER_POLICY"

// Config contains a validated snapshot of the proxy runtime configuration.
type Config struct {
	UpstreamURL            *url.URL
	PublicURL              *url.URL
	WildcardAccessHost     string
	AdminUsernames         map[string]struct{}
	AllowedUsernames       map[string]struct{}
	LogoutCSRFSecret       string
	ExpectedVersionPrefix  string
	ListenAddress          string
	HealthListenAddress    string
	MetricsListenAddress   string
	VersionCheckInterval   time.Duration
	UpstreamTimeout        time.Duration
	ShutdownTimeout        time.Duration
	MaxInspectionBodyBytes int64
}
