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
	exp := "2026-12-31T00:00:00Z"
	sig := "ed25519:xyz"
	f := &nip.TrustFrame{
		IssuerNID:  "urn:nps:node:issuer.com:ca",
		SubjectNID: "urn:nps:node:agent.com:a1",
		Scopes:     []string{"read", "write"},
		ExpiresAt:  &exp,
		Signature:  &sig,
	}
	d := f.ToDict()
	f2 := nip.TrustFrameFromDict(d)
	if f2.IssuerNID != f.IssuerNID {
		t.Errorf("IssuerNID mismatch")
	}
	if len(f2.Scopes) != 2 || f2.Scopes[0] != "read" {
		t.Errorf("Scopes mismatch: %v", f2.Scopes)
	}
	if f2.ExpiresAt == nil || *f2.ExpiresAt != exp {
		t.Errorf("ExpiresAt mismatch")
	}
}

func TestRevokeFrame_Roundtrip(t *testing.T) {
	reason := "key_compromise"
	ts := "2026-01-01T00:00:00Z"
	f := &nip.RevokeFrame{NID: "urn:nps:node:example.com:old", Reason: &reason, RevokedAt: &ts}
	d := f.ToDict()
	f2 := nip.RevokeFrameFromDict(d)
	if f2.NID != f.NID {
		t.Errorf("NID mismatch")
	}
	if f2.Reason == nil || *f2.Reason != reason {
		t.Errorf("Reason mismatch")
	}
}
