// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	npsnip "github.com/labacacia/NPS-sdk-go/nip"
)

func newTestCA(t *testing.T) (*npsnip.NipCaService, *npsnip.InMemoryNipCaStore) {
	t.Helper()
	keys, err := npsnip.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	store := npsnip.NewInMemoryNipCaStore()
	opts := npsnip.DefaultNipCaOptions("urn:nps:org:ca.example.com", "https://ca.example.com")
	return npsnip.NewNipCaService(opts, store, keys), store
}

func freshPubKey(t *testing.T) string {
	t.Helper()
	id, err := npsnip.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return id.PubKeyString()
}

func TestRegisterThenVerify(t *testing.T) {
	ca, _ := newTestCA(t)
	frame, err := ca.Register("agent", "alice", freshPubKey(t), []string{"chat"}, `{"nodes":["*"]}`, "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if frame.NID != "urn:nps:agent:ca.example.com:alice" {
		t.Fatalf("unexpected NID: %s", frame.NID)
	}
	if frame.IssuedBy != "urn:nps:org:ca.example.com" {
		t.Fatalf("issued_by mismatch: %s", frame.IssuedBy)
	}
	if frame.Signature == nil || *frame.Signature == "" {
		t.Fatal("missing signature")
	}
	r := ca.Verify(frame.NID)
	if !r.Valid {
		t.Fatalf("verify should pass: %s %s", r.ErrorCode, r.Message)
	}
}

func TestVerifyNidNotFound(t *testing.T) {
	ca, _ := newTestCA(t)
	r := ca.Verify("urn:nps:agent:ca.example.com:ghost")
	if r.Valid || r.ErrorCode != npsnip.ErrCaNidNotFound {
		t.Fatalf("expected NID-NOT-FOUND, got %+v", r)
	}
}

func TestDuplicateNid(t *testing.T) {
	ca, _ := newTestCA(t)
	pk := freshPubKey(t)
	if _, err := ca.Register("agent", "dup", pk, nil, "{}", ""); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := ca.Register("agent", "dup", pk, nil, "{}", "")
	caErr, ok := err.(*npsnip.NipCaError)
	if !ok || caErr.ErrorCode != npsnip.ErrCaNidAlreadyExists {
		t.Fatalf("expected NID-ALREADY-EXISTS, got %v", err)
	}
}

