// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rawParams(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return b
}

func record(t *testing.T, caps interface{}) map[string]interface{} {
	t.Helper()
	type capser interface{ ToDict() map[string]any }
	c, ok := caps.(capser)
	if !ok {
		t.Fatalf("not a caps frame: %T", caps)
	}
	// Round-trip through JSON so the record reflects on-the-wire decoding
	// (numbers become float64, raw messages become objects).
	wire, err := json.Marshal(c.ToDict())
	if err != nil {
		t.Fatalf("marshal caps: %v", err)
	}
	var d map[string]interface{}
	if err := json.Unmarshal(wire, &d); err != nil {
		t.Fatalf("unmarshal caps: %v", err)
	}
	data, ok := d["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatalf("caps has no data: %+v", d)
	}
	rec, ok := data[0].(map[string]interface{})
	if !ok {
		t.Fatalf("record is not an object: %T", data[0])
	}
	return rec
}

// ── Target parsing ────────────────────────────────────────────────────────────

func TestBridgeTargetFromActionFrame_Direct(t *testing.T) {
	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{
		"protocol": "http",
		"endpoint": "https://example.com/api",
		"method":   "GET",
	})}
	target, err := BridgeTargetFromActionFrame(frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if target.Protocol != "http" || target.Endpoint != "https://example.com/api" {
		t.Fatalf("unexpected target: %+v", target)
	}
	if got := bridgeTargetString(target, "method", "POST"); got != "GET" {
		t.Fatalf("expected extras method GET, got %q", got)
	}
}

func TestBridgeTargetFromActionFrame_Nested(t *testing.T) {
	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{
		"bridge_target": map[string]interface{}{
			"protocol": "mcp",
			"endpoint": "https://mcp.example.com",
			"extras":   map[string]interface{}{"rpc_method": "tools/call"},
		},
		"other": "ignored",
	})}
	target, err := BridgeTargetFromActionFrame(frame)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if target.Protocol != "mcp" {
		t.Fatalf("expected mcp, got %q", target.Protocol)
	}
	if got := bridgeTargetString(target, "rpc_method", ""); got != "tools/call" {
		t.Fatalf("expected extras from 'extras' object, got %q", got)
	}
}

func TestBridgeTargetFromActionFrame_MissingProtocol(t *testing.T) {
	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{
		"endpoint": "https://example.com",
	})}
	_, err := BridgeTargetFromActionFrame(frame)
	assertBridgeError(t, err, BridgeErrTargetInvalid)
}

func TestBridgeTargetFromActionFrame_NoParams(t *testing.T) {
	_, err := BridgeTargetFromActionFrame(&BridgeActionFrame{})
	assertBridgeError(t, err, BridgeErrTargetInvalid)
}

// ── Endpoint SSRF / validation ────────────────────────────────────────────────

func TestEndpointValidator_RejectsPrivateHost(t *testing.T) {
	target := &BridgeTarget{Protocol: "http", Endpoint: "http://127.0.0.1:8080/x"}
	_, err := bridgeParseHTTPEndpoint(target)
	assertBridgeError(t, err, BridgeErrEndpointInvalid)
}

func TestEndpointValidator_RejectsNonHTTPScheme(t *testing.T) {
	target := &BridgeTarget{Protocol: "http", Endpoint: "ftp://example.com/x"}
	_, err := bridgeParseHTTPEndpoint(target)
	assertBridgeError(t, err, BridgeErrEndpointInvalid)
}

func TestEndpointValidator_AllowsPrivateWhenRejectDisabled(t *testing.T) {
	target := &BridgeTarget{
		Protocol: "http",
		Endpoint: "http://127.0.0.1:8080/x",
		Extras:   map[string]interface{}{"reject_private": json.RawMessage("false")},
	}
	if _, err := bridgeParseHTTPEndpoint(target); err != nil {
		t.Fatalf("expected private host allowed, got %v", err)
	}
}

func TestEndpointValidator_AllowedPrefixMismatch(t *testing.T) {
	target := &BridgeTarget{
		Protocol: "http",
		Endpoint: "https://evil.example.com/x",
		Extras: map[string]interface{}{
			"allowed_prefixes": json.RawMessage(`["https://good.example.com/"]`),
		},
	}
	_, err := bridgeParseHTTPEndpoint(target)
	assertBridgeError(t, err, BridgeErrEndpointInvalid)
}

