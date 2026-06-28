// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labacacia/NPS-sdk-go/core"
	"github.com/labacacia/NPS-sdk-go/nip"
)

// ── NipIdentity ───────────────────────────────────────────────────────────────

func TestGenerate_Unique(t *testing.T) {
	id1, err := nip.Generate()
	if err != nil {
		t.Fatal(err)
	}
	id2, err := nip.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if id1.PubKeyString() == id2.PubKeyString() {
		t.Error("two generated identities should have different keys")
	}
}

func TestPubKeyString_Format(t *testing.T) {
	id, _ := nip.Generate()
	pks := id.PubKeyString()
	if !strings.HasPrefix(pks, "ed25519:") {
		t.Errorf("PubKeyString should start with 'ed25519:', got %q", pks)
	}
	hex := pks[8:]
	if len(hex) != 64 {
		t.Errorf("hex-encoded public key should be 64 chars, got %d", len(hex))
	}
}

func TestSign_Verify_Valid(t *testing.T) {
	id, _ := nip.Generate()
	payload := core.FrameDict{"nid": "test-node", "pub_key": id.PubKeyString()}
	sig := id.Sign(payload)
	if !strings.HasPrefix(sig, "ed25519:") {
		t.Errorf("signature should start with 'ed25519:', got %q", sig)
	}
	if !id.Verify(payload, sig) {
		t.Error("Verify should return true for own signature")
	}
}

