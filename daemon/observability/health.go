// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Health / readiness probes and HTTP handlers. Ported from the .NET reference
// NPS.Daemon.Observability/HealthChecks (HealthProbeRenderer.cs +
// HealthEndpoints.cs). Response bodies, status codes, and content type match
// the .NET reference exactly.

package observability

import (
	"context"
	"encoding/json"
	"net/http"
)

// HealthJSONContentType matches HealthProbeRenderer.JsonContentType.
const HealthJSONContentType = "application/json; charset=utf-8"

// ReadinessProbe is one readiness check; daemons register one per backing
// dependency. Check returns "" on success or a short reason on failure.
type ReadinessProbe interface {
	// Name is the short name used in the JSON response (e.g. "storage").
	Name() string
	// Check returns "" on success, a short reason on failure.
	Check(ctx context.Context) (string, error)
}

// DelegateReadinessProbe wraps a func as a ReadinessProbe.
type DelegateReadinessProbe struct {
	name  string
	check func(ctx context.Context) (string, error)
}

// NewDelegateReadinessProbe builds a probe from a name and check func.
func NewDelegateReadinessProbe(name string, check func(ctx context.Context) (string, error)) *DelegateReadinessProbe {
	return &DelegateReadinessProbe{name: name, check: check}
}

// Name returns the probe name.
func (p *DelegateReadinessProbe) Name() string { return p.name }

// Check runs the wrapped function.
func (p *DelegateReadinessProbe) Check(ctx context.Context) (string, error) { return p.check(ctx) }

// HealthProbeResponse is a transport-neutral health/readiness response.
type HealthProbeResponse struct {
	StatusCode  int
	ContentType string
	Body        string
	Status      string
	Reason      string
}

// RenderHealthz renders the liveness response used by /healthz.
func RenderHealthz() HealthProbeResponse { return healthOK() }

// RenderReadyz runs the supplied probes and renders the readiness response used
// by /readyz. With no probes, readiness is "ok". The first failing probe's
// reason produces a 503.
func RenderReadyz(ctx context.Context, probes []ReadinessProbe) HealthProbeResponse {
	for _, probe := range probes {
		reason, err := probe.Check(ctx)
		if err != nil {
			return healthError(probe.Name() + ": " + err.Error())
		}
		if reason != "" {
			return healthError(reason)
		}
	}
	return healthOK()
}

func healthOK() HealthProbeResponse {
	body, _ := json.Marshal(map[string]string{"status": "ok"})
	return HealthProbeResponse{
		StatusCode:  http.StatusOK,
		ContentType: HealthJSONContentType,
		Body:        string(body),
		Status:      "ok",
	}
}

func healthError(reason string) HealthProbeResponse {
	body, _ := json.Marshal(map[string]string{"status": "error", "reason": reason})
	return HealthProbeResponse{
		StatusCode:  http.StatusServiceUnavailable,
		ContentType: HealthJSONContentType,
		Body:        string(body),
		Status:      "error",
		Reason:      reason,
	}
}

// HealthzHandler serves GET /healthz. When a ShutdownState is supplied and it is
// stopping, the probe fails early (503) so load balancers stop routing.
func HealthzHandler(state *ShutdownState) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if state != nil && state.IsStopping() {
			write(w, healthError("shutting_down"))
			return
		}
		write(w, RenderHealthz())
	})
}

// ReadyzHandler serves GET /readyz, walking the supplied probes.
func ReadyzHandler(probes ...ReadinessProbe) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write(w, RenderReadyz(r.Context(), probes))
	})
}

func write(w http.ResponseWriter, resp HealthProbeResponse) {
	w.Header().Set("Content-Type", resp.ContentType)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write([]byte(resp.Body))
}
