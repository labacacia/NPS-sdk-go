// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// Framework-agnostic inbound MCP/A2A Bridge server middleware. Implements
// http.Handler; mount at any path via a mux. Faithful port of the .NET
// BridgeServerMiddleware. Routes: {prefix}{McpPath}[/sse], {prefix}{A2aPath},
// {prefix}{A2aAgentCardPath}.

package nwp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// BridgeServerMiddleware exposes inbound MCP/A2A Bridge server adapters over HTTP.
type BridgeServerMiddleware struct {
	options *BridgeServerOptions
	mcp     *McpServerBridge
	a2a     *A2aServerBridge
	next    http.Handler
}

// NewBridgeServerMiddleware builds Bridge server middleware. next may be nil,
// in which case unmatched requests receive 404.
func NewBridgeServerMiddleware(options *BridgeServerOptions, next http.Handler) *BridgeServerMiddleware {
	if options == nil {
		options = NewBridgeServerOptions()
	}
	return &BridgeServerMiddleware{
		options: options,
		mcp:     NewMcpServerBridge(options),
		a2a:     NewA2aServerBridge(options),
		next:    next,
	}
}

type bridgeHTTPResult struct {
	status   int
	response *BridgeJSONRPCResponse
}

// ServeHTTP implements http.Handler.
func (m *BridgeServerMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	prefix := strings.TrimRight(m.options.PathPrefix, "/")

	if !strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix)) {
		m.serveNext(w, r)
		return
	}

	sub := path[len(prefix):]
	sseSuffix := bridgeAppendPath(m.options.McpPath, "/sse")
	if bridgePathMatches(sub, m.options.McpPath) || bridgePathMatches(sub, sseSuffix) {
		m.handleMcp(w, r, isSseRequest(r) || bridgePathMatches(sub, sseSuffix))
		return
	}
	if bridgePathMatches(sub, m.options.A2aPath) {
		m.handleA2a(w, r)
		return
	}
	if bridgePathMatches(sub, m.options.A2aAgentCardPath) {
		m.handleAgentCard(w, r)
		return
	}
	m.serveNext(w, r)
}

