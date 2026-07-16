package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeVersionGate provides deterministic version-gate lifecycle behavior.
type fakeVersionGate struct {
	checkErr   error
	ready      bool
	version    string
	runStarted chan struct{}
	runStopped chan struct{}
}

// Check returns the configured initial-check result.
func (gate *fakeVersionGate) Check(context.Context) error {
	return gate.checkErr
}

// Run records its lifecycle and waits for context cancellation.
func (gate *fakeVersionGate) Run(ctx context.Context) {
	if gate.runStarted != nil {
		close(gate.runStarted)
	}
	<-ctx.Done()
	if gate.runStopped != nil {
		close(gate.runStopped)
	}
}

// Ready returns the configured readiness snapshot.
func (gate *fakeVersionGate) Ready() (bool, string, error) {
	return gate.ready, gate.version, gate.checkErr
}

// TestRuntimeStopsOnCancellation verifies cancellation stops the gate and returns cleanly.
func TestRuntimeStopsOnCancellation(t *testing.T) {
	gate := &fakeVersionGate{
		ready:      true,
		version:    "v2.35.7",
		runStarted: make(chan struct{}),
		runStopped: make(chan struct{}),
	}
	runtime := &runtime{
		gate:                  gate,
		transport:             newTransport(),
		upstreamTimeout:       time.Second,
		shutdownTimeout:       time.Second,
		expectedVersionPrefix: "v2.35.",
		logger:                serverTestLogger(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- runtime.run(ctx)
	}()

	select {
	case <-gate.runStarted:
	case <-time.After(time.Second):
		t.Fatal("version gate did not start")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runtime error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not stop")
	}
	select {
	case <-gate.runStopped:
	default:
		t.Fatal("version gate did not observe cancellation")
	}
}

// TestRuntimeRejectsInitialVersionFailure verifies listeners never start after a failed check.
func TestRuntimeRejectsInitialVersionFailure(t *testing.T) {
	gateErr := errors.New("version mismatch")
	gate := &fakeVersionGate{checkErr: gateErr, runStarted: make(chan struct{})}
	runtime := &runtime{
		gate:                  gate,
		transport:             newTransport(),
		upstreamTimeout:       time.Second,
		shutdownTimeout:       time.Second,
		expectedVersionPrefix: "v2.35.",
		logger:                serverTestLogger(),
	}

	err := runtime.run(context.Background())
	if !errors.Is(err, gateErr) || !strings.Contains(err.Error(), "initial Coder version check") {
		t.Fatalf("runtime error = %v", err)
	}
	select {
	case <-gate.runStarted:
		t.Fatal("version gate run loop started after failed initial check")
	default:
	}
}

// TestRuntimeReturnsListenerFailure verifies one failed listener stops the remaining services.
func TestRuntimeReturnsListenerFailure(t *testing.T) {
	gate := &fakeVersionGate{ready: true, version: "v2.35.7"}
	runtime := &runtime{
		gate:      gate,
		transport: newTransport(),
		servers: []namedServer{
			{name: "proxy", server: &http.Server{Addr: "not-an-address", Handler: http.NotFoundHandler()}},
			{name: "health", server: &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}},
		},
		upstreamTimeout:       time.Second,
		shutdownTimeout:       time.Second,
		expectedVersionPrefix: "v2.35.",
		logger:                serverTestLogger(),
	}

	err := runtime.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "proxy server") {
		t.Fatalf("runtime error = %v, want proxy listener failure", err)
	}
}