func TestRenewalWindow(t *testing.T) {
	// Too early: fresh 30-day cert with 7-day window cannot renew yet.
	keys, _ := npsnip.Generate()
	store := npsnip.NewInMemoryNipCaStore()
	opts := npsnip.DefaultNipCaOptions("urn:nps:org:ca.example.com", "https://ca.example.com")
	ca := npsnip.NewNipCaService(opts, store, keys)
	frame, err := ca.Register("agent", "renewme", freshPubKey(t), nil, "{}", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := ca.Renew(frame.NID); err == nil {
		t.Fatal("expected renewal-too-early")
	} else if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrCaRenewalTooEarly {
		t.Fatalf("expected RENEWAL-TOO-EARLY, got %v", err)
	}

	// Open window: RenewalWindowDays >= validity means the window is already open.
	opts2 := npsnip.DefaultNipCaOptions("urn:nps:org:ca.example.com", "https://ca.example.com")
	opts2.RenewalWindowDays = 60
	ca2 := npsnip.NewNipCaService(opts2, npsnip.NewInMemoryNipCaStore(), keys)
	f2, err := ca2.Register("agent", "renewme2", freshPubKey(t), nil, "{}", "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	renewed, err := ca2.Renew(f2.NID)
	if err != nil {
		t.Fatalf("renew should succeed: %v", err)
	}
	if renewed.Serial == f2.Serial {
		t.Fatal("renewed cert should have a fresh serial")
	}
}

func TestRevokeAndVerify(t *testing.T) {
	ca, _ := newTestCA(t)
	frame, _ := ca.Register("agent", "bob", freshPubKey(t), nil, "{}", "")
	rf, err := ca.Revoke(frame.NID, "key_compromise")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if rf.TargetNID != frame.NID || rf.Reason != "key_compromise" || rf.Signature == "" {
		t.Fatalf("bad revoke frame: %+v", rf)
	}
	r := ca.Verify(frame.NID)
	if r.Valid || r.ErrorCode != npsnip.ErrCertRevoked {
		t.Fatalf("expected CERT-REVOKED, got %+v", r)
	}
	crl, _ := ca.GetCrl()
	if len(crl) != 1 {
		t.Fatalf("expected 1 CRL entry, got %d", len(crl))
	}
}

func TestGroupRegisterAndIssueSession(t *testing.T) {
	ca, _ := newTestCA(t)
	group, err := ca.RegisterGroup("group-orch1", freshPubKey(t),
		[]string{"chat", "search", "code"}, `{"nodes":["*"]}`, "user-42", "op-key-1", "")
	if err != nil {
		t.Fatalf("RegisterGroup: %v", err)
	}
	if group.NID != "urn:nps:agent:ca.example.com:group-orch1" {
		t.Fatalf("group NID: %s", group.NID)
	}

	// Session inherits group caps + scope by default; clamped validity.
	sess, err := ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{
		Validity: 2 * time.Hour,
		Purpose:  "batch",
	})
	if err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	if sess.NID == "" || len(sess.Capabilities) != 3 {
		t.Fatalf("session caps not inherited: %+v", sess.Capabilities)
	}

	// Capability subset enforcement: cannot exceed the group.
	_, err = ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{
		Capabilities: []string{"chat", "admin"},
	})
	if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrCaScopeExpansionDenied {
		t.Fatalf("expected SCOPE-EXPANSION-DENIED, got %v", err)
	}

	// Valid subset accepted.
	sub, err := ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{
		Capabilities: []string{"chat"},
	})
	if err != nil || len(sub.Capabilities) != 1 {
		t.Fatalf("subset session failed: %v %+v", err, sub)
	}

	// Validity clamp: below min rejected.
	_, err = ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{
		Validity: 10 * time.Second,
	})
	if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrCaSessionValidityInvalid {
		t.Fatalf("expected SESSION-VALIDITY-INVALID (too short), got %v", err)
	}
	// Above max rejected.
	_, err = ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{
		Validity: 48 * time.Hour,
	})
	if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrCaSessionValidityInvalid {
		t.Fatalf("expected SESSION-VALIDITY-INVALID (too long), got %v", err)
	}
}

func TestRevokeGroupCascade(t *testing.T) {
	ca, _ := newTestCA(t)
	group, _ := ca.RegisterGroup("group-x", freshPubKey(t), []string{"chat"}, "{}", "", "", "")
	s1, _ := ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{})
	s2, _ := ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{})

	if _, err := ca.Revoke(group.NID, "ca_compromise"); err != nil {
		t.Fatalf("revoke group: %v", err)
	}
	for _, s := range []string{s1.NID, s2.NID} {
		r := ca.Verify(s)
		if r.Valid {
			t.Fatalf("session %s should be revoked via cascade", s)
		}
		if r.ErrorCode != npsnip.ErrCertRevoked && r.ErrorCode != npsnip.ErrCertParentRevoked {
			t.Fatalf("unexpected cascade error for %s: %s", s, r.ErrorCode)
		}
	}
	// The group itself no longer issues sessions.
	if _, err := ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{}); err == nil {
		t.Fatal("expected group-revoked on issue after cascade")
	}
	// CRL contains group + both sessions.
	crl, _ := ca.GetCrl()
	if len(crl) != 3 {
		t.Fatalf("expected 3 revoked (group+2 sessions), got %d", len(crl))
	}
}

