package config

import (
	"fmt"
	"net"
	"strings"
)

// validate enforces cross-field and security-sensitive configuration constraints.
func (c Config) validate() error {
	if c.LogoutCSRFSecret != "" && len(c.LogoutCSRFSecret) < 32 {
		return fmt.Errorf("logout-csrf-secret must contain at least 32 characters")
	}
	if c.ExpectedVersionPrefix == "" {
		return fmt.Errorf("expected-version-prefix is required")
	}
	if !strings.HasPrefix(c.ExpectedVersionPrefix, "v") {
		return fmt.Errorf("expected-version-prefix must start with v")
	}
	if c.VersionCheckInterval <= 0 {
		return fmt.Errorf("version-check-interval must be greater than zero")
	}
	if c.UpstreamTimeout <= 0 {
		return fmt.Errorf("upstream-timeout must be greater than zero")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown-timeout must be greater than zero")
	}
	if c.MaxInspectionBodyBytes <= 0 {
		return fmt.Errorf("max-inspection-body-bytes must be greater than zero")
	}
	return validateListenAddresses(c.ListenAddress, c.HealthListenAddress, c.MetricsListenAddress)
}

// validateListenAddresses ensures proxy, health, and metrics use valid, distinct ports.
func validateListenAddresses(proxyAddress, healthAddress, metricsAddress string) error {
	addresses := []struct {
		name    string
		address string
	}{
		{name: "listen-address", address: proxyAddress},
		{name: "health-listen-address", address: healthAddress},
		{name: "metrics-listen-address", address: metricsAddress},
	}
	ports := make(map[string]string, len(addresses))
	for _, item := range addresses {
		if item.address == "" {
			return fmt.Errorf("%s must not be empty", item.name)
		}
		_, port, err := net.SplitHostPort(item.address)
		if err != nil || port == "" {
			return fmt.Errorf("%s must be a valid host:port listen address", item.name)
		}
		if previous, exists := ports[port]; exists {
			return fmt.Errorf("%s must use a different port from %s", item.name, previous)
		}
		ports[port] = item.name
	}
	return nil
}
