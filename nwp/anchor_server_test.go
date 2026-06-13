// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	srvPrefix    = "/gw"
	srvAnchorNid = "urn:nps:node:anchor.example.com:svc"
	srvAgent     = "urn:nps:agent:tester"
)

func boolPtr(b bool) *bool { return &b }

func baseOpts() AnchorNodeOptions {
	return AnchorNodeOptions{
		NodeID:     srvAnchorNid,
		PathPrefix: srvPrefix,
		Actions:    map[string]AnchorActionSpec{"orders.create": {ResultAnchor: "nps:orders:result", EstimatedCgn: 10}},
	}
}

func srvMembers() []MemberInfo {
	return []MemberInfo{
		{Nid: "urn:nps:node:w1", NodeRoles: []string{"worker"}, ActivationMode: "resident"},
		{Nid: "urn:nps:node:w2", NodeRoles: []string{"worker"}, ActivationMode: "ephemeral", Tags: []string{"gpu"}},
	}
}

func doReq(t *testing.T, srv *httptest.Server, method, path string, body any, agent string, headers map[string]string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, srv.URL+path, &buf)
	if agent != "" {
		req.Header.Set("X-NWP-Agent", agent)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return m
}

func TestAnchorManifest(t *testing.T) {
	opt := baseOpts()
	opt.DisplayName = "Svc"
	opt.CgnLimit = 500
	opt.ReputationPolicy = &ReputationPolicy{Enabled: false, LogSources: []string{"https://log"}}
	opt.TrustAnchors = []string{"urn:nps:ca:root"}
	app := NewAnchorNodeApp(opt, AnchorNodeAppDeps{})
	srv := httptest.NewServer(app)
	defer srv.Close()

	resp := doReq(t, srv, "GET", srvPrefix+"/.nwm", nil, srvAgent, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/nwp-manifest+json" {
		t.Fatal("content type")
	}
	m := decode(t, resp)
	if m["nwp"] != "0.4" || m["node_type"] != "anchor" {
		t.Fatalf("manifest basics: %+v", m)
	}
	auth := m["auth"].(map[string]any)
	if auth["required"] != true || auth["identity_type"] != "nip-cert" {
		t.Fatal("auth block")
	}
	tb := m["token_budget"].(map[string]any)
	if tb["cgn_limit"].(float64) != 500 {
		t.Fatal("cgn_limit")
	}
	// reputation_policy is disabled → omitted
	if _, ok := m["reputation_policy"]; ok {
		t.Fatal("disabled reputation policy should be omitted")
	}
	if m["trust_anchors"].([]any)[0] != "urn:nps:ca:root" {
		t.Fatal("trust anchors")
	}
}

func TestAnchorAuthGate(t *testing.T) {
	app := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{})
	srv := httptest.NewServer(app)
	defer srv.Close()

	resp := doReq(t, srv, "GET", srvPrefix+"/.nwm", nil, "", nil)
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if decode(t, resp)["error"] != ErrAuthNidScopeViolation {
		t.Fatal("error code")
	}

	opt := baseOpts()
	opt.RequireAuth = boolPtr(false)
	app2 := NewAnchorNodeApp(opt, AnchorNodeAppDeps{})
	srv2 := httptest.NewServer(app2)
	defer srv2.Close()
	resp2 := doReq(t, srv2, "GET", srvPrefix+"/.nwm", nil, "", nil)
	if resp2.StatusCode != 200 {
		t.Fatalf("auth disabled should allow, got %d", resp2.StatusCode)
	}
}

