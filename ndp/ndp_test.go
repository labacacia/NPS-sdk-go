// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labacacia/NPS-sdk-go/ndp"
	"github.com/labacacia/NPS-sdk-go/nip"
)

// ── mockDnsTxtLookup ──────────────────────────────────────────────────────────

type mockDnsTxtLookup struct {
	records []string
	called  bool
}

func (m *mockDnsTxtLookup) LookupTXT(_ context.Context, _ string) ([]string, error) {
	m.called = true
	return m.records, nil
}

// ── AnnounceFrame ─────────────────────────────────────────────────────────────

func TestAnnounceFrame_Roundtrip(t *testing.T) {
	nodeType := "complex"
	f := &ndp.AnnounceFrame{
		NID:       "urn:nps:node:example.com:agent",
		Addresses: []map[string]any{{"host": "example.com", "port": uint64(17433), "protocol": "nps"}},
		Caps:      []string{"nwp", "nop"},
		TTL:       300,
		Timestamp: "2026-01-01T00:00:00Z",
		Signature: "ed25519:abc123",
		NodeType:  &nodeType,
	}
	d := f.ToDict()
	if _, ok := d["capabilities"]; !ok {
		t.Fatalf("ToDict must emit canonical capabilities field: %+v", d)
	}
	if _, ok := d["caps"]; ok {
		t.Fatalf("ToDict must not emit legacy caps field: %+v", d)
	}
	f2 := ndp.AnnounceFrameFromDict(d)
	if f2.NID != f.NID {
		t.Errorf("NID mismatch")
	}
	if f2.TTL != 300 {
		t.Errorf("TTL mismatch: got %d", f2.TTL)
	}
	if len(f2.Caps) != 2 {
		t.Errorf("Caps mismatch: %v", f2.Caps)
	}
	if f2.NodeType == nil || *f2.NodeType != nodeType {
		t.Errorf("NodeType mismatch")
	}
}

func TestAnnounceFrame_DefaultTTL(t *testing.T) {
	f := ndp.AnnounceFrameFromDict(map[string]any{
		"nid":       "urn:nps:node:x.com:n",
		"addresses": []any{},
		"caps":      []any{},
		"timestamp": "t",
		"signature": "ed25519:s",
	})
	if f.TTL != 300 {
		t.Errorf("expected default TTL=300, got %d", f.TTL)
	}
}

func TestAnnounceFrame_FromDictCapabilities(t *testing.T) {
	f := ndp.AnnounceFrameFromDict(map[string]any{
		"nid":          "urn:nps:node:x.com:n",
		"addresses":    []any{},
		"capabilities": []any{"nwp:query"},
		"ttl":          300,
		"timestamp":    "t",
		"signature":    "ed25519:s",
	})
	if len(f.Caps) != 1 || f.Caps[0] != "nwp:query" {
		t.Fatalf("expected capabilities to parse, got %+v", f.Caps)
	}
}

func TestAnnounceFrame_UnsignedDict_NoSignature(t *testing.T) {
	f := &ndp.AnnounceFrame{
		NID: "urn:nps:node:x.com:n", Caps: []string{},
		Addresses: []map[string]any{}, TTL: 300, Timestamp: "t", HeartbeatIntervalMs: 60_000,
	}
	d := f.UnsignedDict()
	if _, ok := d["signature"]; ok {
		t.Error("UnsignedDict should not contain 'signature'")
	}
	if _, ok := d["node_type"]; ok {
		t.Error("UnsignedDict should omit null node_type")
	}
	if _, ok := d["frame"]; ok {
		t.Error("UnsignedDict should not contain frame discriminant")
	}
	if d["heartbeat_interval_ms"] != uint64(60_000) {
		t.Fatalf("UnsignedDict should include default heartbeat_interval_ms, got %+v", d)
	}
}