// ── Registry ──────────────────────────────────────────────────────────────────

func TestRegistry_ProtocolUnsupported(t *testing.T) {
	reg := NewDefaultBridgeDispatcherRegistry(http.DefaultClient)
	_, err := reg.Resolve("smtp")
	assertBridgeError(t, err, BridgeErrProtocolUnsupported)
}

func TestRegistry_ResolveEmptyProtocol(t *testing.T) {
	reg := NewBridgeDispatcherRegistry()
	_, err := reg.Resolve("")
	assertBridgeError(t, err, BridgeErrTargetInvalid)
}

func TestRegistry_DefaultProtocols(t *testing.T) {
	reg := NewDefaultBridgeDispatcherRegistry(http.DefaultClient)
	got := strings.Join(reg.Protocols(), ",")
	if got != "a2a,grpc,http,mcp" {
		t.Fatalf("unexpected protocols: %s", got)
	}
}

// ── HTTP dispatcher ───────────────────────────────────────────────────────────

func TestHTTPBridgeDispatcher_MapsResponse(t *testing.T) {
	var gotBody string
	var gotMethod, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-Custom")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"ok":true,"echo":"hi"}`)
	}))
	defer srv.Close()

	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{
		"body": map[string]interface{}{"hello": "world"},
	})}
	target := &BridgeTarget{
		Protocol: "http",
		Endpoint: srv.URL + "/api",
		Extras: map[string]interface{}{
			"method":         json.RawMessage(`"POST"`),
			"headers":        json.RawMessage(`{"X-Custom":"abc"}`),
			"reject_private": json.RawMessage(`false`),
		},
	}

	d := NewHTTPBridgeDispatcher(srv.Client())
	caps, err := d.Dispatch(context.Background(), frame, target)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if gotMethod != "POST" {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotHeader != "abc" {
		t.Fatalf("expected custom header, got %q", gotHeader)
	}
	if !strings.Contains(gotBody, `"hello":"world"`) {
		t.Fatalf("unexpected upstream body: %s", gotBody)
	}

	rec := record(t, caps)
	if rec["status_code"].(float64) != 201 {
		t.Fatalf("expected 201, got %v", rec["status_code"])
	}
	if rec["success"].(bool) != true {
		t.Fatalf("expected success")
	}
	body, ok := rec["body"].(map[string]interface{})
	if !ok || body["ok"] != true {
		t.Fatalf("expected parsed JSON body, got %v", rec["body"])
	}
	if caps.AnchorRef == nil || *caps.AnchorRef != HTTPBridgeResponseAnchorRef {
		t.Fatalf("unexpected anchor ref: %v", caps.AnchorRef)
	}
}

func TestHTTPBridgeDispatcher_NonJSONBodyText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "plain text")
	}))
	defer srv.Close()

	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{})}
	target := &BridgeTarget{Protocol: "http", Endpoint: srv.URL, Extras: map[string]interface{}{
		"method":         json.RawMessage(`"GET"`),
		"reject_private": json.RawMessage(`false`),
	}}
	caps, err := NewHTTPBridgeDispatcher(srv.Client()).Dispatch(context.Background(), frame, target)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	rec := record(t, caps)
	if rec["body_text"] != "plain text" {
		t.Fatalf("expected body_text, got %v", rec)
	}
}

// ── gRPC-JSON dispatcher ──────────────────────────────────────────────────────

func TestGRPCBridgeDispatcher_MapsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/grpc+json" {
			t.Errorf("expected grpc+json content type, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		// Verify the 5-byte gRPC frame prefix on the request.
		if len(body) < 5 || body[0] != 0 {
			t.Errorf("malformed grpc request frame: %v", body)
		}
		msg := []byte(`{"result":42}`)
		frame := make([]byte, len(msg)+5)
		frame[0] = 0
		frame[1] = 0
		frame[2] = 0
		frame[3] = 0
		frame[4] = byte(len(msg))
		copy(frame[5:], msg)
		w.Header().Set("Content-Type", "application/grpc+json")
		w.Header().Set("grpc-status", "0")
		w.WriteHeader(200)
		_, _ = w.Write(frame)
	}))
	defer srv.Close()

	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{"x": 1})}
	target := &BridgeTarget{Protocol: "grpc", Endpoint: srv.URL + "/pkg.Svc/Method",
		Extras: map[string]interface{}{"reject_private": json.RawMessage(`false`)}}
	caps, err := NewGRPCBridgeDispatcher(srv.Client()).Dispatch(context.Background(), frame, target)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	rec := record(t, caps)
	if rec["grpc_status"] != "0" {
		t.Fatalf("expected grpc_status 0, got %v", rec["grpc_status"])
	}
	if rec["success"] != true {
		t.Fatalf("expected success")
	}
	messages, ok := rec["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Fatalf("expected 1 message, got %v", rec["messages"])
	}
	msg := messages[0].(map[string]interface{})
	if msg["result"].(float64) != 42 {
		t.Fatalf("unexpected message: %v", msg)
	}
}