func TestAnchorSnapshotViaClient(t *testing.T) {
	topo := &InMemoryAnchorTopologyService{Nid: srvAnchorNid, Members: srvMembers(), Version: 7}
	opt := baseOpts()
	opt.RequireAuth = boolPtr(false)
	app := NewAnchorNodeApp(opt, AnchorNodeAppDeps{TopologyService: topo})
	srv := httptest.NewServer(app)
	defer srv.Close()

	client := NewAnchorNodeClient(srv.URL, WithPathPrefix(srvPrefix))
	snap, err := client.GetSnapshot(context.Background(), "cluster", []string{"members"}, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != 7 || snap.AnchorNid != srvAnchorNid || snap.ClusterSize != 2 || len(snap.Members) != 2 {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
}

func TestAnchorStreamViaClient(t *testing.T) {
	events := []TopologyEvent{
		{Kind: "member_joined", Version: 8, MemberJoined: &MemberJoinedEvent{Member: srvMembers()[0]}},
		{Kind: "resync_required", Version: 0, ResyncRequired: &ResyncRequiredEvent{Reason: "rebased"}},
	}
	topo := &InMemoryAnchorTopologyService{Nid: srvAnchorNid, Members: srvMembers(), Version: 1, Events: events}
	opt := baseOpts()
	opt.RequireAuth = boolPtr(false)
	app := NewAnchorNodeApp(opt, AnchorNodeAppDeps{TopologyService: topo})
	srv := httptest.NewServer(app)
	defer srv.Close()

	client := NewAnchorNodeClient(srv.URL, WithPathPrefix(srvPrefix))
	evCh, errCh := client.Subscribe(context.Background(), "cluster", nil, nil)
	var received []TopologyEvent
	for ev := range evCh {
		received = append(received, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if len(received) != 2 || received[0].Kind != "member_joined" || received[1].Kind != "resync_required" {
		t.Fatalf("stream mismatch: %+v", received)
	}
}

func TestAnchorTopologyErrors(t *testing.T) {
	topo := &InMemoryAnchorTopologyService{Nid: srvAnchorNid, Members: srvMembers()}
	app := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{TopologyService: topo})
	srv := httptest.NewServer(app)
	defer srv.Close()

	r := doReq(t, srv, "POST", srvPrefix+"/query", map[string]any{"type": "topology.bogus", "topology": map[string]any{}}, srvAgent, nil)
	if r.StatusCode != 501 || decode(t, r)["error"] != ErrReservedTypeUnsupported {
		t.Fatal("reserved type unsupported")
	}

	r2 := doReq(t, srv, "POST", srvPrefix+"/query", map[string]any{"type": "topology.snapshot", "topology": map[string]any{"scope": "member"}}, srvAgent, nil)
	if r2.StatusCode != 400 || decode(t, r2)["error"] != ErrTopologyUnsupportedScope {
		t.Fatal("member scope needs target_nid")
	}

	app2 := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{}) // no topology service
	srv2 := httptest.NewServer(app2)
	defer srv2.Close()
	r3 := doReq(t, srv2, "POST", srvPrefix+"/query", map[string]any{"type": "topology.snapshot", "topology": map[string]any{"scope": "cluster"}}, srvAgent, nil)
	if r3.StatusCode != 501 || decode(t, r3)["error"] != ErrNodeUnavailable {
		t.Fatal("no topology service")
	}
}

func TestAnchorCapabilityGate(t *testing.T) {
	topo := &InMemoryAnchorTopologyService{Nid: srvAnchorNid, Members: srvMembers()}
	opt := baseOpts()
	opt.RequireTopologyCapability = true
	app := NewAnchorNodeApp(opt, AnchorNodeAppDeps{TopologyService: topo})
	srv := httptest.NewServer(app)
	defer srv.Close()

	denied := doReq(t, srv, "POST", srvPrefix+"/query", map[string]any{"type": "topology.snapshot", "topology": map[string]any{}}, srvAgent, nil)
	if denied.StatusCode != 403 || decode(t, denied)["error"] != ErrTopologyUnauthorized {
		t.Fatal("capability gate should deny")
	}
	ok := doReq(t, srv, "POST", srvPrefix+"/query", map[string]any{"type": "topology.snapshot", "topology": map[string]any{}}, srvAgent, map[string]string{"X-NWP-Capabilities": "topology:read"})
	if ok.StatusCode != 200 {
		t.Fatalf("capability granted should pass, got %d", ok.StatusCode)
	}
}

func okHandler(_ context.Context, actionID string, _ json.RawMessage, ic InvokeContext) (any, error) {
	return map[string]any{"order_id": "o-123", "action": actionID, "agent": ic.AgentNid}, nil
}

func TestAnchorInvoke(t *testing.T) {
	app := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{InvokeHandler: okHandler})
	srv := httptest.NewServer(app)
	defer srv.Close()

	t.Run("sync caps", func(t *testing.T) {
		r := doReq(t, srv, "POST", srvPrefix+"/invoke", map[string]any{"action_id": "orders.create", "params": map[string]any{"x": 1}}, srvAgent, nil)
		if r.StatusCode != 200 || r.Header.Get("Content-Type") != "application/nwp-capsule" {
			t.Fatalf("status %d", r.StatusCode)
		}
		m := decode(t, r)
		if m["count"].(float64) != 1 {
			t.Fatal("count")
		}
		data := m["data"].([]any)[0].(map[string]any)
		if data["order_id"] != "o-123" || data["agent"] != srvAgent {
			t.Fatalf("data %+v", data)
		}
	})

	t.Run("unknown action 404", func(t *testing.T) {
		r := doReq(t, srv, "POST", srvPrefix+"/invoke", map[string]any{"action_id": "nope.verb"}, srvAgent, nil)
		if r.StatusCode != 404 || decode(t, r)["error"] != ErrActionNotFound {
			t.Fatal("unknown action")
		}
	})

	t.Run("cgn limit pre-check", func(t *testing.T) {
		r := doReq(t, srv, "POST", srvPrefix+"/invoke", map[string]any{"action_id": "orders.create"}, srvAgent, map[string]string{"X-NWP-Budget": "5"})
		if r.StatusCode != 400 || decode(t, r)["error"] != ErrCgnLimitExceeded {
			t.Fatal("cgn limit")
		}
	})
}

func TestAnchorInvokeNoHandlerAndErrors(t *testing.T) {
	app := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{})
	srv := httptest.NewServer(app)
	defer srv.Close()
	r := doReq(t, srv, "POST", srvPrefix+"/invoke", map[string]any{"action_id": "orders.create"}, srvAgent, nil)
	if r.StatusCode != 501 {
		t.Fatalf("no handler should 501, got %d", r.StatusCode)
	}

	badHandler := func(_ context.Context, _ string, _ json.RawMessage, _ InvokeContext) (any, error) {
		return nil, &AnchorActionError{HTTPStatus: 422, NpsStatus: "NPS-CLIENT-BAD-REQUEST", ErrorCode: ErrActionParamsInvalid, Message: "bad"}
	}
	app2 := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{InvokeHandler: badHandler})
	srv2 := httptest.NewServer(app2)
	defer srv2.Close()
	r2 := doReq(t, srv2, "POST", srvPrefix+"/invoke", map[string]any{"action_id": "orders.create"}, srvAgent, nil)
	if r2.StatusCode != 422 || decode(t, r2)["error"] != ErrActionParamsInvalid {
		t.Fatal("handler error envelope")
	}
}

