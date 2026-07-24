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

func echoOptions() *BridgeServerOptions {
	o := NewBridgeServerOptions()
	o.RequireAuth = false
	o.AddAction(BridgeServerAction{
		ActionID:    "search.web",
		Description: "Search the web.",
	})
	o.Dispatch = func(ctx context.Context, frame *BridgeActionFrame) (bridgeServerFrame, error) {
		payload, _ := json.Marshal(map[string]interface{}{
			"action":  frame.ActionID,
			"params":  json.RawMessage(nonNil(frame.Params)),
			"request": frame.RequestID,
		})
		return CapsResult(payload), nil
	}
	return o
}

func nonNil(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("null")
	}
	return r
}

// ── MCP inbound ───────────────────────────────────────────────────────────────

func TestMcpServerBridge_ToolsCallDispatchesLocalAction(t *testing.T) {
	b := NewMcpServerBridge(echoOptions())
	req := &BridgeJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"1"`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"search_web","arguments":{"q":"nps"}}`),
	}
	resp := b.Dispatch(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var result mcpToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error tool result")
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, `"action":"search.web"`) {
		t.Fatalf("unexpected content: %+v", result.Content)
	}
	if !strings.Contains(result.Content[0].Text, `"q":"nps"`) {
		t.Fatalf("arguments not passed through: %s", result.Content[0].Text)
	}
}

