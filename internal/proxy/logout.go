package proxy

import (
	"net/http"
)

const browserLogoutRoute = "/_coder-policy/logout"

// browserLogoutPath reports whether a request invokes the browser logout flow.
func browserLogoutPath(method, normalizedPath string) bool {
	return method == http.MethodPost && normalizedPath == browserLogoutRoute
}

// logoutAndRedirect revokes the native Coder session before returning to login.
func (h *Handler) logoutAndRedirect(response http.ResponseWriter, request *http.Request) {
	logoutRequest := request.Clone(request.Context())
	logoutRequest.URL.Path = "/api/v2/users/logout"
	logoutRequest.URL.RawPath = ""
	logoutRequest.URL.RawQuery = ""
	h.upstream.ServeHTTP(&logoutRedirectWriter{
		ResponseWriter: response,
		loginURL:       h.classifier.publicURL + "/login",
	}, logoutRequest)
}

// logoutRedirectWriter preserves Coder cookie deletion and redirects successful logouts.
type logoutRedirectWriter struct {
	http.ResponseWriter
	loginURL    string
	wroteHeader bool
	redirected  bool
}

// WriteHeader converts a successful Coder logout response into a login redirect.
func (w *logoutRedirectWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		w.redirected = true
		w.Header().Del("Content-Length")
		w.Header().Del("Content-Type")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", w.loginURL)
		w.ResponseWriter.WriteHeader(http.StatusSeeOther)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write discards the native JSON body after a successful logout redirect.
func (w *logoutRedirectWriter) Write(payload []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.redirected {
		return len(payload), nil
	}
	return w.ResponseWriter.Write(payload)
}

// Unwrap exposes the underlying writer to net/http response controllers.
func (w *logoutRedirectWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