func TestAnchorReputationBanBlocksInvoke(t *testing.T) {
	// Seed the reference evaluator's in-process cache with a raw log entry so the NID resolves
	// without an HTTP log query (concrete type used for cache access; same package).
	ev := &reputationEvaluator{
		logMap: map[string]cacheEntry{
			srvAgent: {
				expiresAt: time.Now().Add(time.Hour),
				entries:   []logEntry{{Incident: "impersonation-claim", Severity: "critical", Timestamp: time.Now().UTC()}},
			},
		},
	}
	opt := baseOpts()
	opt.ReputationPolicy = &ReputationPolicy{Enabled: true, CacheTtlSeconds: 300, BanOn: []ReputationRule{{Incident: "*", Severity: ">=critical"}}}
	app := NewAnchorNodeApp(opt, AnchorNodeAppDeps{InvokeHandler: okHandler, ReputationEvaluator: ev})
	srv := httptest.NewServer(app)
	defer srv.Close()

	r := doReq(t, srv, "POST", srvPrefix+"/invoke", map[string]any{"action_id": "orders.create"}, srvAgent, nil)
	if r.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", r.StatusCode)
	}
	if decode(t, r)["error"] != ErrReputationBanned {
		t.Fatal("ban error code")
	}
}

func TestAnchorUnknownPathIs404BeforeAuth(t *testing.T) {
	// An unknown sub-path must be 404 regardless of auth — a missing X-NWP-Agent on a route with
	// no resource must NOT leak a 401 (auth state). Note: no agent header is sent.
	app := NewAnchorNodeApp(baseOpts(), AnchorNodeAppDeps{})
	srv := httptest.NewServer(app)
	defer srv.Close()

	resp := doReq(t, srv, "GET", srvPrefix+"/nope", nil, "", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for unknown path, got %d", resp.StatusCode)
	}
	if decode(t, resp)["error"] != ErrActionNotFound {
		t.Fatal("error code")
	}
}
