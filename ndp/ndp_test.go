// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp_test

import (
	"testing"
	"time"

	"github.com/labacacia/NPS-sdk-go/nip"
	"github.com/labacacia/NPS-sdk-go/ndp"
)

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

func TestAnnounceFrame_UnsignedDict_NoSignature(t *testing.T) {
	f := &ndp.AnnounceFrame{
		NID: "urn:nps:node:x.com:n", Caps: []string{},
		Addresses: []map[string]any{}, TTL: 300, Timestamp: "t",
	}
	d := f.UnsignedDict()
	if _, ok := d["signature"]; ok {
		t.Error("UnsignedDict should not contain 'signature'")
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
	nodes := []any{map[string]any{"nid": "n1"}}
	patch := []any{map[string]any{"op": "add", "nid": "n2"}}
	f := &ndp.GraphFrame{Seq: 42, InitialSync: true, Nodes: nodes, Patch: patch}
	d := f.ToDict()
	f2 := ndp.GraphFrameFromDict(d)
	if f2.Seq != 42 {
		t.Errorf("Seq mismatch")
	}
	if !f2.InitialSync {
		t.Error("InitialSync should be true")
	}
	if len(f2.Nodes) != 1 {
		t.Errorf("Nodes length mismatch")
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
	if result.ErrorCode != "NDP-ANNOUNCE-SIG-INVALID" {
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
