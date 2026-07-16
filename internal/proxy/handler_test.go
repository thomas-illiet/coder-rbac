package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/thomas-illiet/coder-rbac/internal/coder"
	"github.com/thomas-illiet/coder-rbac/internal/versiongate"
)

// TestHandlerHostRoutingAndWildcardProtection verifies host policy and original Host forwarding.
func TestHandlerHostRoutingAndWildcardProtection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v2/buildinfo" {
			_, _ = response.Write([]byte(`{"version":"v2.35.2"}`))
			return
		}
		_, _ = fmt.Fprintf(response, `{"host":%q,"path":%q,"forwarded_host":%q}`, request.Host, request.URL.Path, request.Header.Get("X-Forwarded-Host"))
	}))
	defer upstream.Close()
	handler := newReadyTestHandler(t, upstream)
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name       string
		host       string
		path       string
		statusCode int
		body       string
	}{
		{name: "main", host: "coder.example.com", path: "/dashboard", statusCode: http.StatusOK, body: `"forwarded_host":"coder.example.com"`},
		{name: "internal", host: "coder-rbac.coder.svc.cluster.local:8080", path: "/api/v2/workspaces", statusCode: http.StatusOK, body: "coder-rbac.coder.svc.cluster.local:8080"},
		{name: "wildcard application", host: "code--dev--alice.apps.example.com", path: "/", statusCode: http.StatusOK},
		{name: "wildcard API", host: "code--dev--alice.apps.example.com", path: "/api/v2/users/me", statusCode: http.StatusNotFound},
		{name: "encoded wildcard API", host: "code--dev--alice.apps.example.com", path: "/api%252fv2/users/me", statusCode: http.StatusNotFound},
		{name: "unknown host", host: "attacker.example.com", path: "/", statusCode: http.StatusMisdirectedRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, server.URL+test.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Host = test.host
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != test.statusCode {
				t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, test.statusCode, body)
			}
			if test.body != "" && !strings.Contains(string(body), test.body) {
				t.Fatalf("body %q does not contain %q", body, test.body)
			}
		})
	}
}

// TestHandlerRelaysUpgradedConnections verifies upgraded connections pass through the proxy.
func TestHandlerRelaysUpgradedConnections(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v2/buildinfo" {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"version":"v2.35.2"}`))
			return
		}
		if !strings.EqualFold(request.Header.Get("Connection"), "upgrade") {
			http.Error(response, "upgrade required", http.StatusBadRequest)
			return
		}
		hijacker, ok := response.(http.Hijacker)
		if !ok {
			t.Error("upstream response is not hijackable")
			return
		}
		connection, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer connection.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: coder-test\r\n\r\n")
		_ = buffered.Flush()
		message := make([]byte, 4)
		if _, err := io.ReadFull(buffered, message); err != nil {
			t.Errorf("read upgraded payload: %v", err)
			return
		}
		if string(message) == "ping" {
			_, _ = connection.Write([]byte("pong"))
		}
	}))
	defer upstream.Close()
	server := httptest.NewServer(newReadyTestHandler(t, upstream))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	connection, err := net.DialTimeout("tcp", serverURL.Host, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = fmt.Fprintf(connection, "GET /socket HTTP/1.1\r\nHost: coder.example.com\r\nConnection: Upgrade\r\nUpgrade: coder-test\r\n\r\n")
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", response.StatusCode)
	}
	_, _ = connection.Write([]byte("ping"))
	reply := make([]byte, 4)
	if _, err := io.ReadFull(reader, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "pong" {
		t.Fatalf("reply = %q, want pong", reply)
	}
}

// TestHandlerFailsClosedWhenVersionGateIsUnready verifies requests stop when version readiness is lost.
func TestHandlerFailsClosedWhenVersionGateIsUnready(t *testing.T) {
	upstreamURL, _ := url.Parse("http://127.0.0.1:1")
	client := coder.NewClient(upstreamURL, &http.Client{Timeout: time.Second})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate := versiongate.New(client, "v2.35.", time.Minute, logger)
	publicURL, _ := url.Parse("https://coder.example.com")
	handler := NewHandler(
		NewHostClassifier(publicURL, "*.apps.example.com", []string{"coder-rbac"}),
		NewPolicy(client, nil, 1<<20),
		gate,
		newTestReverseProxy(upstreamURL),
		logger,
		NewMetrics(prometheus.NewRegistry()),
	)

	request := httptest.NewRequest(http.MethodGet, "https://coder.example.com/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

// newReadyTestHandler creates a handler backed by a version-compatible test server.
func newReadyTestHandler(t *testing.T, upstream *httptest.Server) http.Handler {
	t.Helper()
	upstreamURL, _ := url.Parse(upstream.URL)
	client := coder.NewClient(upstreamURL, upstream.Client())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate := versiongate.New(client, "v2.35.", time.Minute, logger)
	if err := gate.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	publicURL, _ := url.Parse("https://coder.example.com")
	reverseProxy := newTestReverseProxy(upstreamURL)
	return NewHandler(
		NewHostClassifier(publicURL, "*.apps.example.com", []string{"coder-rbac.coder.svc.cluster.local:8080"}),
		NewPolicy(client, nil, 1<<20),
		gate,
		reverseProxy,
		logger,
		NewMetrics(prometheus.NewRegistry()),
	)
}

// newTestReverseProxy creates a Rewrite-based proxy that preserves the original Coder host.
func newTestReverseProxy(upstreamURL *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(proxyRequest *httputil.ProxyRequest) {
			proxyRequest.SetURL(upstreamURL)
			proxyRequest.Out.Host = proxyRequest.In.Host
			proxyRequest.SetXForwarded()
		},
	}
}
