package proxy

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestHostClassifier verifies public, wildcard, and unknown host classification.
func TestHostClassifier(t *testing.T) {
	publicURL, _ := url.Parse("https://coder.example.com")
	tests := []struct {
		name     string
		wildcard string
		hosts    map[string]HostKind
	}{
		{
			name:     "dot wildcard",
			wildcard: "*.apps.example.com",
			hosts: map[string]HostKind{
				"coder.example.com":                         HostMain,
				"coder.example.com:443":                     HostMain,
				"coder-policy":                              HostUnknown,
				"coder-policy.coder.svc.cluster.local:8080": HostUnknown,
				"app--workspace--user.apps.example.com":     HostWildcard,
				"app--workspace--user.apps.example.com:443": HostWildcard,
				"apps.example.com":                          HostUnknown,
				"evil.example.com":                          HostUnknown,
			},
		},
		{
			name:     "hyphen wildcard",
			wildcard: "*-apps.example.com",
			hosts: map[string]HostKind{
				"coder.example.com":                         HostMain,
				"app--workspace--user-apps.example.com":     HostWildcard,
				"app--workspace--user-apps.example.com:443": HostWildcard,
				"apps.example.com":                          HostUnknown,
				"not-apps.example.com.evil.example":         HostUnknown,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classifier := NewHostClassifier(publicURL, test.wildcard)
			for host, expected := range test.hosts {
				if got := classifier.Classify(host); got != expected {
					t.Errorf("Classify(%q) = %d, want %d", host, got, expected)
				}
			}
		})
	}
}

// TestNormalizePathClosesEncodedRouteBypasses verifies ambiguous encodings cannot bypass policy routes.
func TestNormalizePathClosesEncodedRouteBypasses(t *testing.T) {
	tests := map[string]string{
		"/api/v2/users/me":            "/api/v2/users/me",
		"/api%2fv2/users/me":          "/api/v2/users/me",
		"/api%252fv2/users/me":        "/api/v2/users/me",
		"/ignored/../api/v2/users/me": "/api/v2/users/me",
		"/api\\v2\\users\\me":         "/api/v2/users/me",
		"//api//v2//users//me/":       "/api/v2/users/me",
	}
	for input, expected := range tests {
		request := httptest.NewRequest("GET", "http://proxy"+input, nil)
		got, err := NormalizePath(request)
		if err != nil {
			t.Errorf("NormalizePath(%q): %v", input, err)
			continue
		}
		if got != expected {
			t.Errorf("NormalizePath(%q) = %q, want %q", input, got, expected)
		}
	}
}

// TestWildcardControlPath verifies control-plane paths are hidden on wildcard application hosts.
func TestWildcardControlPath(t *testing.T) {
	for _, blocked := range []string{"/api/v2", "/api/v2/users/me", "/cli-auth", "/cli-auth/callback", "/setup"} {
		if !wildcardControlPath(blocked) {
			t.Errorf("expected %q to be blocked", blocked)
		}
	}
	for _, allowed := range []string{"/", "/api/v3", "/application", "/setup-app"} {
		if wildcardControlPath(allowed) {
			t.Errorf("expected %q to be allowed", allowed)
		}
	}
}
