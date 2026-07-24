// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Test provider ────────────────────────────────────────────────────────────

type rowsComplexProvider struct {
	rows []MemoryNodeRow
}

func (p rowsComplexProvider) Query(_ context.Context, _ *QueryFrame, _ ComplexNodeOptions) (*MemoryNodeQueryResult, error) {
	return &MemoryNodeQueryResult{Rows: p.rows}, nil
}

func (p rowsComplexProvider) Execute(_ context.Context, frame *ActionFrame, actx ActionContext) (*ActionExecutionResult, error) {
	payload, _ := json.Marshal(map[string]any{"ok": true, "action": frame.Action})
	return &ActionExecutionResult{Result: payload, AnchorRef: actx.Spec.ResultAnchor}, nil
}

func cnPost(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte(`{"filter":{}}`)))
	req.Header.Set("Content-Type", MimeFrame)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func cnDecode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return m
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestComplexNodeManifestAndLocalQuery(t *testing.T) {
	prov := rowsComplexProvider{rows: []MemoryNodeRow{{"id": "u1"}}}
	srv := httptest.NewServer(NewComplexNodeServer(prov, ComplexNodeOptions{
		NodeID:     "urn:nps:node:api.example.com:orders",
		PathPrefix: "/orders",
		Schema:     &MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}},
	}, nil))
	defer srv.Close()

	// Manifest.
	resp, err := http.Get(srv.URL + "/orders/.nwm")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Header.Get(HeaderNodeType) != "complex" {
		t.Fatalf("nwm status %d type %s", resp.StatusCode, resp.Header.Get(HeaderNodeType))
	}
	nwm := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&nwm)
	resp.Body.Close()
	if nwm["node_type"] != "complex" || nwm["nwp"] != "0.4" {
		t.Fatalf("manifest %+v", nwm)
	}

	// Local query, no depth => no graph field.
	q := cnPost(t, srv.URL+"/orders/query", nil)
	if q.StatusCode != 200 || q.Header.Get("Content-Type") != MimeCapsule {
		t.Fatalf("query status %d ct %s", q.StatusCode, q.Header.Get("Content-Type"))
	}
	m := cnDecode(t, q)
	if m["count"].(float64) != 1 {
		t.Fatalf("count %+v", m)
	}
	if _, hasGraph := m["graph"]; hasGraph {
		t.Fatal("graph must be absent when depth=0")
	}
}

func TestComplexNodeGraphExpansion(t *testing.T) {
	// Child node serving its own rows.
	child := httptest.NewServer(NewComplexNodeServer(
		rowsComplexProvider{rows: []MemoryNodeRow{{"id": "c1"}}},
		ComplexNodeOptions{NodeID: "urn:nps:node:child", PathPrefix: "/c",
			Schema: &MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}}},
		nil))
	defer child.Close()

	// Parent references the child; private child URLs allowed (loopback in tests).
	allowHTTP := true
	rejectPriv := false
	parent := httptest.NewServer(NewComplexNodeServer(
		rowsComplexProvider{rows: []MemoryNodeRow{{"id": "p1"}}},
		ComplexNodeOptions{
			NodeID:                 "urn:nps:node:parent",
			PathPrefix:             "/p",
			Schema:                 &MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}},
			Graph:                  []ComplexGraphRef{{Rel: "child", NodeURL: child.URL + "/c"}},
			AllowHTTPChildURLs:     allowHTTP,
			RejectPrivateChildURLs: &rejectPriv,
		}, nil))
	defer parent.Close()

	resp := cnPost(t, parent.URL+"/p/query", map[string]string{HeaderDepth: "1"})
	if resp.StatusCode != 200 {
		t.Fatalf("query status %d", resp.StatusCode)
	}
	m := cnDecode(t, resp)
	graph, ok := m["graph"].([]any)
	if !ok || len(graph) != 1 {
		t.Fatalf("expected 1 graph entry, got %+v", m["graph"])
	}
	entry := graph[0].(map[string]any)
	if entry["rel"] != "child" {
		t.Fatalf("graph rel %+v", entry)
	}
	if _, hasErr := entry["error"]; hasErr {
		t.Fatalf("child fetch errored: %+v", entry)
	}
	// Child capsule embedded under "data".
	childCaps, ok := entry["data"].(map[string]any)
	if !ok {
		t.Fatalf("child data %+v", entry)
	}
	if childCaps["count"].(float64) != 1 {
		t.Fatalf("child count %+v", childCaps)
	}
}