func TestMcpServerBridge_ToolNotFound(t *testing.T) {
	b := NewMcpServerBridge(echoOptions())
	req := &BridgeJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"does_not_exist","arguments":{}}`),
	}
	resp := b.Dispatch(context.Background(), req)
	if resp.Error == nil {
		t.Fatalf("expected error response")
	}
	if resp.Error.Code != JSONRPCToolNotFound {
		t.Fatalf("expected tool-not-found code, got %d", resp.Error.Code)
	}
	if !strings.Contains(string(resp.Error.Data), BridgeErrServerToolNotFound) {
		t.Fatalf("expected server-tool-not-found in data, got %s", resp.Error.Data)
	}
}

func TestMcpServerBridge_ToolsList(t *testing.T) {
	b := NewMcpServerBridge(echoOptions())
	resp := b.Dispatch(context.Background(), &BridgeJSONRPCRequest{Method: "tools/list", ID: json.RawMessage(`1`)})
	var list mcpToolListResult
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "search_web" {
		t.Fatalf("unexpected tools: %+v", list.Tools)
	}
}

func TestMcpServerBridge_DispatcherMissing(t *testing.T) {
	o := NewBridgeServerOptions()
	o.RequireAuth = false
	o.AddAction(BridgeServerAction{ActionID: "x.y"})
	b := NewMcpServerBridge(o)
	req := &BridgeJSONRPCRequest{
		Method: "tools/call",
		ID:     json.RawMessage(`1`),
		Params: json.RawMessage(`{"name":"x_y"}`),
	}
	resp := b.Dispatch(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("expected success wrapper, got error %+v", resp.Error)
	}
	var result mcpToolCallResult
	_ = json.Unmarshal(resp.Result, &result)
	if !result.IsError || !strings.Contains(result.Content[0].Text, BridgeErrServerDispatcherMissing) {
		t.Fatalf("expected dispatcher-missing error frame, got %+v", result)
	}
}

func TestMcpServerBridge_MethodNotFound(t *testing.T) {
	b := NewMcpServerBridge(echoOptions())
	resp := b.Dispatch(context.Background(), &BridgeJSONRPCRequest{Method: "nope", ID: json.RawMessage(`1`)})
	if resp.Error == nil || resp.Error.Code != JSONRPCMethodNotFound {
		t.Fatalf("expected method-not-found, got %+v", resp.Error)
	}
}

// ── A2A inbound ───────────────────────────────────────────────────────────────

func TestA2aServerBridge_SendTaskSingleAction(t *testing.T) {
	b := NewA2aServerBridge(echoOptions())
	req := &BridgeJSONRPCRequest{
		Method: "tasks/send",
		ID:     json.RawMessage(`"call-1"`),
		Params: json.RawMessage(`{
			"id":"task-1",
			"message":{"role":"user","parts":[{"type":"data","data":{"q":"nps"}}]}
		}`),
	}
	resp := b.Dispatch(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var task A2aTask
	if err := json.Unmarshal(resp.Result, &task); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if task.ID != "task-1" {
		t.Fatalf("expected task-1, got %s", task.ID)
	}
	if task.Status.State != A2aTaskStateCompleted {
		t.Fatalf("expected completed, got %s", task.Status.State)
	}
	if len(task.Artifacts) != 1 || task.Artifacts[0].Name != "nps-result" {
		t.Fatalf("unexpected artifacts: %+v", task.Artifacts)
	}
	dataPart := string(task.Artifacts[0].Parts[0].Data)
	if !strings.Contains(dataPart, `"action":"search.web"`) {
		t.Fatalf("action not dispatched: %s", dataPart)
	}
	if !strings.Contains(dataPart, `"q":"nps"`) {
		t.Fatalf("data not passed as params: %s", dataPart)
	}
}

func TestA2aServerBridge_ToolNotFoundWithMultipleActions(t *testing.T) {
	o := NewBridgeServerOptions()
	o.RequireAuth = false
	o.AddAction(BridgeServerAction{ActionID: "a.one"})
	o.AddAction(BridgeServerAction{ActionID: "a.two"})
	o.Dispatch = func(ctx context.Context, frame *BridgeActionFrame) (bridgeServerFrame, error) {
		return CapsResult(json.RawMessage(`{}`)), nil
	}
	b := NewA2aServerBridge(o)
	req := &BridgeJSONRPCRequest{
		Method: "tasks/send",
		ID:     json.RawMessage(`"c"`),
		Params: json.RawMessage(`{"id":"t","message":{"role":"user","parts":[{"type":"text","text":"hi"}]}}`),
	}
	resp := b.Dispatch(context.Background(), req)
	if resp.Error == nil || resp.Error.Code != JSONRPCInvalidParams {
		t.Fatalf("expected invalid-params tool-not-found, got %+v", resp.Error)
	}
	if !strings.Contains(string(resp.Error.Data), BridgeErrServerToolNotFound) {
		t.Fatalf("expected server-tool-not-found data, got %s", resp.Error.Data)
	}
}

func TestA2aServerBridge_ResolvesByMetadata(t *testing.T) {
	o := NewBridgeServerOptions()
	o.RequireAuth = false
	o.AddAction(BridgeServerAction{ActionID: "a.one"})
	o.AddAction(BridgeServerAction{ActionID: "a.two"})
	var dispatched string
	o.Dispatch = func(ctx context.Context, frame *BridgeActionFrame) (bridgeServerFrame, error) {
		dispatched = frame.ActionID
		return CapsResult(json.RawMessage(`{}`)), nil
	}
	b := NewA2aServerBridge(o)
	req := &BridgeJSONRPCRequest{
		Method: "tasks/send",
		ID:     json.RawMessage(`"c"`),
		Params: json.RawMessage(`{"id":"t","metadata":{"action_id":"a.two"},"message":{"role":"user","parts":[{"type":"data","data":{"x":1}}]}}`),
	}
	resp := b.Dispatch(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if dispatched != "a.two" {
		t.Fatalf("expected a.two dispatched, got %s", dispatched)
	}
}

func TestA2aServerBridge_AgentCard(t *testing.T) {
	card := NewA2aServerBridge(echoOptions()).BuildAgentCard("https://host/a2a")
	if card.URL != "https://host/a2a" {
		t.Fatalf("unexpected url: %s", card.URL)
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "search.web" {
		t.Fatalf("unexpected skills: %+v", card.Skills)
	}
	if card.Authentication != nil {
		t.Fatalf("expected no auth when RequireAuth is false")
	}
}

// ── Middleware routing ────────────────────────────────────────────────────────

func TestBridgeServerMiddleware_McpRoute(t *testing.T) {
	mw := NewBridgeServerMiddleware(echoOptions(), nil)
	srv := httptest.NewServer(mw)
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":"1","method":"tools/call","params":{"name":"search_web","arguments":{"q":"x"}}}`
	resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out BridgeJSONRPCResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}
}

