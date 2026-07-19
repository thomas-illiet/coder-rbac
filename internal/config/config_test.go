package config

import (
	"testing"
	"time"

	"github.com/spf13/pflag"
)

// TestLoadEnvironmentAndFlagPrecedence verifies flags override environment values and defaults.
func TestLoadEnvironmentAndFlagPrecedence(t *testing.T) {
	t.Setenv("CODER_RBAC_UPSTREAM_URL", "http://coder:3000")
	t.Setenv("CODER_RBAC_PUBLIC_URL", "https://coder.example.com")
	t.Setenv("CODER_RBAC_WILDCARD_ACCESS_HOST", "*.apps.example.com")
	t.Setenv("CODER_RBAC_INTERNAL_HOSTS", "coder-rbac,coder-rbac.ns.svc.cluster.local")
	t.Setenv("CODER_RBAC_ADMIN_USERNAMES", "platform-admin, automation, platform-admin")
	t.Setenv("CODER_RBAC_ALLOWED_USERNAMES", "developer, developer")
	t.Setenv("CODER_RBAC_EXPECTED_VERSION_PREFIX", "v2.35.")
	t.Setenv("CODER_RBAC_UPSTREAM_TIMEOUT", "7s")
	t.Setenv("CODER_RBAC_METRICS_LISTEN_ADDRESS", "127.0.0.1:9191")

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterFlags(flags)
	if err := flags.Set("public-url", "https://flag.example.com"); err != nil {
		t.Fatal(err)
	}

	v, err := NewViper(flags)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.UpstreamURL.String(), "http://coder:3000"; got != want {
		t.Fatalf("upstream URL = %q, want %q", got, want)
	}
	if got, want := cfg.PublicURL.String(), "https://flag.example.com"; got != want {
		t.Fatalf("public URL = %q, want %q", got, want)
	}
	if len(cfg.AdminUsernames) != 2 {
		t.Fatalf("admin usernames = %d, want 2", len(cfg.AdminUsernames))
	}
	for _, username := range []string{"platform-admin", "automation"} {
		if _, ok := cfg.AdminUsernames[username]; !ok {
			t.Errorf("admin usernames do not contain %q", username)
		}
	}
	if _, ok := cfg.AllowedUsernames["developer"]; !ok || len(cfg.AllowedUsernames) != 1 {
		t.Errorf("allowed usernames = %#v, want developer", cfg.AllowedUsernames)
	}
	if cfg.UpstreamTimeout != 7*time.Second {
		t.Fatalf("upstream timeout = %s, want 7s", cfg.UpstreamTimeout)
	}
	if cfg.MetricsListenAddress != "127.0.0.1:9191" {
		t.Fatalf("metrics listen address = %q, want 127.0.0.1:9191", cfg.MetricsListenAddress)
	}
	if cfg.MaxInspectionBodyBytes != defaultMaxInspectionBytes {
		t.Fatalf("maximum inspection body = %d, want %d", cfg.MaxInspectionBodyBytes, defaultMaxInspectionBytes)
	}
}

// TestLoadRejectsLegacyAllowedUserIDs verifies obsolete UUID configuration fails explicitly.
func TestLoadRejectsLegacyAllowedUserIDs(t *testing.T) {
	t.Setenv("CODER_RBAC_ALLOWED_USER_IDS", "")
	v, err := NewViper(nil)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("upstream-url", "http://coder:3000")
	v.Set("public-url", "https://coder.example.com")
	v.Set("wildcard-access-host", "*.apps.example.com")
	v.Set("internal-hosts", "coder-rbac")
	v.Set("expected-version-prefix", "v2.35.")
	if _, err := Load(v); err == nil || err.Error() != "CODER_RBAC_ALLOWED_USER_IDS is no longer supported; use CODER_RBAC_ADMIN_USERNAMES" {
		t.Fatalf("error = %v", err)
	}
}