func (m *BridgeServerMiddleware) serveNext(w http.ResponseWriter, r *http.Request) {
	if m.next != nil {
		m.next.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func (m *BridgeServerMiddleware) handleMcp(w http.ResponseWriter, r *http.Request, useSse bool) {
	if r.Method == http.MethodGet && useSse {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "event: endpoint\ndata: "+
			bridgeJoinPath(m.options.PathPrefix, m.options.McpPath)+"\n\n")
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if ok, msg := m.authorize(r); !ok {
		m.writeJSONRPCError(w, 401, JSONRPCInvalidRequest, msg)
		return
	}

	result := m.readAndDispatch(r, m.mcp.Dispatch)
	if useSse {
		m.writeSse(w, result.response, result.status)
	} else {
		m.writeJSON(w, result.status, result.response)
	}
}

func (m *BridgeServerMiddleware) handleA2a(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if ok, msg := m.authorize(r); !ok {
		m.writeJSONRPCError(w, 401, JSONRPCInvalidRequest, msg)
		return
	}

	result := m.readAndDispatch(r, m.a2a.Dispatch)
	m.writeJSON(w, result.status, result.response)
}

func (m *BridgeServerMiddleware) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	endpoint := scheme + "://" + r.Host + bridgeJoinPath(m.options.PathPrefix, m.options.A2aPath)
	m.writeJSON(w, 200, m.a2a.BuildAgentCard(endpoint))
}

type bridgeDispatchFn func(ctx context.Context, req *BridgeJSONRPCRequest) *BridgeJSONRPCResponse

func (m *BridgeServerMiddleware) readAndDispatch(r *http.Request, dispatch bridgeDispatchFn) bridgeHTTPResult {
	req, err := m.readJSONRPCRequest(r)
	if err != nil {
		switch e := err.(type) {
		case *bridgePayloadTooLargeError:
			return bridgeHTTPResult{http.StatusRequestEntityTooLarge,
				bridgeJSONRPCError(nil, JSONRPCInvalidRequest, e.Error(), nil)}
		default:
			return bridgeHTTPResult{http.StatusBadRequest,
				bridgeJSONRPCError(nil, JSONRPCParseError, err.Error(), nil)}
		}
	}
	if req == nil {
		return bridgeHTTPResult{http.StatusBadRequest,
			bridgeJSONRPCError(nil, JSONRPCInvalidRequest, "JSON-RPC request is required.", nil)}
	}

	resp, timedOut := m.dispatchWithTimeout(r.Context(), req, dispatch)
	if timedOut {
		return bridgeHTTPResult{http.StatusGatewayTimeout,
			bridgeJSONRPCError(nil, JSONRPCUpstreamError,
				"Bridge server dispatch timed out.", nil)}
	}
	return bridgeHTTPResult{200, resp}
}

func (m *BridgeServerMiddleware) readJSONRPCRequest(r *http.Request) (*BridgeJSONRPCRequest, error) {
	maxBytes := m.options.MaxRequestBodyBytes
	if maxBytes > 0 && r.ContentLength > maxBytes {
		return nil, &bridgePayloadTooLargeError{maxBytes}
	}

	var reader io.Reader = r.Body
	if maxBytes > 0 {
		reader = io.LimitReader(r.Body, maxBytes+1)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(raw)) > maxBytes {
		return nil, &bridgePayloadTooLargeError{maxBytes}
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}

	var req BridgeJSONRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (m *BridgeServerMiddleware) dispatchWithTimeout(ctx context.Context, req *BridgeJSONRPCRequest, dispatch bridgeDispatchFn) (*BridgeJSONRPCResponse, bool) {
	if m.options.DispatchTimeoutMs == 0 {
		return dispatch(ctx, req), false
	}

	dctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan *BridgeJSONRPCResponse, 1)
	go func() { done <- dispatch(dctx, req) }()

	timer := time.NewTimer(time.Duration(m.options.DispatchTimeoutMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case resp := <-done:
		return resp, false
	case <-timer.C:
		return nil, true
	case <-ctx.Done():
		return nil, false
	}
}

func (m *BridgeServerMiddleware) authorize(r *http.Request) (bool, string) {
	if !m.options.RequireAuth {
		return true, ""
	}
	values := r.Header.Values(HeaderAgent)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return false, "A valid X-NWP-Agent NID is required."
	}
	agentNid := strings.TrimSpace(values[0])
	if !isValidAgentNid(agentNid) {
		return false, "A valid X-NWP-Agent NID is required."
	}
	if m.options.VerifyAgent == nil {
		return false, "Bridge server agent verifier is required."
	}
	if !m.options.VerifyAgent(agentNid, r) {
		return false, "X-NWP-Agent was rejected by Bridge server policy."
	}
	return true, ""
}

func isValidAgentNid(nid string) bool {
	const prefix = "urn:nps:agent:"
	if !strings.HasPrefix(nid, prefix) || len(nid) > 512 {
		return false
	}
	rest := nid[len(prefix):]
	sep := strings.IndexByte(rest, ':')
	if sep <= 0 || sep == len(rest)-1 {
		return false
	}
	domain := rest[:sep]
	identifier := rest[sep+1:]
	for _, ch := range domain {
		if !isNidDomainChar(ch) {
			return false
		}
	}
	for _, ch := range identifier {
		if !isNidIdentifierChar(ch) {
			return false
		}
	}
	return true
}

func isNidDomainChar(ch rune) bool {
	return isASCIILetterOrDigit(ch) || ch == '.' || ch == '-'
}

func isNidIdentifierChar(ch rune) bool {
	return isASCIILetterOrDigit(ch) ||
		ch == '.' || ch == '_' || ch == '-' || ch == '~' || ch == ':' || ch == '@' || ch == '/'
}

func (m *BridgeServerMiddleware) writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (m *BridgeServerMiddleware) writeJSONRPCError(w http.ResponseWriter, status, code int, message string) {
	m.writeJSON(w, status, bridgeJSONRPCError(nil, code, message, nil))
}

func (m *BridgeServerMiddleware) writeSse(w http.ResponseWriter, response *BridgeJSONRPCResponse, status int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(status)
	payload, _ := json.Marshal(response)
	_, _ = io.WriteString(w, "event: message\ndata: "+string(payload)+"\n\n")
}

func isSseRequest(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept") {
		if strings.Contains(strings.ToLower(v), "text/event-stream") {
			return true
		}
	}
	return false
}

func bridgePathMatches(actual, expected string) bool {
	normalized := expected
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	return strings.EqualFold(actual, normalized) || strings.EqualFold(actual, normalized+"/")
}

func bridgeAppendPath(path, suffix string) string {
	return strings.TrimRight(path, "/") + suffix
}

func bridgeJoinPath(prefix, path string) string {
	left := strings.TrimRight(prefix, "/")
	right := path
	if !strings.HasPrefix(right, "/") {
		right = "/" + right
	}
	if left == "" {
		return right
	}
	return left + right
}

type bridgePayloadTooLargeError struct{ maxBytes int64 }

func (e *bridgePayloadTooLargeError) Error() string {
	return "Bridge server request body exceeds the configured byte limit."
}
