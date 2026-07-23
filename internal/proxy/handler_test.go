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

	"coder-policy/internal/coder"
	"coder-policy/internal/versiongate"
)

// TestHandlerHostRoutingAndWildcardProtection verifies host policy and original Host forwarding.
func TestHandlerHostRoutingAndWildcardProtection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v2/buildinfo" {
			_, _ = response.Write([]byte(`{"version":"v2.35.2"}`))
			return
		}
		if request.URL.Path == "/api/v2/users/me" {
			response.WriteHeader(http.StatusUnauthorized)
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
		{name: "internal service", host: "coder-policy.coder.svc.cluster.local:8080", path: "/api/v2/workspaces", statusCode: http.StatusMisdirectedRequest},
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

// TestHandlerEnforcesUserAdmissionAcrossHosts verifies access and administrator policy ordering.
func TestHandlerEnforcesUserAdmissionAcrossHosts(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v2/buildinfo":
			_, _ = response.Write([]byte(`{"version":"v2.35.2"}`))
		case "/api/v2/users/me":
			switch request.Header.Get("Coder-Session-Token") {
			case "admin-token":
				_, _ = response.Write([]byte(`{"username":"admin"}`))
			case "allowed-token":
				_, _ = response.Write([]byte(`{"username":"developer"}`))
			case "blocked-token":
				_, _ = response.Write([]byte(`{"username":"blocked"}`))
			case "failure-token":
				response.WriteHeader(http.StatusInternalServerError)
			default:
				response.WriteHeader(http.StatusUnauthorized)
			}
		default:
			_, _ = response.Write([]byte(`{"forwarded":true}`))
		}
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	client := coder.NewClient(upstreamURL, upstream.Client())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate := versiongate.New(client, "v2.35.", time.Minute, logger)
	if err := gate.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	publicURL, _ := url.Parse("https://coder.example.com")
	handler := NewHandler(
		NewHostClassifier(publicURL, "*.apps.example.com"),
		NewPolicy(client, map[string]struct{}{"admin": {}}, map[string]struct{}{"developer": {}}, 1<<20),
		gate,
		newTestReverseProxy(upstreamURL),
		logger,
		NewMetrics(prometheus.NewRegistry()),
	)
	server := httptest.NewServer(handler)
	defer server.Close()

	tests := []struct {
		name   string
		host   string
		method string
		path   string
		token  string
		status int
		code   string
	}{
		{name: "anonymous login", host: "coder.example.com", method: http.MethodGet, path: "/login", status: http.StatusOK},
		{name: "allowed main", host: "coder.example.com", method: http.MethodGet, path: "/dashboard", token: "allowed-token", status: http.StatusOK},
		{name: "allowed remains restricted", host: "coder.example.com", method: http.MethodPatch, path: "/api/v2/workspaces/" + testWorkspaceID.String(), token: "allowed-token", status: http.StatusForbidden, code: "workspace_rename_forbidden"},
		{name: "allowed cannot update password", host: "coder.example.com", method: http.MethodPut, path: "/api/v2/users/me/password", token: "allowed-token", status: http.StatusForbidden, code: "user_password_update_forbidden"},
		{name: "allowed cannot update profile", host: "coder.example.com", method: http.MethodPut, path: "/api/v2/users/me/profile", token: "allowed-token", status: http.StatusForbidden, code: "user_profile_update_forbidden"},
		{name: "admin bypasses policy", host: "coder.example.com", method: http.MethodPatch, path: "/api/v2/workspaces/" + testWorkspaceID.String(), token: "admin-token", status: http.StatusOK},
		{name: "admin may reach profile update", host: "coder.example.com", method: http.MethodPut, path: "/api/v2/users/me/profile", token: "admin-token", status: http.StatusOK},
		{name: "blocked main", host: "coder.example.com", method: http.MethodGet, path: "/dashboard", token: "blocked-token", status: http.StatusForbidden, code: "user_access_forbidden"},
		{name: "blocked session may reopen login", host: "coder.example.com", method: http.MethodGet, path: "/login", token: "blocked-token", status: http.StatusOK},
		{name: "blocked session may submit another login", host: "coder.example.com", method: http.MethodPost, path: "/api/v2/users/login", token: "blocked-token", status: http.StatusOK},
		{name: "blocked session may load login assets", host: "coder.example.com", method: http.MethodGet, path: "/assets/index.js", token: "blocked-token", status: http.StatusOK},
		{name: "internal service rejected", host: "coder-policy:8080", method: http.MethodGet, path: "/api/v2/workspaces", token: "blocked-token", status: http.StatusMisdirectedRequest, code: "unknown_host"},
		{name: "allowed wildcard", host: "code--dev--alice.apps.example.com", method: http.MethodGet, path: "/", token: "allowed-token", status: http.StatusOK},
		{name: "blocked wildcard", host: "code--dev--alice.apps.example.com", method: http.MethodGet, path: "/", token: "blocked-token", status: http.StatusForbidden, code: "user_access_forbidden"},
		{name: "wildcard control path stays hidden", host: "code--dev--alice.apps.example.com", method: http.MethodGet, path: "/api/v2/users/me", token: "blocked-token", status: http.StatusNotFound},
		{name: "identity failure", host: "coder.example.com", method: http.MethodGet, path: "/dashboard", token: "failure-token", status: http.StatusServiceUnavailable, code: "identity_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(test.method, server.URL+test.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Host = test.host
			if test.token != "" {
				request.Header.Set("Coder-Session-Token", test.token)
			}
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, test.status, body)
			}
			if test.code != "" && !strings.Contains(string(body), `"code":"`+test.code+`"`) {
				t.Fatalf("body %q does not contain code %q", body, test.code)
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
		if request.URL.Path == "/api/v2/users/me" {
			response.WriteHeader(http.StatusUnauthorized)
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
		NewHostClassifier(publicURL, "*.apps.example.com"),
		NewPolicy(client, nil, nil, 1<<20),
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
		NewHostClassifier(publicURL, "*.apps.example.com"),
		NewPolicy(client, nil, nil, 1<<20),
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
