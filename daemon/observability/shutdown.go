// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Graceful shutdown coordination for NPS daemons. Ported from the .NET
// reference NPS.Daemon.Observability/Shutdown/GracefulShutdown.cs. Provides a
// SIGTERM/SIGINT-aware liveness gate (ShutdownState) and a coordinator that
// drains registered http.Servers within a fixed timeout, logging the signal
// arrival and completion.

package observability

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// DefaultDrainTimeout is the default drain window for NPS daemons (NPS-Dev #45).
const DefaultDrainTimeout = 30 * time.Second

// ShutdownState is a liveness flag flipped on SIGTERM; read by health probes so
// /healthz starts failing the moment a drain begins.
type ShutdownState struct {
	stopping int32
	mu       sync.Mutex
}

// IsStopping reports whether shutdown has begun.
func (s *ShutdownState) IsStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping == 1
}

// MarkStopping flips the liveness gate.
func (s *ShutdownState) MarkStopping() {
	s.mu.Lock()
	s.stopping = 1
	s.mu.Unlock()
}

// GracefulShutdown coordinates draining a set of http.Servers on SIGTERM/SIGINT
// within DrainTimeout, flipping the ShutdownState so probes fail early.
type GracefulShutdown struct {
	State        *ShutdownState
	DrainTimeout time.Duration
	Logger       *slog.Logger

	mu      sync.Mutex
	servers []*http.Server
}

// NewGracefulShutdown builds a coordinator with the default drain timeout.
// A nil logger disables logging.
func NewGracefulShutdown(logger *slog.Logger) *GracefulShutdown {
	return &GracefulShutdown{
		State:        &ShutdownState{},
		DrainTimeout: DefaultDrainTimeout,
		Logger:       logger,
	}
}

// Register adds an http.Server to be gracefully shut down on signal.
func (g *GracefulShutdown) Register(srv *http.Server) {
	g.mu.Lock()
	g.servers = append(g.servers, srv)
	g.mu.Unlock()
}

// Run blocks until a SIGTERM/SIGINT arrives or ctx is cancelled, then drains all
// registered servers within DrainTimeout. It flips the liveness gate on signal
// and logs signal arrival and completion. Returns the shutdown error, if any.
func (g *GracefulShutdown) Run(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case <-ctx.Done():
	case <-sigCh:
	}
	return g.Shutdown()
}

// Shutdown flips the liveness gate and drains all registered servers within the
// drain timeout. Safe to call directly (e.g. from tests) without a signal.
func (g *GracefulShutdown) Shutdown() error {
	g.State.MarkStopping()

	timeout := g.DrainTimeout
	if timeout <= 0 {
		timeout = DefaultDrainTimeout
	}
	if g.Logger != nil {
		g.Logger.Info("shutdown signal received; draining",
			slog.Int("timeout_s", int(timeout/time.Second)))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	g.mu.Lock()
	servers := append([]*http.Server(nil), g.servers...)
	g.mu.Unlock()

	var firstErr error
	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if g.Logger != nil {
		g.Logger.Info("shutdown complete")
	}
	return firstErr
}