func TestAnnounceFrame_HeartbeatZeroCanonicalizesLiterally(t *testing.T) {
	f := &ndp.AnnounceFrame{
		NID: "urn:nps:node:x.com:n", Caps: []string{},
		Addresses: []map[string]any{}, TTL: 300, Timestamp: "t", HeartbeatIntervalMs: 0,
	}
	if got := f.UnsignedDict()["heartbeat_interval_ms"]; got != uint64(0) {
		t.Fatalf("explicit heartbeat_interval_ms=0 must be signed literally, got %v", got)
	}
}

func TestAnnounceFrame_FromDictAbsentHeartbeatDefaults(t *testing.T) {
	f := ndp.AnnounceFrameFromDict(map[string]any{
		"nid":          "urn:nps:node:x.com:n",
		"addresses":    []any{},
		"capabilities": []any{},
		"ttl":          300,
		"timestamp":    "t",
		"signature":    "ed25519:s",
	})
	if f.HeartbeatIntervalMs != 60_000 {
		t.Fatalf("absent heartbeat_interval_ms should default to 60000, got %d", f.HeartbeatIntervalMs)
	}

	explicitZero := ndp.AnnounceFrameFromDict(map[string]any{
		"nid":                   "urn:nps:node:x.com:n",
		"addresses":             []any{},
		"capabilities":          []any{},
		"ttl":                   300,
		"timestamp":             "t",
		"signature":             "ed25519:s",
		"heartbeat_interval_ms": uint64(0),
	})
	if explicitZero.HeartbeatIntervalMs != 0 {
		t.Fatalf("explicit heartbeat_interval_ms=0 should round-trip literally, got %d", explicitZero.HeartbeatIntervalMs)
	}
}

func TestAnnounceFrame_LivenessWireOnly(t *testing.T) {
	// NDP v0.9 health/last_seen: on the wire, but NOT in the signed canonical form.
	f := &ndp.AnnounceFrame{
		NID: "urn:nps:node:x.com:n", Caps: []string{}, Addresses: []map[string]any{},
		TTL: 300, Timestamp: "t", Health: "draining", LastSeen: "2026-06-13T00:00:00Z",
	}
	d := f.ToDict()
	if d["health"] != "draining" || d["last_seen"] != "2026-06-13T00:00:00Z" {
		t.Fatalf("ToDict missing liveness fields: %+v", d)
	}
	u := f.UnsignedDict()
	if _, ok := u["health"]; ok {
		t.Error("UnsignedDict must exclude health")
	}
	if _, ok := u["last_seen"]; ok {
		t.Error("UnsignedDict must exclude last_seen")
	}
	f2 := ndp.AnnounceFrameFromDict(d)
	if f2.Health != "draining" || f2.LastSeen != "2026-06-13T00:00:00Z" {
		t.Errorf("FromDict did not round-trip liveness: %+v", f2)
	}
}

func TestAnnounceFrame_ActivationEndpointIsAddressObject(t *testing.T) {
	endpoint := map[string]any{"host": "10.0.0.5", "port": uint64(17440), "protocol": "nwp"}
	f := &ndp.AnnounceFrame{
		NID: "urn:nps:node:x.com:n", Caps: []string{}, Addresses: []map[string]any{},
		TTL: 300, Timestamp: "t", Signature: "ed25519:s",
		ActivationMode: "resident", ActivationEndpoint: endpoint,
	}
	d := f.ToDict()
	got, ok := d["activation_endpoint"].(map[string]any)
	if !ok {
		t.Fatalf("activation_endpoint must be an address object, got %+v", d["activation_endpoint"])
	}
	if got["host"] != "10.0.0.5" || got["protocol"] != "nwp" {
		t.Fatalf("unexpected activation_endpoint: %+v", got)
	}
	if _, ok := f.UnsignedDict()["activation_endpoint"].(map[string]any); !ok {
		t.Fatalf("UnsignedDict must include activation_endpoint: %+v", f.UnsignedDict())
	}
	f2 := ndp.AnnounceFrameFromDict(d)
	if f2.ActivationEndpoint["host"] != "10.0.0.5" {
		t.Fatalf("FromDict did not round-trip activation_endpoint: %+v", f2.ActivationEndpoint)
	}
}

// ── ResolveFrame ──────────────────────────────────────────────────────────────

