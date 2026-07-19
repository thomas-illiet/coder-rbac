package config

import (
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/spf13/viper"

	hostutil "github.com/thomas-illiet/coder-rbac/internal/host"
)

// Load reads, normalizes, and validates the complete runtime configuration.
func Load(v *viper.Viper) (Config, error) {
	upstream, err := parseHTTPURL("upstream-url", v.GetString("upstream-url"))
	if err != nil {
		return Config{}, err
	}
	publicURL, err := parseHTTPURL("public-url", v.GetString("public-url"))
	if err != nil {
		return Config{}, err
	}
	wildcardHost, err := normalizeWildcard(v.GetString("wildcard-access-host"))
	if err != nil {
		return Config{}, err
	}
	internalHosts, err := parseHosts(v.GetString("internal-hosts"))
	if err != nil {
		return Config{}, fmt.Errorf("internal-hosts: %w", err)
	}
	if _, present := os.LookupEnv("CODER_RBAC_ALLOWED_USER_IDS"); present {
		return Config{}, fmt.Errorf("CODER_RBAC_ALLOWED_USER_IDS is no longer supported; use CODER_RBAC_ADMIN_USERNAMES")
	}
	if legacy := strings.TrimSpace(v.GetString("allowed-user-ids")); legacy != "" {
		return Config{}, fmt.Errorf("allowed-user-ids is no longer supported; use admin-usernames")
	}
	adminUsernames := parseUsernameSet(v.GetString("admin-usernames"))
	allowedUsernames := parseUsernameSet(v.GetString("allowed-usernames"))

	cfg := Config{
		UpstreamURL:            upstream,
		PublicURL:              publicURL,
		WildcardAccessHost:     wildcardHost,
		InternalHosts:          internalHosts,
		AdminUsernames:         adminUsernames,
		AllowedUsernames:       allowedUsernames,
		ExpectedVersionPrefix:  strings.TrimSpace(v.GetString("expected-version-prefix")),
		ListenAddress:          strings.TrimSpace(v.GetString("listen-address")),
		HealthListenAddress:    strings.TrimSpace(v.GetString("health-listen-address")),
		MetricsListenAddress:   strings.TrimSpace(v.GetString("metrics-listen-address")),
		VersionCheckInterval:   v.GetDuration("version-check-interval"),
		UpstreamTimeout:        v.GetDuration("upstream-timeout"),
		ShutdownTimeout:        v.GetDuration("shutdown-timeout"),
		MaxInspectionBodyBytes: v.GetInt64("max-inspection-body-bytes"),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// parseHTTPURL validates a required absolute HTTP URL without credentials or suffix data.
func parseHTTPURL(name, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute http or https URL", name)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must not contain credentials, a query, or a fragment", name)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return nil, fmt.Errorf("%s must not contain a path", name)
	}
	parsed.Path = ""
	return parsed, nil
}

// normalizeWildcard validates and normalizes a wildcard hostname in *.example.com form.
func normalizeWildcard(raw string) (string, error) {
	raw = hostutil.Normalize(raw)
	if !strings.HasPrefix(raw, "*.") || len(raw) <= 2 || strings.Contains(raw[2:], "*") {
		return "", fmt.Errorf("wildcard-access-host must use the form *.example.com")
	}
	if strings.Contains(raw, ":") {
		return "", fmt.Errorf("wildcard-access-host must not contain a port")
	}
	return raw, nil
}

// parseHosts parses, validates, sorts, and deduplicates a comma-separated host list.
func parseHosts(raw string) ([]string, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("at least one internal host is required")
	}
	hosts := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := hostutil.Normalize(part)
		if normalized == "" || strings.ContainsAny(normalized, "/?*#") {
			return nil, fmt.Errorf("invalid host %q", part)
		}
		hosts = append(hosts, normalized)
	}
	slices.Sort(hosts)
	return slices.Compact(hosts), nil
}

// parseUsernameSet parses canonical Coder usernames into an exact-match set.
func parseUsernameSet(raw string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, username := range splitCSV(raw) {
		result[username] = struct{}{}
	}
	return result
}

// splitCSV returns the non-empty, trimmed items in a comma-separated value.
func splitCSV(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
