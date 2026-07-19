package proxy

import (
	"net/http"
	"strings"
)

// admissionExemptPath identifies native login and public asset routes that must
// remain reachable even when the browser still presents a denied user session.
func admissionExemptPath(method, normalizedPath string) bool {
	if method == http.MethodGet || method == http.MethodHead {
		if normalizedPath == "/login" || strings.HasPrefix(normalizedPath, "/login/") ||
			normalizedPath == "/reset-password" || strings.HasPrefix(normalizedPath, "/reset-password/") ||
			normalizedPath == "/assets" || strings.HasPrefix(normalizedPath, "/assets/") {
			return true
		}
	}
	for _, exactPath := range []string{
		"/api/v2/users/login",
		"/api/v2/users/logout",
		"/api/v2/users/password",
		"/api/v2/authmethods",
		"/api/v2/users/authmethods",
	} {
		if normalizedPath == exactPath {
			return true
		}
	}
	for _, prefix := range []string{"/api/v2/users/oauth2/", "/api/v2/users/oidc/", "/api/v2/users/otp/"} {
		if strings.HasPrefix(normalizedPath, prefix) {
			return true
		}
	}
	return false
}