func TestResolveFrame_Roundtrip(t *testing.T) {
	req := "urn:nps:node:example.com:agent"
	resolved := map[string]any{"host": "example.com", "port": float64(17433)}
	f := &ndp.ResolveFrame{
		Target:       "nwp://example.com/agent",
		RequesterNID: &req,
		Resolved:     resolved,
	}
	d := f.ToDict()
	f2 := ndp.ResolveFrameFromDict(d)
	if f2.Target != f.Target {
		t.Errorf("Target mismatch")
	}
	if f2.RequesterNID == nil || *f2.RequesterNID != req {
		t.Errorf("RequesterNID mismatch")
	}
	if f2.Resolved == nil {
		t.Error("Resolved should not be nil")
	}
}

// ── GraphFrame ────────────────────────────────────────────────────────────────

func TestGraphFrame_Roundtrip(t *testing.T) {
	// GraphFrame was rewritten to the §3.3 topology-snapshot format
	// (graph_id / nodes / edges / ttl); exercise ToDict + FromDict against it.
	f := &ndp.GraphFrame{GraphID: "g1", TTL: 300}
	d := f.ToDict()
	if d["graph_id"] != "g1" || d["ttl"] != 300 {
		t.Errorf("ToDict mismatch: %+v", d)
	}
	// Reconstruct from the wire shape (nodes/edges arrive as []any of maps).
	wire := map[string]any{
		"graph_id": "g1",
		"ttl":      300,
		"nodes":    []any{map[string]any{"nid": "n1", "cluster_anchor": "a1"}},
	}
	f2 := ndp.GraphFrameFromDict(wire)
	if f2.GraphID != "g1" {
		t.Errorf("GraphID mismatch: %q", f2.GraphID)
	}
	if len(f2.Nodes) != 1 || f2.Nodes[0].NID != "n1" {
		t.Errorf("Nodes mismatch: %+v", f2.Nodes)
	}
}

func TestGraphFrame_ValidateRejectsTooLarge(t *testing.T) {
	nodes := make([]ndp.GraphNode, 257)
	for i := range nodes {
		nodes[i] = ndp.GraphNode{NID: "urn:nps:node:example.com:n"}
	}
	err := (&ndp.GraphFrame{GraphID: "too-big", Nodes: nodes, Edges: nil, TTL: 60}).Validate()
	if err == nil || !strings.Contains(err.Error(), ndp.ErrGraphTooLarge) {
		t.Fatalf("want %s error, got %v", ndp.ErrGraphTooLarge, err)
	}
}

func TestGraphFrame_ValidateRejectsInvalidEdges(t *testing.T) {
	nodes := []ndp.GraphNode{{NID: "urn:nps:node:example.com:a"}}
	cases := []ndp.NdpGraphEdge{
		{FromNID: nodes[0].NID, ToNID: nodes[0].NID},
		{FromNID: nodes[0].NID, ToNID: "urn:nps:node:example.com:missing"},
	}
	for _, edge := range cases {
		err := (&ndp.GraphFrame{GraphID: "bad-edge", Nodes: nodes, Edges: []ndp.NdpGraphEdge{edge}, TTL: 60}).Validate()
		if err == nil || !strings.Contains(err.Error(), ndp.ErrGraphInvalid) {
			t.Fatalf("want %s error, got %v", ndp.ErrGraphInvalid, err)
		}
	}
}

func TestFederationForwardedBy(t *testing.T) {
	header := "urn:nps:agent:registry-a.example.com:r1, urn:nps:agent:registry-b.example.com:r2"
	hops := ndp.ParseForwardedBy(header)
	if len(hops) != 2 || hops[0] != "urn:nps:agent:registry-a.example.com:r1" {
		t.Fatalf("unexpected hops: %#v", hops)
	}

	next, ok, err := ndp.AppendForwardedBy("urn:nps:agent:registry-c.example.com:r3", header)
	if err != nil || !ok || !strings.Contains(next, "registry-c") {
		t.Fatalf("append failed: next=%q ok=%v err=%v", next, ok, err)
	}

	_, ok, err = ndp.AppendForwardedBy("urn:nps:agent:registry-b.example.com:r2", header)
	if err == nil || ok || !strings.Contains(err.Error(), ndp.ErrFederationLoop) {
		t.Fatalf("want loop error, ok=%v err=%v", ok, err)
	}

	_, ok, err = ndp.AppendForwardedBy(
		"urn:nps:agent:registry-d.example.com:r4",
		header+", urn:nps:agent:registry-c.example.com:r3",
	)
	if err != nil || ok {
		t.Fatalf("want silent hop-limit drop, ok=%v err=%v", ok, err)
	}
}

