package coder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// TestClientForwardsOnlyAuthenticationHeaders verifies source authentication is copied exactly.
func TestClientForwardsOnlyAuthenticationHeaders(t *testing.T) {
	// receivedRequest captures the upstream authentication request.
	type receivedRequest struct {
		path    string
		headers http.Header
	}
	received := make(chan receivedRequest, 1)
	client := newTestClient(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		received <- receivedRequest{path: request.URL.Path, headers: request.Header.Clone()}
		response.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(response, `{"username":"platform-admin"}`)
	}))

	source := httptest.NewRequest(http.MethodGet, "https://coder.example.com/", nil)
	source.Header["Authorization"] = []string{"Bearer primary", "Bearer secondary"}
	source.Header["Coder-Session-Token"] = []string{"session-token"}
	source.Header["Cookie"] = []string{"session=one", "other=two"}
	source.Header.Set("Accept", "text/html")
	source.Header.Set("X-Do-Not-Forward", "secret")

	user, err := client.CurrentUser(context.Background(), source)
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	if user.Username != "platform-admin" {
		t.Fatalf("username = %q, want platform-admin", user.Username)
	}

	request := <-received
	if request.path != "/api/v2/users/me" {
		t.Fatalf("path = %q, want /api/v2/users/me", request.path)
	}
	wantHeaders := http.Header{
		"Authorization":       []string{"Bearer primary", "Bearer secondary"},
		"Coder-Session-Token": []string{"session-token"},
		"Cookie":              []string{"session=one", "other=two"},
	}
	for name, want := range wantHeaders {
		if got := request.headers.Values(name); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
	}
	if got := request.headers.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, want application/json", got)
	}
	if got := request.headers.Get("X-Do-Not-Forward"); got != "" {
		t.Errorf("X-Do-Not-Forward = %q, want empty", got)
	}
}

// TestCurrentUserRejectsInvalidUsername verifies identity responses need one canonical username.
func TestCurrentUserRejectsInvalidUsername(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing username", body: `{}`},
		{name: "empty username", body: `{"username":""}`},
		{name: "surrounding whitespace", body: `{"username":" admin "}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, test.body)
			}))

			_, err := client.CurrentUser(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
			if err == nil || !strings.Contains(err.Error(), "invalid username") {
				t.Fatalf("error = %v, want invalid username", err)
			}
		})
	}
}

// TestClientDecodesJSONWithTrailingWhitespace verifies one JSON value and trailing space are accepted.
func TestClientDecodesJSONWithTrailingWhitespace(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "Application/JSON; charset=utf-8")
		_, _ = io.WriteString(response, "{\"version\":\"v2.35.7\"}\n \t")
	}))

	info, err := client.BuildInfo(context.Background())
	if err != nil {
		t.Fatalf("build info: %v", err)
	}
	if info.Version != "v2.35.7" {
		t.Fatalf("version = %q, want v2.35.7", info.Version)
	}
}

// TestClientRejectsInvalidJSONResponses verifies content type and single-value decoding rules.
func TestClientRejectsInvalidJSONResponses(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantError   string
	}{
		{name: "non JSON content", contentType: "text/html", body: "<html></html>", wantError: "non-JSON content"},
		{name: "malformed JSON", contentType: "application/json", body: `{"version":`, wantError: "decode Coder API"},
		{name: "additional JSON", contentType: "application/json", body: `{"version":"v2.35.1"} {"version":"v2.35.2"}`, wantError: "additional JSON value"},
		{name: "trailing garbage", contentType: "application/json", body: `{"version":"v2.35.1"} invalid`, wantError: "trailing data"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", test.contentType)
				_, _ = io.WriteString(response, test.body)
			}))

			_, err := client.BuildInfo(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want text %q", err, test.wantError)
			}
		})
	}
}

// TestClientReturnsAPIError verifies non-success responses preserve status and operation details.
func TestClientReturnsAPIError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain")
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(response, "authentication required")
	}))

	_, err := client.CurrentUser(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized || apiErr.Operation != "/api/v2/users/me" {
		t.Fatalf("API error = %#v", apiErr)
	}
}

// TestClientEnforcesResponseLimit verifies the exact 4 MiB boundary is accepted and overflow fails.
func TestClientEnforcesResponseLimit(t *testing.T) {
	const envelopeBytes = len(`{"version":""}`)
	tests := []struct {
		name      string
		bodyBytes int
		wantError bool
	}{
		{name: "exact limit", bodyBytes: maxAPIResponseBytes},
		{name: "one byte over", bodyBytes: maxAPIResponseBytes + 1, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := strings.Repeat("v", test.bodyBytes-envelopeBytes)
			body := `{"version":"` + version + `"}`
			if len(body) != test.bodyBytes {
				t.Fatalf("test body length = %d, want %d", len(body), test.bodyBytes)
			}
			client := newTestClient(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, body)
			}))

			info, err := client.BuildInfo(context.Background())
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "response exceeds 4194304 bytes") {
					t.Fatalf("error = %v, want response limit error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("build info: %v", err)
			}
			if info.Version != version {
				t.Fatalf("decoded version length = %d, want %d", len(info.Version), len(version))
			}
		})
	}
}

// newTestClient starts an HTTP server and returns a client configured to call it.
func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return NewClient(baseURL, server.Client())
}
