package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// readinessSnapshot is an immutable readinessReader used by handler tests.
type readinessSnapshot struct {
	ready   bool
	version string
	err     error
}

// Ready returns the configured test readiness snapshot.
func (snapshot readinessSnapshot) Ready() (bool, string, error) {
	return snapshot.ready, snapshot.version, snapshot.err
}

// roundTripperFunc adapts a function to an HTTP round tripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip invokes the adapted HTTP round-trip function.
func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

// TestHealthHandlerReportsLivenessAndReadiness verifies both health contracts.
func TestHealthHandlerReportsLivenessAndReadiness(t *testing.T) {
	tests := []struct {
		name       string
		snapshot   readinessSnapshot
		path       string
		wantStatus int
		wantBody   map[string]string
	}{
		{name: "liveness", path: "/healthz", wantStatus: http.StatusOK, wantBody: map[string]string{"status": "ok"}},
		{name: "ready", snapshot: readinessSnapshot{ready: true, version: "v2.35.7"}, path: "/readyz", wantStatus: http.StatusOK, wantBody: map[string]string{"status": "ready", "coder_version": "v2.35.7"}},
		{name: "unready", snapshot: readinessSnapshot{err: errors.New("version mismatch")}, path: "/readyz", wantStatus: http.StatusServiceUnavailable, wantBody: map[string]string{"status": "unready"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			healthHandler(test.snapshot).ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if len(body) != len(test.wantBody) {
				t.Fatalf("body = %#v, want %#v", body, test.wantBody)
			}
			for key, want := range test.wantBody {
				if body[key] != want {
					t.Errorf("body[%q] = %q, want %q", key, body[key], want)
				}
			}
		})
	}
}

// TestMetricsHandlerExposesOnlyMetricsRoute verifies readiness metrics and route isolation.
func TestMetricsHandlerExposesOnlyMetricsRoute(t *testing.T) {
	registry, _ := newMetricsRegistry(readinessSnapshot{ready: true, version: "v2.35.7"})
	handler := metricsHandler(registry)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "coder_rbac_coder_version_ready 1") || !strings.Contains(body, "coder_rbac_build_info") {
		t.Fatalf("metrics body does not contain readiness and build metrics")
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("root status = %d, want 404", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST metrics status = %d, want 405", response.Code)
	}
}

// TestReverseProxyPreservesIncomingHost verifies URL rewriting and forwarded request metadata.
func TestReverseProxyPreservesIncomingHost(t *testing.T) {
	// observedRequest captures the request received by the upstream server.
	type observedRequest struct {
		host           string
		path           string
		rawQuery       string
		forwardedHost  string
		forwardedProto string
	}
	observed := make(chan observedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		observed <- observedRequest{
			host:           request.Host,
			path:           request.URL.Path,
			rawQuery:       request.URL.RawQuery,
			forwardedHost:  request.Header.Get("X-Forwarded-Host"),
			forwardedProto: request.Header.Get("X-Forwarded-Proto"),
		}
		response.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(response, "proxied")
	}))
	defer upstream.Close()
	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	proxyServer := httptest.NewServer(newReverseProxy(upstreamURL, upstream.Client().Transport, serverTestLogger()))
	defer proxyServer.Close()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, proxyServer.URL+"/api/v2/workspaces?limit=5", nil)
	if err != nil {
		t.Fatalf("create proxy request: %v", err)
	}
	request.Host = "coder.example.com"
	response, err := proxyServer.Client().Do(request)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.StatusCode)
	}

	got := <-observed
	if got.host != "coder.example.com" || got.path != "/api/v2/workspaces" || got.rawQuery != "limit=5" {
		t.Fatalf("upstream request = %#v", got)
	}
	if got.forwardedHost != "coder.example.com" || got.forwardedProto != "http" {
		t.Fatalf("forwarded headers = host %q, proto %q", got.forwardedHost, got.forwardedProto)
	}
}

// TestReverseProxyReturnsStableUpstreamError verifies dependency failures remain fail closed.
func TestReverseProxyReturnsStableUpstreamError(t *testing.T) {
	upstreamURL, err := url.Parse("http://coder.invalid")
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://coder.example.com/api/v2/users/me", nil)
	request.Host = "coder.example.com"
	newReverseProxy(upstreamURL, transport, serverTestLogger()).ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "upstream_unavailable" || body["message"] != "Coder upstream is unavailable." {
		t.Fatalf("body = %#v", body)
	}
}

// serverTestLogger returns a logger that discards test output.
func serverTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
