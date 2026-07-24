// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Framework-agnostic outbound Bridge Node middleware. Implements http.Handler;
// mount at any path via a mux. Faithful port of the .NET BridgeNodeMiddleware.
// Routes: {prefix}/.nwm, {prefix}/actions, {prefix}/invoke.

package nwp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// BridgeNodeOptions configures the hosted outbound Bridge Node.
type BridgeNodeOptions struct {
	// NodeID is the Bridge Node identifier surfaced in /.nwm.
	NodeID string
	// PathPrefix is the path prefix for Bridge Node endpoints. Empty means root.
	PathPrefix string
	// ActionID is the action id accepted by /invoke.
	ActionID string
	// RequireAuth requires the X-NWP-Agent header before dispatching.
	RequireAuth bool
}

// NewBridgeNodeOptions returns options populated with the .NET defaults.
func NewBridgeNodeOptions() *BridgeNodeOptions {
	return &BridgeNodeOptions{
		NodeID:     "nps-bridge",
		PathPrefix: "",
		ActionID:   "bridge.dispatch",
	}
}

// BridgeNodeMiddleware exposes an outbound Bridge Node over HTTP.
type BridgeNodeMiddleware struct {
	bridge   *BridgeNode
	registry *BridgeDispatcherRegistry
	options  *BridgeNodeOptions
	next     http.Handler
}

// NewBridgeNodeMiddleware builds Bridge Node middleware. next may be nil, in
// which case unmatched requests receive 404.
func NewBridgeNodeMiddleware(bridge *BridgeNode, registry *BridgeDispatcherRegistry, options *BridgeNodeOptions, next http.Handler) *BridgeNodeMiddleware {
	if options == nil {
		options = NewBridgeNodeOptions()
	}
	return &BridgeNodeMiddleware{bridge: bridge, registry: registry, options: options, next: next}
}

// ServeHTTP implements http.Handler.
func (m *BridgeNodeMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := strings.TrimRight(m.options.PathPrefix, "/")

	if !strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix)) {
		m.serveNext(w, r)
		return
	}

	sub := path[len(prefix):]
	switch sub {
	case "/.nwm", "/.nwm/":
		m.writeJSON(w, 200, m.buildManifest(), MimeManifest)
	case "/actions", "/actions/":
		m.writeJSON(w, 200, m.buildActions(), "application/json")
	case "/invoke", "/invoke/":
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		m.handleInvoke(w, r)
	default:
		m.serveNext(w, r)
	}
}

func (m *BridgeNodeMiddleware) serveNext(w http.ResponseWriter, r *http.Request) {
	if m.next != nil {
		m.next.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func (m *BridgeNodeMiddleware) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if m.options.RequireAuth && r.Header.Get(HeaderAgent) == "" {
		m.writeError(w, 401, "NPS-CLIENT-UNAUTHORIZED",
			"NWP-BRIDGE-AUTH-REQUIRED", "X-NWP-Agent header is required.")
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		m.writeError(w, 400, "NPS-CLIENT-BAD-REQUEST", BridgeErrTargetInvalid, err.Error())
		return
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		m.writeError(w, 400, "NPS-CLIENT-BAD-REQUEST", BridgeErrTargetInvalid, "ActionFrame body is required.")
		return
	}

	var frame BridgeActionFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		m.writeError(w, 400, "NPS-CLIENT-BAD-REQUEST", BridgeErrTargetInvalid, err.Error())
		return
	}

	if frame.ActionID != m.options.ActionID {
		m.writeError(w, 404, "NPS-CLIENT-NOT-FOUND",
			"NWP-BRIDGE-ACTION-NOT-FOUND", "Unknown bridge action '"+frame.ActionID+"'.")
		return
	}

	caps, err := m.bridge.Dispatch(r.Context(), &frame)
	if err != nil {
		var de *BridgeDispatchError
		if errors.As(err, &de) {
			status := 400
			npsStatus := "NPS-CLIENT-BAD-REQUEST"
			if de.ErrorCode == BridgeErrUpstreamFailed {
				status = 502
				npsStatus = "NPS-SERVER-UPSTREAM-FAILED"
			}
			m.writeError(w, status, npsStatus, de.ErrorCode, de.Message)
			return
		}
		m.writeError(w, 500, "NPS-SERVER-ERROR", BridgeErrUpstreamFailed, err.Error())
		return
	}

	m.writeJSON(w, 200, caps.ToDict(), "application/json")
}

func (m *BridgeNodeMiddleware) buildManifest() map[string]interface{} {
	return map[string]interface{}{
		"node_type":        NodeTypeBridge,
		"node_id":          m.options.NodeID,
		"bridge_protocols": m.registry.Protocols(),
		"actions":          []string{m.options.ActionID},
	}
}

func (m *BridgeNodeMiddleware) buildActions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"action_id":        m.options.ActionID,
			"description":      "Dispatch an NWP ActionFrame to an external Bridge target.",
			"bridge_protocols": m.registry.Protocols(),
		},
	}
}

func (m *BridgeNodeMiddleware) writeJSON(w http.ResponseWriter, status int, body interface{}, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (m *BridgeNodeMiddleware) writeError(w http.ResponseWriter, status int, npsStatus, errorCode, message string) {
	frame := map[string]interface{}{
		"status":  npsStatus,
		"error":   errorCode,
		"message": message,
	}
	m.writeJSON(w, status, frame, "application/json")
}
