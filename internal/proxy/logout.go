package proxy

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	browserLogoutRoute    = "/_coder-policy/logout"
	coderSessionCookie    = "coder_session_token"
	coderSessionHeader    = "Coder-Session-Token"
	logoutCSRFField       = "csrf_token"
	logoutCSRFValidity    = 5 * time.Minute
	logoutCSRFClockSkew   = 30 * time.Second
	maxLogoutRequestBytes = 4 << 10
)

// browserLogoutPath reports whether a request invokes the browser logout flow.
func browserLogoutPath(method, normalizedPath string) bool {
	return method == http.MethodPost && normalizedPath == browserLogoutRoute
}

// newLogoutCSRFToken creates a short-lived signed token for the account-change form.
func (h *Handler) newLogoutCSRFToken(now time.Time) string {
	expires := strconv.FormatInt(now.Add(logoutCSRFValidity).Unix(), 10)
	payload := expires + "." + rand.Text()
	return payload + "." + h.signLogoutCSRFPayload(payload)
}

// validLogoutCSRFToken verifies a signed form token against its expiry.
func (h *Handler) validLogoutCSRFToken(token string, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires < now.Add(-logoutCSRFClockSkew).Unix() || expires > now.Add(logoutCSRFValidity+logoutCSRFClockSkew).Unix() {
		return false
	}
	payload := parts[0] + "." + parts[1]
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(h.signLogoutCSRFPayload(payload))
	return err == nil && hmac.Equal(provided, expected)
}

// signLogoutCSRFPayload returns the encoded authentication tag for payload.
func (h *Handler) signLogoutCSRFPayload(payload string) string {
	mac := hmac.New(sha256.New, h.logoutCSRFKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// logoutAndRedirect revokes the native Coder session before returning to login.
func (h *Handler) logoutAndRedirect(response http.ResponseWriter, request *http.Request) bool {
	request.Body = http.MaxBytesReader(response, request.Body, maxLogoutRequestBytes)
	if err := request.ParseForm(); err != nil || !h.validLogoutCSRFToken(request.PostForm.Get(logoutCSRFField), time.Now()) {
		h.reject(response, request, Decision{
			Status:  http.StatusForbidden,
			Code:    "invalid_logout_csrf",
			Message: "The account-change request has expired. Reload the access denied page and try again.",
		})
		return false
	}

	logoutRequest := request.Clone(request.Context())
	logoutRequest.URL.Path = "/api/v2/users/logout"
	logoutRequest.URL.RawPath = ""
	logoutRequest.URL.RawQuery = ""
	logoutRequest.Body = nil
	logoutRequest.ContentLength = 0
	logoutRequest.Header.Del("Content-Length")
	logoutRequest.Header.Del("Content-Type")
	if cookie, err := request.Cookie(coderSessionCookie); err == nil && cookie.Value != "" {
		logoutRequest.Header.Set(coderSessionHeader, cookie.Value)
	}
	h.upstream.ServeHTTP(&logoutRedirectWriter{
		ResponseWriter: response,
		loginURL:       h.classifier.publicURL + "/login",
	}, logoutRequest)
	return true
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