// ── InMemoryNdpRegistry ───────────────────────────────────────────────────────

func makeAnnounce(nid string, ttl uint64) *ndp.AnnounceFrame {
	return &ndp.AnnounceFrame{
		NID: nid,
		Addresses: []map[string]any{
			{"host": "example.com", "port": uint64(17433), "protocol": "nps"},
		},
		Caps:      []string{"nwp"},
		TTL:       ttl,
		Timestamp: "2026-01-01T00:00:00Z",
	}
}

func TestRegistry_AnnounceAndGet(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	frame := makeAnnounce("urn:nps:node:example.com:agent", 300)
	reg.Announce(frame)
	got := reg.GetByNID("urn:nps:node:example.com:agent")
	if got == nil {
		t.Fatal("expected frame, got nil")
	}
	if got.NID != frame.NID {
		t.Errorf("NID mismatch")
	}
}

func TestRegistry_TTLExpiry(t *testing.T) {
	now := time.Unix(2000, 0)
	reg := ndp.NewInMemoryNdpRegistry()
	reg.Clock = func() time.Time { return now }

	frame := makeAnnounce("urn:nps:node:example.com:agent", 10)
	reg.Announce(frame)

	reg.Clock = func() time.Time { return now.Add(15 * time.Second) }
	got := reg.GetByNID("urn:nps:node:example.com:agent")
	if got != nil {
		t.Error("expected nil after TTL expiry")
	}
}

func TestRegistry_TTLZero_Removes(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	frame := makeAnnounce("urn:nps:node:example.com:agent", 300)
	reg.Announce(frame)

	// TTL=0 should deregister
	frame.TTL = 0
	reg.Announce(frame)
	got := reg.GetByNID("urn:nps:node:example.com:agent")
	if got != nil {
		t.Error("expected nil after TTL=0 removal")
	}
}

func TestRegistry_GetAll(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	reg.Announce(makeAnnounce("urn:nps:node:a.com:n1", 300))
	reg.Announce(makeAnnounce("urn:nps:node:b.com:n2", 300))
	all := reg.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 frames, got %d", len(all))
	}
}

func TestRegistry_Resolve(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	reg.Announce(makeAnnounce("urn:nps:node:example.com:agent", 300))
	res := reg.Resolve("nwp://example.com/agent")
	if res == nil {
		t.Fatal("expected resolve result")
	}
	if res.Host != "example.com" {
		t.Errorf("host mismatch: %s", res.Host)
	}
	if res.Port != 17433 {
		t.Errorf("port mismatch: %d", res.Port)
	}
}

func TestRegistry_Resolve_SubPath(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	reg.Announce(makeAnnounce("urn:nps:node:example.com:agent", 300))
	res := reg.Resolve("nwp://example.com/agent/subpath")
	if res == nil {
		t.Fatal("sub-path should still resolve")
	}
}

func TestRegistry_Resolve_NotFound(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	res := reg.Resolve("nwp://unknown.com/agent")
	if res != nil {
		t.Error("expected nil for unknown target")
	}
}

// ── NwpTargetMatchesNID ───────────────────────────────────────────────────────

