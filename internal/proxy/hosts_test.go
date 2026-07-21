package proxy

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestHostClassifier verifies public, internal, wildcard, and unknown host classification.
func TestHostClassifier(t *testing.T) {
	publicURL, _ := url.Parse("https://coder.example.com")
	classifier := NewHostClassifier(publicURL, "*.apps.example.com", []string{"coder-policy", "coder-policy.coder.svc.cluster.local:8080"})
	tests := map[string]HostKind{
		"coder.example.com":                         HostMain,
		"coder.example.com:443":                     HostMain,
		"coder-policy":                              HostInternal,
		"coder-policy.coder.svc.cluster.local:8080": HostInternal,
		"app--workspace--user.apps.example.com":     HostWildcard,
		"app--workspace--user.apps.example.com:443": HostWildcard,
		"apps.example.com":                          HostUnknown,
		"evil.example.com":                          HostUnknown,
	}
	for host, expected := range tests {
		if got := classifier.Classify(host); got != expected {
			t.Errorf("Classify(%q) = %d, want %d", host, got, expected)
		}
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