func TestSessionCapabilitiesSignedInFrame(t *testing.T) {
	ca, _ := newTestCA(t)
	group, _ := ca.RegisterGroup("group-sig", freshPubKey(t), []string{"chat"}, "{}", "owner-1", "", "")
	sess, err := ca.IssueSession(group.NID, freshPubKey(t), npsnip.IssueSessionParams{Purpose: "p"})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Session frame must carry a signature and inherit the owner via lineage.
	if sess.Signature == nil {
		t.Fatal("session frame unsigned")
	}
	rec, _ := ca.GetCert(sess.NID)
	if rec == nil || rec.ParentNid == nil || *rec.ParentNid != group.NID {
		t.Fatalf("session parent not recorded: %+v", rec)
	}
	if rec.LineageJson == nil {
		t.Fatal("session lineage not persisted")
	}
	var lin map[string]any
	_ = json.Unmarshal([]byte(*rec.LineageJson), &lin)
	if lin["owner_user_id"] != "owner-1" || lin["role"] != "session" {
		t.Fatalf("lineage not propagated: %+v", lin)
	}
}

func TestRegisterX509(t *testing.T) {
	ca, _ := newTestCA(t)
	frame, err := ca.RegisterX509("agent", "x1", freshPubKey(t), []string{"chat"}, "{}",
		npsnip.AssuranceAttested, "", nil)
	if err != nil {
		t.Fatalf("RegisterX509: %v", err)
	}
	if frame.CertFormat == nil || *frame.CertFormat != npsnip.CertFormatV2X509 {
		t.Fatalf("cert_format not v2-x509: %+v", frame.CertFormat)
	}
	if len(frame.CertChain) != 2 {
		t.Fatalf("expected [leaf, root] chain, got %d", len(frame.CertChain))
	}
	if _, err := base64.RawURLEncoding.DecodeString(frame.CertChain[0]); err != nil {
		t.Fatalf("chain not base64url DER: %v", err)
	}
	if frame.AssuranceLevel == nil || frame.AssuranceLevel.Wire != "attested" {
		t.Fatalf("assurance level missing: %+v", frame.AssuranceLevel)
	}
}

// ── RA tiers ─────────────────────────────────────────────────────────────────

func TestRaTierAllowlist(t *testing.T) {
	opts := npsnip.DefaultNipCaOptions("urn:nps:org:ca.example.com", "https://ca.example.com")
	opts.EnrollmentTier = npsnip.EnrollmentTierAllowlist
	opts.EnrollmentAllowlistPatterns = []string{"svc-*"}
	keys, _ := npsnip.Generate()
	ca := npsnip.NewNipCaService(opts, npsnip.NewInMemoryNipCaStore(), keys)
	policy, err := npsnip.CreateEnrollmentPolicy(opts, nil, nil)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if _, err := ca.RegisterWithRa("agent", "svc-1", freshPubKey(t), nil, "{}", "", "", policy); err != nil {
		t.Fatalf("allowed identifier rejected: %v", err)
	}
	_, err = ca.RegisterWithRa("agent", "other", freshPubKey(t), nil, "{}", "", "", policy)
	if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrRaNidNotAllowed {
		t.Fatalf("expected RA-NID-NOT-ALLOWED, got %v", err)
	}
}

func TestRaTierBootstrapToken(t *testing.T) {
	opts := npsnip.DefaultNipCaOptions("urn:nps:org:ca.example.com", "https://ca.example.com")
	opts.EnrollmentTier = npsnip.EnrollmentTierBootstrapToken
	keys, _ := npsnip.Generate()
	ca := npsnip.NewNipCaService(opts, npsnip.NewInMemoryNipCaStore(), keys)
	tokStore := npsnip.NewInMemoryBootstrapTokenStore()
	policy, err := npsnip.CreateEnrollmentPolicy(opts, tokStore, nil)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}

	// No token → invalid.
	_, err = ca.RegisterWithRa("agent", "a", freshPubKey(t), nil, "{}", "", "", policy)
	if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrRaTokenInvalid {
		t.Fatalf("expected RA-TOKEN-INVALID, got %v", err)
	}

	tok, _ := tokStore.Create("test", time.Now().Add(time.Hour))
	if _, err := ca.RegisterWithRa("agent", "a", freshPubKey(t), nil, "{}", "", tok, policy); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	// Single-use: replay fails.
	_, err = ca.RegisterWithRa("agent", "b", freshPubKey(t), nil, "{}", "", tok, policy)
	if caErr, ok := err.(*npsnip.NipCaError); !ok || caErr.ErrorCode != npsnip.ErrRaTokenExpired {
		t.Fatalf("expected RA-TOKEN-EXPIRED on replay, got %v", err)
	}
}

