// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core_test

import (
	"testing"
	"time"

	"github.com/labacacia/nps/impl/go/core"
)

// ── FrameType ─────────────────────────────────────────────────────────────────

func TestFrameTypeFromByte_known(t *testing.T) {
	cases := []struct {
		b  byte
		ft core.FrameType
	}{
		{0x01, core.FrameTypeAnchor},
		{0x02, core.FrameTypeDiff},
		{0x03, core.FrameTypeStream},
		{0x04, core.FrameTypeCaps},
		{0x10, core.FrameTypeQuery},
		{0x11, core.FrameTypeAction},
		{0x20, core.FrameTypeIdent},
		{0x21, core.FrameTypeTrust},
		{0x22, core.FrameTypeRevoke},
		{0x30, core.FrameTypeAnnounce},
		{0x31, core.FrameTypeResolve},
		{0x32, core.FrameTypeGraph},
		{0x40, core.FrameTypeTask},
		{0x41, core.FrameTypeDelegate},
		{0x42, core.FrameTypeSync},
		{0x43, core.FrameTypeAlignStream},
		{0xFE, core.FrameTypeError},
	}
	for _, tc := range cases {
		ft, err := core.FrameTypeFromByte(tc.b)
		if err != nil {
			t.Errorf("byte 0x%02X: unexpected error %v", tc.b, err)
		}
		if ft != tc.ft {
			t.Errorf("byte 0x%02X: got 0x%02X want 0x%02X", tc.b, ft, tc.ft)
		}
	}
}

func TestFrameTypeFromByte_unknown(t *testing.T) {
	_, err := core.FrameTypeFromByte(0x99)
	if err == nil {
		t.Fatal("expected error for unknown frame type")
	}
}

// ── FrameHeader ───────────────────────────────────────────────────────────────

func TestFrameHeader_DefaultRoundtrip(t *testing.T) {
	hdr := core.NewFrameHeader(core.FrameTypeAnchor, core.EncodingTierJSON, true, 1234)
	if hdr.IsExtended {
		t.Fatal("should not be extended for payload < 64KiB")
	}
	if hdr.HeaderSize() != 4 {
		t.Fatalf("expected header size 4, got %d", hdr.HeaderSize())
	}
	wire := hdr.ToBytes()
	if len(wire) != 4 {
		t.Fatalf("expected 4-byte header, got %d", len(wire))
	}
	parsed, err := core.ParseFrameHeader(wire)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.FrameType != core.FrameTypeAnchor {
		t.Errorf("frame type mismatch")
	}
	if parsed.PayloadLength != 1234 {
		t.Errorf("payload length mismatch: got %d", parsed.PayloadLength)
	}
	if !parsed.IsFinal() {
		t.Error("expected IsFinal=true")
	}
	if parsed.EncodingTier() != core.EncodingTierJSON {
		t.Error("expected JSON tier")
	}
}

func TestFrameHeader_ExtendedRoundtrip(t *testing.T) {
	payloadLen := uint64(70000) // > 64KiB
	hdr := core.NewFrameHeader(core.FrameTypeStream, core.EncodingTierMsgPack, false, payloadLen)
	if !hdr.IsExtended {
		t.Fatal("should be extended for payload > 64KiB")
	}
	if hdr.HeaderSize() != 8 {
		t.Fatalf("expected header size 8, got %d", hdr.HeaderSize())
	}
	wire := hdr.ToBytes()
	if len(wire) != 8 {
		t.Fatalf("expected 8-byte header, got %d", len(wire))
	}
	parsed, err := core.ParseFrameHeader(wire)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PayloadLength != payloadLen {
		t.Errorf("payload length mismatch: got %d want %d", parsed.PayloadLength, payloadLen)
	}
	if parsed.EncodingTier() != core.EncodingTierMsgPack {
		t.Error("expected MsgPack tier")
	}
	if parsed.IsFinal() {
		t.Error("expected IsFinal=false")
	}
}

func TestFrameHeader_TooShort(t *testing.T) {
	_, err := core.ParseFrameHeader([]byte{0x01, 0x40})
	if err == nil {
		t.Fatal("expected error for too-short buffer")
	}
}

// ── FrameRegistry ─────────────────────────────────────────────────────────────

func TestFrameRegistry_Default(t *testing.T) {
	reg := core.CreateDefaultRegistry()
	ncpTypes := []core.FrameType{
		core.FrameTypeAnchor, core.FrameTypeDiff,
		core.FrameTypeStream, core.FrameTypeCaps, core.FrameTypeError,
	}
	for _, ft := range ncpTypes {
		if !reg.IsRegistered(ft) {
			t.Errorf("expected 0x%02X to be registered in default registry", ft)
		}
	}
	if reg.IsRegistered(core.FrameTypeTask) {
		t.Error("Task frame should not be in default (NCP-only) registry")
	}
}