func TestNwpTargetMatchesNID(t *testing.T) {
	cases := []struct {
		nid    string
		target string
		want   bool
	}{
		{"urn:nps:node:example.com:agent", "nwp://example.com/agent", true},
		{"urn:nps:node:example.com:agent", "nwp://example.com/agent/sub", true},
		{"urn:nps:node:example.com:agent", "nwp://other.com/agent", false},
		{"urn:nps:node:example.com:agent", "nwp://example.com/other", false},
		{"urn:nps:node:example.com:agent", "http://example.com/agent", false},
		{"urn:nps:node:example.com:agent", "nwp://example.com/agentextra", false},
		{"invalid", "nwp://example.com/agent", false},
	}
	for _, tc := range cases {
		got := ndp.NwpTargetMatchesNID(tc.nid, tc.target)
		if got != tc.want {
			t.Errorf("NwpTargetMatchesNID(%q, %q) = %v, want %v", tc.nid, tc.target, got, tc.want)
		}
	}
}

// ── NdpAnnounceValidator ──────────────────────────────────────────────────────

func TestValidator_Valid(t *testing.T) {
	id, _ := nip.Generate()
	validator := ndp.NewNdpAnnounceValidator()
	nid := "urn:nps:node:example.com:agent"
	validator.RegisterPublicKey(nid, id.PubKeyString())

	frame := &ndp.AnnounceFrame{
		NID:       nid,
		Addresses: []map[string]any{},
		Caps:      []string{"nwp"},
		TTL:       300,
		Timestamp: "2026-01-01T00:00:00Z",
	}
	frame.Signature = id.Sign(frame.UnsignedDict())

	result := validator.Validate(frame)
	if !result.IsValid {
		t.Errorf("expected valid, got error: %s %s", result.ErrorCode, result.Message)
	}
}

func TestValidator_UnknownNID(t *testing.T) {
	validator := ndp.NewNdpAnnounceValidator()
	frame := &ndp.AnnounceFrame{
		NID:       "urn:nps:node:unknown.com:n",
		Signature: "ed25519:abc",
	}
	result := validator.Validate(frame)
	if result.IsValid {
		t.Error("should be invalid for unknown NID")
	}
	if result.ErrorCode != "NDP-ANNOUNCE-NID-MISMATCH" {
		t.Errorf("wrong error code: %s", result.ErrorCode)
	}
}

func TestValidator_BadSignaturePrefix(t *testing.T) {
	validator := ndp.NewNdpAnnounceValidator()
	nid := "urn:nps:node:example.com:n"
	id, _ := nip.Generate()
	validator.RegisterPublicKey(nid, id.PubKeyString())
	frame := &ndp.AnnounceFrame{
		NID:       nid,
		Signature: "rsa:invalidsig",
	}
	result := validator.Validate(frame)
	if result.IsValid {
		t.Error("should be invalid for non-ed25519 signature prefix")
	}
	if result.ErrorCode != "NDP-ANNOUNCE-SIGNATURE-INVALID" {
		t.Errorf("wrong error code: %s", result.ErrorCode)
	}
}

func TestValidator_RemoveKey(t *testing.T) {
	id, _ := nip.Generate()
	validator := ndp.NewNdpAnnounceValidator()
	nid := "urn:nps:node:example.com:n"
	validator.RegisterPublicKey(nid, id.PubKeyString())
	validator.RemovePublicKey(nid)

	frame := &ndp.AnnounceFrame{NID: nid, Signature: "ed25519:abc"}
	result := validator.Validate(frame)
	if result.IsValid {
		t.Error("should be invalid after key removal")
	}
}

func TestValidator_KnownPublicKeys(t *testing.T) {
	id, _ := nip.Generate()
	validator := ndp.NewNdpAnnounceValidator()
	validator.RegisterPublicKey("nid1", id.PubKeyString())
	keys := validator.KnownPublicKeys()
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
	if keys["nid1"] != id.PubKeyString() {
		t.Error("key value mismatch")
	}
}

// ── DNS TXT ───────────────────────────────────────────────────────────────────

func TestParseNpsTxtRecord_Valid(t *testing.T) {
	txt := "v=nps1 nid=urn:nps:node:api.example.com:products type=memory port=17434"
	res := ndp.ParseNpsTxtRecord(txt, "api.example.com")
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Host != "api.example.com" {
		t.Errorf("Host = %q, want %q", res.Host, "api.example.com")
	}
	if res.Port != 17434 {
		t.Errorf("Port = %d, want 17434", res.Port)
	}
	if res.Protocol != "https" {
		t.Errorf("Protocol = %q, want https", res.Protocol)
	}
}