func TestRaTierPendingQueue(t *testing.T) {
	opts := npsnip.DefaultNipCaOptions("urn:nps:org:ca.example.com", "https://ca.example.com")
	opts.EnrollmentTier = npsnip.EnrollmentTierPendingQueue
	keys, _ := npsnip.Generate()
	ca := npsnip.NewNipCaService(opts, npsnip.NewInMemoryNipCaStore(), keys)
	pending := npsnip.NewInMemoryPendingStore(opts.PendingQueueMaxAge)
	policy, err := npsnip.CreateEnrollmentPolicy(opts, nil, pending)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	_, err = ca.RegisterWithRa("agent", "q1", freshPubKey(t), nil, "{}", "", "", policy)
	pend, ok := err.(*npsnip.NipRaPendingError)
	if !ok || pend.PendingID == "" {
		t.Fatalf("expected NipRaPendingError, got %v", err)
	}
	if pending.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", pending.PendingCount())
	}
	if ok, _ := pending.Approve(pend.PendingID); !ok {
		t.Fatal("approve failed")
	}
	if pending.PendingCount() != 0 {
		t.Fatal("pending count should drop after approve")
	}
}

func TestGroupJwsVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	header := map[string]any{"alg": "EdDSA", "kid": "urn:nps:agent:ca.example.com:group-1", "nps-purpose": "session-issue"}
	payload := map[string]any{"session_pub_key": "ed25519:AAAA", "iat": time.Now().Unix()}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	protectedB64 := base64.RawURLEncoding.EncodeToString(hb)
	payloadB64 := base64.RawURLEncoding.EncodeToString(pb)
	sig := ed25519.Sign(priv, []byte(protectedB64+"."+payloadB64))
	jws := npsnip.FlattenedJws{
		Protected: protectedB64,
		Payload:   payloadB64,
		Signature: base64.RawURLEncoding.EncodeToString(sig),
	}

	pj, kid, ec, ok := npsnip.VerifyGroupJws(jws, pub)
	if !ok {
		t.Fatalf("verify should pass, code=%s", ec)
	}
	if kid != "urn:nps:agent:ca.example.com:group-1" {
		t.Fatalf("kid mismatch: %s", kid)
	}
	if pj == "" {
		t.Fatal("payload missing")
	}

	// Tampered signature → invalid.
	badPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, _, ec, ok := npsnip.VerifyGroupJws(jws, badPub); ok || ec != npsnip.ErrCaJwsInvalid {
		t.Fatalf("expected JWS-INVALID for wrong key, got ok=%v ec=%s", ok, ec)
	}

	// Wrong purpose → invalid.
	badHeader := map[string]any{"alg": "EdDSA", "kid": "k", "nps-purpose": "other"}
	bhb, _ := json.Marshal(badHeader)
	bp := base64.RawURLEncoding.EncodeToString(bhb)
	bsig := ed25519.Sign(priv, []byte(bp+"."+payloadB64))
	badJws := npsnip.FlattenedJws{Protected: bp, Payload: payloadB64, Signature: base64.RawURLEncoding.EncodeToString(bsig)}
	if _, _, ec, ok := npsnip.VerifyGroupJws(badJws, pub); ok || ec != npsnip.ErrCaJwsInvalid {
		t.Fatalf("expected JWS-INVALID for wrong purpose, got ok=%v ec=%s", ok, ec)
	}
}
