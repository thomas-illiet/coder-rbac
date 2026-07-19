package proxy

import (
	"net/http"
	"testing"
)

// TestAdmissionExemptPathKeepsOnlyNativeAuthenticationAndAssetsPublic verifies
// that a denied session cannot use a similar-looking path to bypass admission.
func TestAdmissionExemptPathKeepsOnlyNativeAuthenticationAndAssetsPublic(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{method: http.MethodGet, path: "/login", want: true},
		{method: http.MethodGet, path: "/assets/index.js", want: true},
		{method: http.MethodPost, path: "/api/v2/users/login", want: true},
		{method: http.MethodGet, path: "/api/v2/users/oauth2/github/callback", want: true},
		{method: http.MethodPost, path: "/api/v2/users/logout", want: true},
		{method: http.MethodGet, path: "/dashboard", want: false},
		{method: http.MethodGet, path: "/api/v2/users/me", want: false},
		{method: http.MethodGet, path: "/api/v2/workspaces", want: false},
		{method: http.MethodPost, path: "/api/v2/users/login-attacker", want: false},
		{method: http.MethodPost, path: "/assets/index.js", want: false},
	}
	for _, test := range tests {
		if got := admissionExemptPath(test.method, test.path); got != test.want {
			t.Errorf("admissionExemptPath(%q, %q) = %t, want %t", test.method, test.path, got, test.want)
		}
	}
}