func TestVerify_Tampered(t *testing.T) {
	id, _ := nip.Generate()
	payload := core.FrameDict{"nid": "original"}
	sig := id.Sign(payload)
	tampered := core.FrameDict{"nid": "tampered"}
	if id.Verify(tampered, sig) {
		t.Error("Verify should return false for tampered payload")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	id1, _ := nip.Generate()
	id2, _ := nip.Generate()
	payload := core.FrameDict{"nid": "node"}
	sig := id1.Sign(payload)
	if id2.Verify(payload, sig) {
		t.Error("Verify should return false for wrong key")
	}
}

func TestVerifyWithPubKeyStr_BadPrefix(t *testing.T) {
	id, _ := nip.Generate()
	payload := core.FrameDict{"nid": "node"}
	sig := id.Sign(payload)
	if nip.VerifyWithPubKeyStr(payload, "rsa:"+id.PubKeyString()[8:], sig) {
		t.Error("should reject non-ed25519 prefix")
	}
}

func TestSign_CanonicalOrder_Independent(t *testing.T) {
	id, _ := nip.Generate()
	p1 := core.FrameDict{"z": "last", "a": "first", "m": "middle"}
	p2 := core.FrameDict{"a": "first", "m": "middle", "z": "last"}
	sig1 := id.Sign(p1)
	if !id.Verify(p2, sig1) {
		t.Error("signature should be order-independent (canonical JSON)")
	}
}

func TestSave_Load(t *testing.T) {
	id, _ := nip.Generate()
	dir := t.TempDir()
	path := filepath.Join(dir, "id.json")
	if err := id.Save(path, "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	loaded, err := nip.Load(path, "s3cr3t")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PubKeyString() != id.PubKeyString() {
		t.Error("loaded identity has different public key")
	}
	// Ensure loaded key can verify original's signatures
	payload := core.FrameDict{"test": "value"}
	sig := id.Sign(payload)
	if !loaded.Verify(payload, sig) {
		t.Error("loaded identity should verify original signature")
	}
}

func TestLoad_WrongPassphrase(t *testing.T) {
	id, _ := nip.Generate()
	dir := t.TempDir()
	path := filepath.Join(dir, "id.json")
	id.Save(path, "correct")
	_, err := nip.Load(path, "wrong")
	if err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := nip.Load("/nonexistent/path/id.json", "pass")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSave_Permissions(t *testing.T) {
	id, _ := nip.Generate()
	dir := t.TempDir()
	path := filepath.Join(dir, "id.json")
	id.Save(path, "pass")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", info.Mode().Perm())
	}
}

// ── NIP Frames ────────────────────────────────────────────────────────────────

func TestIdentFrame_Roundtrip(t *testing.T) {
	id, _ := nip.Generate()
	sig := "ed25519:abc123"
	f := &nip.IdentFrame{
		NID:       "urn:nps:node:example.com:agent",
		PubKey:    id.PubKeyString(),
		Meta:      map[string]any{"version": "1"},
		Signature: &sig,
	}
	d := f.ToDict()
	f2 := nip.IdentFrameFromDict(d)
	if f2.NID != f.NID {
		t.Errorf("NID mismatch")
	}
	if f2.PubKey != f.PubKey {
		t.Errorf("PubKey mismatch")
	}
	if f2.Signature == nil || *f2.Signature != sig {
		t.Errorf("Signature mismatch")
	}
}

func TestIdentFrame_UnsignedDict_NoSignature(t *testing.T) {
	f := &nip.IdentFrame{NID: "n1", PubKey: "ed25519:abcd"}
	d := f.UnsignedDict()
	if _, ok := d["signature"]; ok {
		t.Error("UnsignedDict should not contain 'signature'")
	}
}

func TestTrustFrame_Roundtrip(t *testing.T) {
	f := &nip.TrustFrame{
		GrantorNID: "urn:nps:org:org-a.com",
		GranteeCA:  "urn:nps:org:org-b.com",
		TrustScope: []string{"nwp:query", "nwp:action"},
		Nodes:      []string{"nwp://api.org-a.com/public/**"},
		IssuedAt:   "2026-05-11T00:00:00Z",
		ExpiresAt:  "2026-12-31T00:00:00Z",
		Serial:     "00000000000A3F9C",
		SignerNID:  "urn:nps:org:org-a.com",
		Signature:  "ed25519:xyz",
	}
	d := f.ToDict()
	f2 := nip.TrustFrameFromDict(d)
	if f2.GrantorNID != f.GrantorNID {
		t.Errorf("GrantorNID mismatch")
	}
	if len(f2.TrustScope) != 2 || f2.TrustScope[0] != "nwp:query" {
		t.Errorf("TrustScope mismatch: %v", f2.TrustScope)
	}
	if len(f2.Nodes) != 1 || f2.Nodes[0] != "nwp://api.org-a.com/public/**" {
		t.Errorf("Nodes mismatch: %v", f2.Nodes)
	}
	if f2.Serial != f.Serial || f2.SignerNID != f.SignerNID {
		t.Errorf("audit fields mismatch: %#v", f2)
	}
	if _, ok := f.UnsignedDict()["signature"]; ok {
		t.Error("UnsignedDict should not contain signature")
	}
}

func TestRevokeFrame_Roundtrip(t *testing.T) {
	serial := "0x0A3F9C"
	parent := "urn:nps:agent:ca.example.com:group-1"
	f := &nip.RevokeFrame{
		TargetNID: "urn:nps:agent:ca.example.com:session-1",
		Serial:    &serial,
		Reason:    "parent_revoked",
		RevokedAt: "2026-01-01T00:00:00Z",
		ParentNID: &parent,
		SignerNID: "urn:nps:org:ca.example.com",
		Signature: "ed25519:sig",
	}
	d := f.ToDict()
	f2 := nip.RevokeFrameFromDict(d)
	if f2.TargetNID != f.TargetNID {
		t.Errorf("TargetNID mismatch")
	}
	if f2.Serial == nil || *f2.Serial != serial {
		t.Errorf("Serial mismatch")
	}
	if f2.ParentNID == nil || *f2.ParentNID != parent {
		t.Errorf("ParentNID mismatch")
	}
	if f2.Reason != "parent_revoked" || f2.SignerNID != "urn:nps:org:ca.example.com" {
		t.Errorf("Revoke fields mismatch: %#v", f2)
	}
	if _, ok := f.UnsignedDict()["signature"]; ok {
		t.Error("UnsignedDict should not contain signature")
	}
}

func TestRevokeFrame_ValidateParentNIDRule(t *testing.T) {
	parent := "urn:nps:agent:ca.example.com:group-1"
	cases := []struct {
		name      string
		reason    string
		parentNID *string
		wantErr   bool
	}{
		{"parent_revoked with parent", "parent_revoked", &parent, false},
		{"parent_revoked missing parent", "parent_revoked", nil, true},
		{"non-parent reason with parent", "key_compromise", &parent, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := &nip.RevokeFrame{
				TargetNID: "urn:nps:agent:ca.example.com:session-1",
				Reason:    tc.reason,
				RevokedAt: "2026-06-01T00:00:00Z",
				ParentNID: tc.parentNID,
				SignerNID: "urn:nps:org:ca.example.com",
				Signature: "ed25519:sig",
			}
			err := frame.Validate()
			if tc.wantErr && (err == nil || !strings.Contains(err.Error(), nip.ErrRevokeFrameInvalid)) {
				t.Fatalf("want %s error, got %v", nip.ErrRevokeFrameInvalid, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
