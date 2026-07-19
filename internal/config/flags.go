package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	defaultListenAddress        = ":8080"
	defaultHealthListenAddress  = ":8081"
	defaultMetricsListenAddress = ":9090"
	defaultVersionCheckInterval = time.Minute
	defaultUpstreamTimeout      = 15 * time.Second
	defaultShutdownTimeout      = 15 * time.Second
	defaultMaxInspectionBytes   = int64(1 << 20)
)

// RegisterFlags registers every proxy configuration flag with shared defaults.
func RegisterFlags(flags *pflag.FlagSet) {
	flags.String("upstream-url", "", "Internal Coder URL")
	flags.String("public-url", "", "Public Coder URL served by this proxy")
	flags.String("wildcard-access-host", "", "Wildcard application host, for example *.apps.example.com")
	flags.String("internal-hosts", "", "Comma-separated internal proxy hosts")
	flags.String("admin-usernames", "", "Comma-separated canonical Coder usernames exempt from workspace restrictions")
	flags.String("allowed-usernames", "", "Comma-separated canonical Coder usernames allowed to access Coder")
	flags.String("expected-version-prefix", "", "Required Coder version prefix, for example v2.35.")
	flags.String("listen-address", defaultListenAddress, "Proxy listen address")
	flags.String("health-listen-address", defaultHealthListenAddress, "Health server listen address")
	flags.String("metrics-listen-address", defaultMetricsListenAddress, "Prometheus metrics server listen address")
	flags.Duration("version-check-interval", defaultVersionCheckInterval, "Coder version recheck interval")
	flags.Duration("upstream-timeout", defaultUpstreamTimeout, "Timeout for policy API calls")
	flags.Duration("shutdown-timeout", defaultShutdownTimeout, "Graceful shutdown timeout")
	flags.Int64("max-inspection-body-bytes", defaultMaxInspectionBytes, "Maximum request body inspected by policy")
}

// NewViper creates a Viper instance with coder-rbac defaults, environment mapping, and flags.
func NewViper(flags *pflag.FlagSet) (*viper.Viper, error) {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	v.SetDefault("listen-address", defaultListenAddress)
	v.SetDefault("health-listen-address", defaultHealthListenAddress)
	v.SetDefault("metrics-listen-address", defaultMetricsListenAddress)
	v.SetDefault("version-check-interval", defaultVersionCheckInterval)
	v.SetDefault("upstream-timeout", defaultUpstreamTimeout)
	v.SetDefault("shutdown-timeout", defaultShutdownTimeout)
	v.SetDefault("max-inspection-body-bytes", defaultMaxInspectionBytes)

	if flags != nil {
		if err := v.BindPFlags(flags); err != nil {
			return nil, fmt.Errorf("bind flags: %w", err)
		}
	}
	return v, nil
}