// TestRegisterFlagsUsesSharedDefaults verifies CLI and environment-only loading share one default set.
func TestRegisterFlagsUsesSharedDefaults(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterFlags(flags)
	if flags.Lookup("admin-usernames") == nil {
		t.Fatal("admin-usernames flag is not registered")
	}
	if flags.Lookup("allowed-usernames") == nil {
		t.Fatal("allowed-usernames flag is not registered")
	}
	if flags.Lookup("allowed-user-ids") != nil {
		t.Fatal("legacy allowed-user-ids flag is still registered")
	}
	v, err := NewViper(flags)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("upstream-url", "http://coder:3000")
	v.Set("public-url", "https://coder.example.com")
	v.Set("wildcard-access-host", "*.apps.example.com")
	v.Set("internal-hosts", "coder-rbac")
	v.Set("expected-version-prefix", "v2.35.")
	cfg, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != defaultListenAddress || cfg.HealthListenAddress != defaultHealthListenAddress || cfg.MetricsListenAddress != defaultMetricsListenAddress {
		t.Fatalf("listener defaults = %q, %q, %q", cfg.ListenAddress, cfg.HealthListenAddress, cfg.MetricsListenAddress)
	}
	if cfg.VersionCheckInterval != defaultVersionCheckInterval || cfg.UpstreamTimeout != defaultUpstreamTimeout || cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("duration defaults = %s, %s, %s", cfg.VersionCheckInterval, cfg.UpstreamTimeout, cfg.ShutdownTimeout)
	}
}

// TestLoadNormalizesConfiguredHosts verifies host normalization and deduplication happen during loading.
func TestLoadNormalizesConfiguredHosts(t *testing.T) {
	v, err := NewViper(nil)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("upstream-url", "http://coder:3000")
	v.Set("public-url", "https://coder.example.com")
	v.Set("wildcard-access-host", " *.APPS.EXAMPLE.COM. ")
	v.Set("internal-hosts", " CODER-RBAC. , coder-rbac")
	v.Set("expected-version-prefix", "v2.35.")
	cfg, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WildcardAccessHost != "*.apps.example.com" {
		t.Fatalf("wildcard host = %q", cfg.WildcardAccessHost)
	}
	if len(cfg.InternalHosts) != 1 || cfg.InternalHosts[0] != "coder-rbac" {
		t.Fatalf("internal hosts = %#v", cfg.InternalHosts)
	}
}

// TestLoadRejectsSharedListenerPorts verifies Prometheus cannot share a proxy or health port.
func TestLoadRejectsSharedListenerPorts(t *testing.T) {
	v, err := NewViper(nil)
	if err != nil {
		t.Fatal(err)
	}
	v.Set("upstream-url", "http://coder:3000")
	v.Set("public-url", "https://coder.example.com")
	v.Set("wildcard-access-host", "*.apps.example.com")
	v.Set("internal-hosts", "coder-rbac")
	v.Set("expected-version-prefix", "v2.35.")
	v.Set("metrics-listen-address", "127.0.0.1:8080")
	if _, err := Load(v); err == nil {
		t.Fatal("expected duplicate listener port validation error")
	}
}

// TestLoadRejectsInvalidSecurityConfiguration verifies unsafe or incomplete settings fail validation.
func TestLoadRejectsInvalidSecurityConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		wildcard string
		expected string
	}{
		{name: "missing version", wildcard: "*.apps.example.com", expected: ""},
		{name: "non wildcard host", wildcard: "apps.example.com", expected: "v2.35."},
		{name: "wildcard port", wildcard: "*.apps.example.com:443", expected: "v2.35."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := NewViper(nil)
			if err != nil {
				t.Fatal(err)
			}
			v.Set("upstream-url", "http://coder:3000")
			v.Set("public-url", "https://coder.example.com")
			v.Set("wildcard-access-host", test.wildcard)
			v.Set("internal-hosts", "coder-rbac")
			v.Set("expected-version-prefix", test.expected)
			if _, err := Load(v); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