// ── JSON-RPC (MCP/A2A) dispatchers ────────────────────────────────────────────

func TestMCPBridgeDispatcher_RequestShapeAndMapping(t *testing.T) {
	var reqEnvelope map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &reqEnvelope)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"ok"}]}}`)
	}))
	defer srv.Close()

	frame := &BridgeActionFrame{
		RequestID: "req-1",
		Params: rawParams(t, map[string]interface{}{
			"rpc_params": map[string]interface{}{"name": "search", "arguments": map[string]interface{}{"q": "x"}},
		}),
	}
	target := &BridgeTarget{Protocol: "mcp", Endpoint: srv.URL + "/mcp",
		Extras: map[string]interface{}{"reject_private": json.RawMessage(`false`)}}
	caps, err := NewMCPBridgeDispatcher(srv.Client()).Dispatch(context.Background(), frame, target)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if reqEnvelope["jsonrpc"] != "2.0" {
		t.Fatalf("expected jsonrpc 2.0")
	}
	if reqEnvelope["method"] != "tools/call" {
		t.Fatalf("expected default method tools/call, got %v", reqEnvelope["method"])
	}
	if reqEnvelope["id"] != "req-1" {
		t.Fatalf("expected id req-1, got %v", reqEnvelope["id"])
	}
	params := reqEnvelope["params"].(map[string]interface{})
	if params["name"] != "search" {
		t.Fatalf("expected rpc_params carried through, got %v", params)
	}

	rec := record(t, caps)
	if _, ok := rec["result"]; !ok {
		t.Fatalf("expected result extracted, got %v", rec)
	}
	if _, ok := rec["jsonrpc_response"]; !ok {
		t.Fatalf("expected jsonrpc_response, got %v", rec)
	}
}

func TestA2ABridgeDispatcher_DefaultMethod(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var env map[string]interface{}
		_ = json.Unmarshal(b, &env)
		method, _ = env["method"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"1","result":{}}`)
	}))
	defer srv.Close()

	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{"foo": "bar"})}
	target := &BridgeTarget{Protocol: "a2a", Endpoint: srv.URL + "/a2a",
		Extras: map[string]interface{}{"reject_private": json.RawMessage(`false`)}}
	if _, err := NewA2ABridgeDispatcher(srv.Client()).Dispatch(context.Background(), frame, target); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if method != "tasks/send" {
		t.Fatalf("expected default a2a method tasks/send, got %q", method)
	}
}

func TestBridgeNode_EndToEndHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	node := NewBridgeNode(NewDefaultBridgeDispatcherRegistry(srv.Client()))
	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{
		"protocol":       "http",
		"endpoint":       srv.URL,
		"method":         "GET",
		"reject_private": false,
	})}
	caps, err := node.Dispatch(context.Background(), frame)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	rec := record(t, caps)
	if rec["success"] != true {
		t.Fatalf("expected success")
	}
}

func TestBridgeNode_ProtocolUnsupported(t *testing.T) {
	node := NewBridgeNode(NewDefaultBridgeDispatcherRegistry(http.DefaultClient))
	frame := &BridgeActionFrame{Params: rawParams(t, map[string]interface{}{
		"protocol": "smtp",
		"endpoint": "https://example.com",
	})}
	_, err := node.Dispatch(context.Background(), frame)
	assertBridgeError(t, err, BridgeErrProtocolUnsupported)
}

func assertBridgeError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s, got nil", code)
	}
	de, ok := err.(*BridgeDispatchError)
	if !ok {
		t.Fatalf("expected *BridgeDispatchError, got %T: %v", err, err)
	}
	if de.ErrorCode != code {
		t.Fatalf("expected error code %s, got %s", code, de.ErrorCode)
	}
}
