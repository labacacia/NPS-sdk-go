// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Health ────────────────────────────────────────────────────────────────────

func TestHealthz_OK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	HealthzHandler(nil).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != HealthJSONContentType {
		t.Fatalf("ct=%q", rec.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(rec.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHealthz_Draining_503(t *testing.T) {
	state := &ShutdownState{}
	state.MarkStopping()
	rec := httptest.NewRecorder()
	HealthzHandler(state).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 503 {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestReadyz_NoProbes_OK(t *testing.T) {
	rec := httptest.NewRecorder()
	ReadyzHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestReadyz_FailingProbe_503_WithReason(t *testing.T) {
	probe := NewDelegateReadinessProbe("storage", func(ctx context.Context) (string, error) {
		return "db down", nil
	})
	rec := httptest.NewRecorder()
	ReadyzHandler(probe).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != 503 {
		t.Fatalf("code=%d", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["status"] != "error" || body["reason"] != "db down" {
		t.Fatalf("body=%v", body)
	}
}

func TestReadyz_ProbeError_IncludesName(t *testing.T) {
	probe := NewDelegateReadinessProbe("keys", func(ctx context.Context) (string, error) {
		return "", errors.New("boom")
	})
	rec := httptest.NewRecorder()
	ReadyzHandler(probe).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), "keys: boom") {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func TestMetrics_Exposition(t *testing.T) {
	reg := NewMetricsRegistry()
	c := reg.RegisterCounter("nps_frames_total", "frames", "kind")
	c.Inc("query")
	c.Add(2, "stream")
	g := reg.RegisterGauge("nps_inflight", "inflight")
	g.Set(3)

	out := reg.Render()
	if !strings.Contains(out, "# TYPE nps_frames_total counter") {
		t.Fatalf("no counter type: %q", out)
	}
	if !strings.Contains(out, `nps_frames_total{kind="query"} 1`) {
		t.Fatalf("no query cell: %q", out)
	}
	if !strings.Contains(out, `nps_frames_total{kind="stream"} 2`) {
		t.Fatalf("no stream cell: %q", out)
	}
	if !strings.Contains(out, "# TYPE nps_inflight gauge") || !strings.Contains(out, "nps_inflight 3") {
		t.Fatalf("no gauge: %q", out)
	}
}

func TestMetricsHandler_ContentType(t *testing.T) {
	reg := NewMetricsRegistry()
	rec := httptest.NewRecorder()
	MetricsHandler(reg, "", false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Header().Get("Content-Type") != MetricsContentType {
		t.Fatalf("ct=%q", rec.Header().Get("Content-Type"))
	}
}

func TestMetricsHandler_BearerAuth(t *testing.T) {
	reg := NewMetricsRegistry()
	h := MetricsHandler(reg, "secret", false)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != 401 {
		t.Fatalf("no-token code=%d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("token code=%d", rec.Code)
	}
}

// ── Logging ───────────────────────────────────────────────────────────────────

func TestJSONLogger_Fields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONLogger(&buf, slog.LevelInfo)
	logger.Info("hello", slog.String("k", "v"))

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("not json: %v — %q", err, buf.String())
	}
	if rec["msg"] != "hello" || rec["level"] != "info" || rec["k"] != "v" {
		t.Fatalf("rec=%v", rec)
	}
	if _, ok := rec["timestamp"]; !ok {
		t.Fatalf("no timestamp: %v", rec)
	}
}

func TestResolveLogLevel_EnvNames(t *testing.T) {
	t.Setenv(LogLevelEnvVar, "warning")
	if ResolveLogLevel(slog.LevelInfo) != slog.LevelWarn {
		t.Fatal("warning → warn")
	}
	t.Setenv(LogLevelEnvVar, "debug")
	if ResolveLogLevel(slog.LevelInfo) != slog.LevelDebug {
		t.Fatal("debug")
	}
	t.Setenv(LogLevelEnvVar, "")
	if ResolveLogLevel(slog.LevelError) != slog.LevelError {
		t.Fatal("fallback")
	}
}

// ── Shutdown ──────────────────────────────────────────────────────────────────

func TestGracefulShutdown_DrainsServer(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	gs := NewGracefulShutdown(nil)
	gs.DrainTimeout = 2 * time.Second
	gs.Register(srv)

	if gs.State.IsStopping() {
		t.Fatal("should not be stopping yet")
	}
	if err := gs.Shutdown(); err != nil {
		t.Fatalf("shutdown err=%v", err)
	}
	if !gs.State.IsStopping() {
		t.Fatal("should be stopping after shutdown")
	}
}

func TestGracefulShutdown_RunRespectsContext(t *testing.T) {
	gs := NewGracefulShutdown(nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- gs.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
	if !gs.State.IsStopping() {
		t.Fatal("should be stopping")
	}
}