func TestFrameRegistry_Full(t *testing.T) {
	reg := core.CreateFullRegistry()
	all := []core.FrameType{
		core.FrameTypeAnchor, core.FrameTypeDiff, core.FrameTypeStream, core.FrameTypeCaps,
		core.FrameTypeQuery, core.FrameTypeAction,
		core.FrameTypeIdent, core.FrameTypeTrust, core.FrameTypeRevoke,
		core.FrameTypeAnnounce, core.FrameTypeResolve, core.FrameTypeGraph,
		core.FrameTypeTask, core.FrameTypeDelegate, core.FrameTypeSync, core.FrameTypeAlignStream,
		core.FrameTypeError,
	}
	for _, ft := range all {
		if !reg.IsRegistered(ft) {
			t.Errorf("expected 0x%02X to be in full registry", ft)
		}
	}
}

// ── NpsFrameCodec ─────────────────────────────────────────────────────────────

func TestCodec_JSONRoundtrip(t *testing.T) {
	reg := core.CreateFullRegistry()
	codec := core.NewNpsFrameCodec(reg)
	dict := core.FrameDict{"anchor_id": "test", "ttl": int64(3600)}
	wire, err := codec.Encode(core.FrameTypeAnchor, dict, core.EncodingTierJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	ft, got, err := codec.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if ft != core.FrameTypeAnchor {
		t.Errorf("frame type mismatch")
	}
	if got["anchor_id"] != "test" {
		t.Errorf("anchor_id mismatch")
	}
}

func TestCodec_MsgPackRoundtrip(t *testing.T) {
	reg := core.CreateFullRegistry()
	codec := core.NewNpsFrameCodec(reg)
	dict := core.FrameDict{"node_id": "n1", "caps": []any{"nwp", "nop"}}
	wire, err := codec.Encode(core.FrameTypeCaps, dict, core.EncodingTierMsgPack, false)
	if err != nil {
		t.Fatal(err)
	}
	ft, got, err := codec.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if ft != core.FrameTypeCaps {
		t.Errorf("frame type mismatch")
	}
	if got["node_id"] != "n1" {
		t.Errorf("node_id mismatch: got %v", got["node_id"])
	}
}

func TestCodec_UnregisteredFrame(t *testing.T) {
	reg := core.CreateDefaultRegistry()
	codec := core.NewNpsFrameCodec(reg)
	// Encode succeeds (codec doesn't validate ft on encode)
	dict := core.FrameDict{"task_id": "t1"}
	wire, _ := core.NewNpsFrameCodec(core.CreateFullRegistry()).Encode(
		core.FrameTypeTask, dict, core.EncodingTierJSON, true)
	// Default registry doesn't know Task
	_, _, err := codec.Decode(wire)
	if err == nil {
		t.Fatal("expected error for unregistered frame type")
	}
}

// ── AnchorFrameCache ──────────────────────────────────────────────────────────

func TestAnchorCache_SetAndGet(t *testing.T) {
	cache := core.NewAnchorFrameCache()
	schema := core.FrameDict{"type": "object", "version": "1"}
	id, err := cache.Set(schema, 60)
	if err != nil {
		t.Fatal(err)
	}
	got := cache.Get(id)
	if got == nil {
		t.Fatal("expected cached schema")
	}
	if got["version"] != "1" {
		t.Errorf("version mismatch")
	}
}

func TestAnchorCache_Expiry(t *testing.T) {
	now := time.Unix(1000, 0)
	cache := core.NewAnchorFrameCache()
	cache.Clock = func() time.Time { return now }

	schema := core.FrameDict{"type": "string"}
	id, _ := cache.Set(schema, 10) // 10s TTL

	// advance past TTL
	cache.Clock = func() time.Time { return now.Add(15 * time.Second) }
	if cache.Get(id) != nil {
		t.Error("expected nil after TTL expiry")
	}
	if cache.Len() != 0 {
		t.Error("expected Len=0 after expiry")
	}
}

func TestAnchorCache_GetRequired_Missing(t *testing.T) {
	cache := core.NewAnchorFrameCache()
	_, err := cache.GetRequired("sha256:nonexistent")
	if err == nil {
		t.Fatal("expected ErrAnchorNotFound")
	}
}

func TestAnchorCache_PoisonDetection(t *testing.T) {
	cache := core.NewAnchorFrameCache()
	schema1 := core.FrameDict{"type": "object"}
	id, _ := cache.Set(schema1, 60)

	// ComputeAnchorID produces unique IDs per distinct schema, so no hash collision
	// in practice. Test the safe path: same schema twice is not poison.
	_ = id
	_, err := cache.Set(schema1, 60)
	if err != nil {
		t.Errorf("re-setting same schema should not poison: %v", err)
	}
}

func TestAnchorCache_Invalidate(t *testing.T) {
	cache := core.NewAnchorFrameCache()
	schema := core.FrameDict{"type": "object"}
	id, _ := cache.Set(schema, 60)
	cache.Invalidate(id)
	if cache.Get(id) != nil {
		t.Error("expected nil after invalidation")
	}
}

func TestAnchorCache_ComputeAnchorID_Deterministic(t *testing.T) {
	a := core.FrameDict{"z": "last", "a": "first"}
	b := core.FrameDict{"a": "first", "z": "last"}
	if core.ComputeAnchorID(a) != core.ComputeAnchorID(b) {
		t.Error("AnchorID should be order-independent")
	}
}