func TestParseNpsTxtRecord_MissingV(t *testing.T) {
	txt := "nid=urn:nps:node:api.example.com:products port=17433"
	res := ndp.ParseNpsTxtRecord(txt, "api.example.com")
	if res != nil {
		t.Error("expected nil when v= is absent")
	}
}

func TestParseNpsTxtRecord_WrongV(t *testing.T) {
	txt := "v=nps2 nid=urn:nps:node:api.example.com:products"
	res := ndp.ParseNpsTxtRecord(txt, "api.example.com")
	if res != nil {
		t.Error("expected nil when v != nps1")
	}
}

func TestParseNpsTxtRecord_MissingNid(t *testing.T) {
	txt := "v=nps1 type=memory port=17433"
	res := ndp.ParseNpsTxtRecord(txt, "api.example.com")
	if res != nil {
		t.Error("expected nil when nid is absent")
	}
}

func TestParseNpsTxtRecord_DefaultPort(t *testing.T) {
	txt := "v=nps1 nid=urn:nps:node:api.example.com:products"
	res := ndp.ParseNpsTxtRecord(txt, "api.example.com")
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Port != 17433 {
		t.Errorf("Port = %d, want 17433 (default)", res.Port)
	}
}

func TestParseNpsTxtRecord_WithFingerprint(t *testing.T) {
	txt := "v=nps1 nid=urn:nps:node:api.example.com:products fp=sha256:a3f9deadbeef"
	res := ndp.ParseNpsTxtRecord(txt, "api.example.com")
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.CertFingerprint != "sha256:a3f9deadbeef" {
		t.Errorf("CertFingerprint = %q, want %q", res.CertFingerprint, "sha256:a3f9deadbeef")
	}
}

func TestExtractHostFromTarget(t *testing.T) {
	cases := []struct {
		target string
		want   string
	}{
		{"nwp://api.example.com/products", "api.example.com"},
		{"nwp://api.example.com/products/sub", "api.example.com"},
		{"nwp://api.example.com", "api.example.com"},
		{"http://api.example.com/products", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := ndp.ExtractHostFromTarget(tc.target)
		if got != tc.want {
			t.Errorf("ExtractHostFromTarget(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestResolveViaDns_RegistryHit(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()
	reg.Announce(makeAnnounce("urn:nps:node:example.com:agent", 300))

	mock := &mockDnsTxtLookup{records: []string{"v=nps1 nid=urn:nps:node:example.com:agent"}}
	res := reg.ResolveViaDns(context.Background(), "nwp://example.com/agent", mock)
	if res == nil {
		t.Fatal("expected resolve result from registry")
	}
	if mock.called {
		t.Error("DNS lookup should not be called when registry has a hit")
	}
}

func TestResolveViaDns_DnsFallback(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry() // empty registry

	txt := "v=nps1 nid=urn:nps:node:api.example.com:products port=17434"
	mock := &mockDnsTxtLookup{records: []string{txt}}
	res := reg.ResolveViaDns(context.Background(), "nwp://api.example.com/products", mock)
	if res == nil {
		t.Fatal("expected non-nil result from DNS fallback")
	}
	if res.Host != "api.example.com" {
		t.Errorf("Host = %q, want api.example.com", res.Host)
	}
	if res.Port != 17434 {
		t.Errorf("Port = %d, want 17434", res.Port)
	}
	if !mock.called {
		t.Error("DNS lookup should have been called on registry miss")
	}
}

func TestResolveViaDns_InvalidTxt(t *testing.T) {
	reg := ndp.NewInMemoryNdpRegistry()

	mock := &mockDnsTxtLookup{records: []string{"v=nps2 nid=urn:nps:node:api.example.com:products"}}
	res := reg.ResolveViaDns(context.Background(), "nwp://api.example.com/products", mock)
	if res != nil {
		t.Error("expected nil when all TXT records are invalid")
	}
}