func TestComplexNodeCycleDetection(t *testing.T) {
	srv := httptest.NewServer(NewComplexNodeServer(
		rowsComplexProvider{rows: []MemoryNodeRow{}},
		ComplexNodeOptions{NodeID: "urn:nps:node:self", PathPrefix: "/s",
			Schema: &MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}}},
		nil))
	defer srv.Close()

	// Trace already contains this node => cycle.
	resp := cnPost(t, srv.URL+"/s/query", map[string]string{HeaderTrace: "urn:nps:node:other,urn:nps:node:self"})
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422 cycle, got %d", resp.StatusCode)
	}
	if cnDecode(t, resp)["error"] != ErrGraphCycle {
		t.Fatal("expected graph cycle error code")
	}
}

func TestComplexNodeDepthExceeded(t *testing.T) {
	maxDepth := uint(2)
	srv := httptest.NewServer(NewComplexNodeServer(
		rowsComplexProvider{rows: []MemoryNodeRow{}},
		ComplexNodeOptions{NodeID: "urn:nps:node:d", PathPrefix: "/d",
			GraphMaxDepth: maxDepth,
			Schema:        &MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}}},
		nil))
	defer srv.Close()

	resp := cnPost(t, srv.URL+"/d/query", map[string]string{HeaderDepth: "3"})
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 depth exceeded, got %d", resp.StatusCode)
	}
	if cnDecode(t, resp)["error"] != ErrDepthExceeded {
		t.Fatal("expected depth exceeded error code")
	}
}

func TestComplexNodeChildSSRFRejected(t *testing.T) {
	// Default RejectPrivateChildURLs (true) => loopback child rejected inline.
	parent := httptest.NewServer(NewComplexNodeServer(
		rowsComplexProvider{rows: []MemoryNodeRow{{"id": "p1"}}},
		ComplexNodeOptions{
			NodeID:     "urn:nps:node:parent",
			PathPrefix: "/p",
			Schema:     &MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}},
			Graph:      []ComplexGraphRef{{Rel: "child", NodeURL: "https://127.0.0.1/c"}},
		}, nil))
	defer parent.Close()

	resp := cnPost(t, parent.URL+"/p/query", map[string]string{HeaderDepth: "1"})
	if resp.StatusCode != 200 {
		t.Fatalf("query status %d", resp.StatusCode)
	}
	m := cnDecode(t, resp)
	entry := m["graph"].([]any)[0].(map[string]any)
	errObj, ok := entry["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected SSRF error on child, got %+v", entry)
	}
	if errObj["code"] != ErrAuthNidScopeViolation {
		t.Fatalf("SSRF error code %+v", errObj)
	}
	if _, hasData := entry["data"]; hasData {
		t.Fatal("rejected child must not carry data")
	}
}

func TestComplexNodeInvoke(t *testing.T) {
	srv := httptest.NewServer(NewComplexNodeServer(
		rowsComplexProvider{},
		ComplexNodeOptions{
			NodeID:     "urn:nps:node:api",
			PathPrefix: "/a",
			Actions:    map[string]ActionSpec{"orders.touch": {ResultAnchor: "nps:touch"}},
		}, nil))
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/a/invoke",
		strings.NewReader(`{"action_id":"orders.touch"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("invoke status %d", resp.StatusCode)
	}
	m := cnDecode(t, resp)
	if m["anchor_ref"] != "nps:touch" || m["count"].(float64) != 1 {
		t.Fatalf("invoke result %+v", m)
	}

	// Async on a Complex Node is rejected.
	req2, _ := http.NewRequest("POST", srv.URL+"/a/invoke",
		strings.NewReader(`{"action_id":"orders.touch","async":true}`))
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2.StatusCode != 400 || cnDecode(t, resp2)["error"] != ErrActionParamsInvalid {
		t.Fatal("async on complex node should be rejected")
	}
}