func TestBridgeServerMiddleware_AgentCardRoute(t *testing.T) {
	mw := NewBridgeServerMiddleware(echoOptions(), nil)
	srv := httptest.NewServer(mw)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var card A2aAgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasSuffix(card.URL, "/a2a") {
		t.Fatalf("unexpected card url: %s", card.URL)
	}
}

func TestBridgeServerMiddleware_AuthRequired(t *testing.T) {
	o := echoOptions()
	o.RequireAuth = true
	o.VerifyAgent = func(agentNid string, r *http.Request) bool { return true }
	mw := NewBridgeServerMiddleware(o, nil)
	srv := httptest.NewServer(mw)
	defer srv.Close()

	// No agent header → 401.
	resp, _ := http.Post(srv.URL+"/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"ping"}`))
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 without agent, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Valid agent header → 200.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderAgent, "urn:nps:agent:example.com:alice")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200 with valid agent, got %d", resp2.StatusCode)
	}
}

func TestBridgeServerMiddleware_FallThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(203)
		_, _ = io.WriteString(w, "next")
	})
	mw := NewBridgeServerMiddleware(echoOptions(), next)
	srv := httptest.NewServer(mw)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/unrelated")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 203 {
		t.Fatalf("expected fall-through 203, got %d", resp.StatusCode)
	}
}

// ── Node middleware ───────────────────────────────────────────────────────────

func TestBridgeNodeMiddleware_InvokeAndManifest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()

	node := NewBridgeNode(NewDefaultBridgeDispatcherRegistry(upstream.Client()))
	opts := NewBridgeNodeOptions()
	mw := NewBridgeNodeMiddleware(node, NewDefaultBridgeDispatcherRegistry(upstream.Client()), opts, nil)
	srv := httptest.NewServer(mw)
	defer srv.Close()

	// Manifest.
	mresp, _ := http.Get(srv.URL + "/.nwm")
	var manifest map[string]interface{}
	_ = json.NewDecoder(mresp.Body).Decode(&manifest)
	mresp.Body.Close()
	if manifest["node_type"] != NodeTypeBridge {
		t.Fatalf("unexpected node_type: %v", manifest["node_type"])
	}

	// Invoke.
	body, _ := json.Marshal(map[string]interface{}{
		"action_id": "bridge.dispatch",
		"params": map[string]interface{}{
			"protocol":       "http",
			"endpoint":       upstream.URL,
			"method":         "GET",
			"reject_private": false,
		},
	})
	iresp, err := http.Post(srv.URL+"/invoke", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	defer iresp.Body.Close()
	if iresp.StatusCode != 200 {
		b, _ := io.ReadAll(iresp.Body)
		t.Fatalf("expected 200, got %d: %s", iresp.StatusCode, b)
	}
	var caps map[string]interface{}
	_ = json.NewDecoder(iresp.Body).Decode(&caps)
	if caps["anchor_ref"] != HTTPBridgeResponseAnchorRef {
		t.Fatalf("unexpected anchor_ref: %v", caps["anchor_ref"])
	}
}

func TestBridgeNodeMiddleware_UnknownAction(t *testing.T) {
	node := NewBridgeNode(NewDefaultBridgeDispatcherRegistry(http.DefaultClient))
	mw := NewBridgeNodeMiddleware(node, NewDefaultBridgeDispatcherRegistry(http.DefaultClient), NewBridgeNodeOptions(), nil)
	srv := httptest.NewServer(mw)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/invoke", "application/json",
		strings.NewReader(`{"action_id":"wrong.action","params":{}}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
