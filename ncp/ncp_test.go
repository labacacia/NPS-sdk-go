// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp_test

import (
	"testing"

	"github.com/labacacia/NPS-sdk-go/core"
	"github.com/labacacia/NPS-sdk-go/ncp"
)

// ── AnchorFrame ───────────────────────────────────────────────────────────────

func TestAnchorFrame_Roundtrip(t *testing.T) {
	ns := "com.example"
	desc := "User schema"
	nt := "complex"
	f := &ncp.AnchorFrame{
		AnchorID:    "sha256:abc",
		Schema:      core.FrameDict{"type": "object", "version": "1"},
		Namespace:   &ns,
		Description: &desc,
		NodeType:    &nt,
		TTL:         7200,
	}
	d := f.ToDict()
	f2 := ncp.AnchorFrameFromDict(d)
	if f2.AnchorID != "sha256:abc" {
		t.Errorf("AnchorID mismatch")
	}
	if f2.TTL != 7200 {
		t.Errorf("TTL mismatch: got %d", f2.TTL)
	}
	if f2.Namespace == nil || *f2.Namespace != ns {
		t.Errorf("Namespace mismatch")
	}
	if f2.Description == nil || *f2.Description != desc {
		t.Errorf("Description mismatch")
	}
	if f2.NodeType == nil || *f2.NodeType != nt {
		t.Errorf("NodeType mismatch")
	}
}

func TestAnchorFrame_DefaultTTL(t *testing.T) {
	d := core.FrameDict{"anchor_id": "sha256:x", "schema": map[string]any{}}
	f := ncp.AnchorFrameFromDict(d)
	if f.TTL != 3600 {
		t.Errorf("expected default TTL=3600, got %d", f.TTL)
	}
}

func TestAnchorFrame_FrameType(t *testing.T) {
	f := &ncp.AnchorFrame{}
	if f.FrameType() != core.FrameTypeAnchor {
		t.Errorf("wrong frame type: 0x%02X", f.FrameType())
	}
}

// ── DiffFrame ─────────────────────────────────────────────────────────────────

func TestDiffFrame_Roundtrip(t *testing.T) {
	patch := []any{map[string]any{"op": "replace", "path": "/version", "value": "2"}}
	f := &ncp.DiffFrame{
		AnchorID:    "sha256:old",
		NewAnchorID: "sha256:new",
		Patch:       patch,
	}
	d := f.ToDict()
	f2 := ncp.DiffFrameFromDict(d)
	if f2.AnchorID != "sha256:old" {
		t.Errorf("AnchorID mismatch")
	}
	if f2.NewAnchorID != "sha256:new" {
		t.Errorf("NewAnchorID mismatch")
	}
	if len(f2.Patch) != 1 {
		t.Errorf("Patch length mismatch")
	}
}

// ── StreamFrame ───────────────────────────────────────────────────────────────

func TestStreamFrame_Roundtrip(t *testing.T) {
	f := &ncp.StreamFrame{
		AnchorID: "sha256:abc",
		Seq:      5,
		Payload:  map[string]any{"chunk": "data"},
		IsLast:   true,
	}
	d := f.ToDict()
	f2 := ncp.StreamFrameFromDict(d)
	if f2.Seq != 5 {
		t.Errorf("Seq mismatch")
	}
	if !f2.IsLast {
		t.Error("IsLast should be true")
	}
}

// ── CapsFrame ─────────────────────────────────────────────────────────────────

func TestCapsFrame_Roundtrip(t *testing.T) {
	ref := "sha256:abc"
	f := &ncp.CapsFrame{
		NodeID:    "urn:nps:node:example.com:n1",
		Caps:      []string{"nwp", "nop", "nip"},
		AnchorRef: &ref,
		Payload:   map[string]any{"meta": "data"},
	}
	d := f.ToDict()
	f2 := ncp.CapsFrameFromDict(d)
	if f2.NodeID != f.NodeID {
		t.Errorf("NodeID mismatch")
	}
	if len(f2.Caps) != 3 {
		t.Errorf("Caps length mismatch: %v", f2.Caps)
	}
	if f2.AnchorRef == nil || *f2.AnchorRef != ref {
		t.Errorf("AnchorRef mismatch")
	}
}

// ── ErrorFrame ────────────────────────────────────────────────────────────────

func TestErrorFrame_Roundtrip(t *testing.T) {
	f := &ncp.ErrorFrame{
		ErrorCode: "NCP-FRAME-TOO-LARGE",
		Message:   "payload exceeds limit",
		Detail:    map[string]any{"max": 65535},
	}
	d := f.ToDict()
	f2 := ncp.ErrorFrameFromDict(d)
	if f2.ErrorCode != "NCP-FRAME-TOO-LARGE" {
		t.Errorf("ErrorCode mismatch")
	}
	if f2.Message != "payload exceeds limit" {
		t.Errorf("Message mismatch")
	}
	if f2.Detail == nil {
		t.Error("Detail should not be nil")
	}
}

// ── Codec integration with NCP frames ─────────────────────────────────────────

func TestCodec_AnchorFrame_MsgPack(t *testing.T) {
	reg := core.CreateFullRegistry()
	codec := core.NewNpsFrameCodec(reg)

	f := &ncp.AnchorFrame{
		AnchorID: "sha256:test",
		Schema:   core.FrameDict{"type": "object"},
		TTL:      3600,
	}
	wire, err := codec.Encode(f.FrameType(), f.ToDict(), core.EncodingTierMsgPack, true)
	if err != nil {
		t.Fatal(err)
	}
	ft, dict, err := codec.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if ft != core.FrameTypeAnchor {
		t.Errorf("frame type mismatch")
	}
	f2 := ncp.AnchorFrameFromDict(dict)
	if f2.AnchorID != "sha256:test" {
		t.Errorf("AnchorID mismatch after codec roundtrip")
	}
}
