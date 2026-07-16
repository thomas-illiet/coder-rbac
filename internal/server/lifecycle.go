package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

// namedServer associates a listener name with its HTTP server.
type namedServer struct {
	name   string
	server *http.Server
}

// run performs the initial version check and owns all runtime resource cleanup.
func (r *runtime) run(ctx context.Context) error {
	if r.transport != nil {
		defer r.transport.CloseIdleConnections()
	}
	if err := r.checkInitialVersion(ctx); err != nil {
		return err
	}
	return r.serve(ctx)
}

// checkInitialVersion verifies Coder compatibility before opening any listener.
func (r *runtime) checkInitialVersion(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, r.upstreamTimeout)
	err := r.gate.Check(checkCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("initial Coder version check: %w", err)
	}
	_, runningVersion, _ := r.gate.Ready()
	r.logger.Info("Coder version policy accepted", "version", runningVersion, "expected_prefix", r.expectedVersionPrefix)
	return nil
}

// serve runs the version gate and every HTTP server until cancellation or listener failure.
func (r *runtime) serve(ctx context.Context) error {
	runCtx, stop := context.WithCancel(ctx)
	gateDone := make(chan struct{})
	go func() {
		r.gate.Run(runCtx)
		close(gateDone)
	}()

	errorsChannel := make(chan error, len(r.servers))
	var listeners sync.WaitGroup
	for _, item := range r.servers {
		item := item
		listeners.Add(1)
		go func() {
			defer listeners.Done()
			if err := listen(item, r.logger); err != nil {
				select {
				case errorsChannel <- err:
				case <-runCtx.Done():
				}
			}
		}()
	}

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errorsChannel:
	}
	stop()
	shutdownErr := r.shutdown()
	listeners.Wait()
	<-gateDone
	if serveErr != nil {
		return serveErr
	}
	return shutdownErr
}

// shutdown gracefully stops every HTTP server within the shared shutdown deadline.
func (r *runtime) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()
	errorsByServer := make([]error, 0, len(r.servers))
	for _, item := range r.servers {
		errorsByServer = append(errorsByServer, item.server.Shutdown(shutdownCtx))
	}
	return errors.Join(errorsByServer...)
}

// listen serves one HTTP server and returns only unexpected listener failures.
func listen(item namedServer, logger *slog.Logger) error {
	logger.Info("server listening", "name", item.name, "address", item.server.Addr)
	if err := item.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s server: %w", item.name, err)
	}
	return nil
}
