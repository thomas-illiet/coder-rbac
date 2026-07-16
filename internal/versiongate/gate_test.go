package versiongate

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/thomas-illiet/coder-rbac/internal/coder"
)

// buildInfoClientFunc adapts a function to the buildInfoClient interface.
type buildInfoClientFunc func(context.Context) (coder.BuildInfo, error)

// BuildInfo invokes the adapted build-information function.
func (function buildInfoClientFunc) BuildInfo(ctx context.Context) (coder.BuildInfo, error) {
	return function(ctx)
}

// TestGateTracksVersionAndDependencyTransitions verifies every check replaces the readiness snapshot.
func TestGateTracksVersionAndDependencyTransitions(t *testing.T) {
	dependencyErr := errors.New("Coder is unavailable")
	responses := []struct {
		info coder.BuildInfo
		err  error
	}{
		{info: coder.BuildInfo{Version: "v2.35.2"}},
		{info: coder.BuildInfo{Version: "v2.36.0"}},
		{err: dependencyErr},
		{info: coder.BuildInfo{Version: "v2.35.9"}},
	}
	call := 0
	client := buildInfoClientFunc(func(context.Context) (coder.BuildInfo, error) {
		response := responses[call]
		call++
		return response.info, response.err
	})
	gate := New(client, "v2.35.", time.Minute, testLogger())

	if err := gate.Check(context.Background()); err != nil {
		t.Fatalf("matching version: %v", err)
	}
	assertGateState(t, gate, true, "v2.35.2", nil)

	err := gate.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), `does not match required prefix "v2.35."`) {
		t.Fatalf("mismatched version error = %v", err)
	}
	assertGateState(t, gate, false, "v2.36.0", err)

	err = gate.Check(context.Background())
	if !errors.Is(err, dependencyErr) {
		t.Fatalf("dependency error = %v, want %v", err, dependencyErr)
	}
	assertGateState(t, gate, false, "", dependencyErr)

	if err := gate.Check(context.Background()); err != nil {
		t.Fatalf("recovered version: %v", err)
	}
	assertGateState(t, gate, true, "v2.35.9", nil)
}

// TestGateRunPropagatesCancellation verifies cancellation interrupts an in-flight periodic check.
func TestGateRunPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	checkResult := make(chan error, 1)
	client := buildInfoClientFunc(func(ctx context.Context) (coder.BuildInfo, error) {
		close(started)
		<-ctx.Done()
		checkResult <- ctx.Err()
		return coder.BuildInfo{}, ctx.Err()
	})
	gate := New(client, "v2.35.", time.Millisecond, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		gate.Run(ctx)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("periodic check did not start")
	}
	cancel()

	select {
	case err := <-checkResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("check context error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight check did not observe cancellation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("gate run loop did not stop")
	}

	ready, version, err := gate.Ready()
	if ready || version != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("ready = %v, version = %q, error = %v", ready, version, err)
	}
}

// assertGateState compares a gate snapshot with the expected state.
func assertGateState(t *testing.T, gate *Gate, wantReady bool, wantVersion string, wantErr error) {
	t.Helper()
	ready, version, err := gate.Ready()
	if ready != wantReady || version != wantVersion || !errors.Is(err, wantErr) {
		t.Fatalf("ready = %v, version = %q, error = %v; want %v, %q, %v", ready, version, err, wantReady, wantVersion, wantErr)
	}
}

// testLogger returns a logger that discards test output.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
