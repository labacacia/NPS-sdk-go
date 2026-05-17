// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/labacacia/NPS-sdk-go/nip"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func makeKey(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func makeUnsigned(subjectNid string) nip.ReputationLogEntry {
	return nip.ReputationLogEntry{
		V:          1,
		LogID:      "urn:nps:org:log.test",
		Seq:        1,
		Timestamp:  "2026-01-01T00:00:00Z",
		SubjectNid: subjectNid,
		Incident:   nip.IncidentCertRevoked,
		Severity:   nip.SeverityInfo,
		IssuerNid:  "urn:nps:org:issuer.test",
		Signature:  "",
	}
}

// leafHash computes the Merkle leaf hash for an entry using the same scheme as
// VerifyInclusion: SHA-256( 0x00 || canonical_json(entry) ).
// We replicate the logic here because canonicalJSONMap is unexported.
func leafHash(t *testing.T, entry nip.ReputationLogEntry) []byte {
	t.Helper()
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("leafHash: marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("leafHash: unmarshal: %v", err)
	}
	canonical := canonicalJSONMapTest(t, m)
	input := append([]byte{0x00}, canonical...)
	h := sha256.Sum256(input)
	return h[:]
}

// nodeHashTest computes SHA-256( 0x01 || left || right ).
func nodeHashTest(left, right []byte) []byte {
	buf := make([]byte, 65)
	buf[0] = 0x01
	copy(buf[1:], left)
	copy(buf[33:], right)
	h := sha256.Sum256(buf)
	return h[:]
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// canonicalJSONMapTest replicates the unexported canonicalJSONMap logic.
func canonicalJSONMapTest(t *testing.T, m map[string]any) []byte {
	t.Helper()
	// collect and sort keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// simple insertion sort — small maps only
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			t.Fatalf("canonicalJSONMapTest: marshal key: %v", err)
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m[k])
		if err != nil {
			t.Fatalf("canonicalJSONMapTest: marshal val: %v", err)
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// ─── Part 1 — IncidentType constants ─────────────────────────────────────────

func TestIncidentType_WireStrings(t *testing.T) {
	cases := []struct {
		name string
		got  nip.IncidentType
		want string
	}{
		{"IncidentOther", nip.IncidentOther, "other"},
		{"IncidentCertRevoked", nip.IncidentCertRevoked, "cert-revoked"},
		{"IncidentRateLimitViolation", nip.IncidentRateLimitViolation, "rate-limit-violation"},
		{"IncidentTosViolation", nip.IncidentTosViolation, "tos-violation"},
		{"IncidentScrapingPattern", nip.IncidentScrapingPattern, "scraping-pattern"},
		{"IncidentPaymentDefault", nip.IncidentPaymentDefault, "payment-default"},
		{"IncidentContractDispute", nip.IncidentContractDispute, "contract-dispute"},
		{"IncidentImpersonationClaim", nip.IncidentImpersonationClaim, "impersonation-claim"},
		{"IncidentPositiveAttestation", nip.IncidentPositiveAttestation, "positive-attestation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", string(tc.got), tc.want)
			}
		})
	}
}

// ─── Part 2 — Severity JSON round-trip ───────────────────────────────────────

func TestSeverity_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		sev  nip.Severity
		wire string
	}{
		{nip.SeverityInfo, "info"},
		{nip.SeverityMinor, "minor"},
		{nip.SeverityModerate, "moderate"},
		{nip.SeverityMajor, "major"},
		{nip.SeverityCritical, "critical"},
	}
	for _, tc := range cases {
		t.Run(tc.wire, func(t *testing.T) {
			// marshal
			data, err := json.Marshal(tc.sev)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			want := `"` + tc.wire + `"`
			if string(data) != want {
				t.Errorf("marshal: got %s, want %s", data, want)
			}
			// unmarshal
			var s nip.Severity
			if err := json.Unmarshal(data, &s); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if s != tc.sev {
				t.Errorf("unmarshal: got %v, want %v", s, tc.sev)
			}
		})
	}
}

func TestSeverity_Ordering(t *testing.T) {
	if nip.SeverityInfo >= nip.SeverityMinor {
		t.Error("SeverityInfo must be < SeverityMinor")
	}
	if nip.SeverityMinor >= nip.SeverityModerate {
		t.Error("SeverityMinor must be < SeverityModerate")
	}
	if nip.SeverityModerate >= nip.SeverityMajor {
		t.Error("SeverityModerate must be < SeverityMajor")
	}
	if nip.SeverityMajor >= nip.SeverityCritical {
		t.Error("SeverityMajor must be < SeverityCritical")
	}
	if int(nip.SeverityInfo) != 0 {
		t.Errorf("SeverityInfo must equal 0, got %d", int(nip.SeverityInfo))
	}
	if int(nip.SeverityMinor) != 1 {
		t.Errorf("SeverityMinor must equal 1, got %d", int(nip.SeverityMinor))
	}
	if int(nip.SeverityModerate) != 2 {
		t.Errorf("SeverityModerate must equal 2, got %d", int(nip.SeverityModerate))
	}
	if int(nip.SeverityMajor) != 3 {
		t.Errorf("SeverityMajor must equal 3, got %d", int(nip.SeverityMajor))
	}
	if int(nip.SeverityCritical) != 4 {
		t.Errorf("SeverityCritical must equal 4, got %d", int(nip.SeverityCritical))
	}
}

func TestSeverity_UnmarshalUnknown(t *testing.T) {
	var s nip.Severity
	err := json.Unmarshal([]byte(`"legendary"`), &s)
	if err == nil {
		t.Error("expected error for unknown severity wire value, got nil")
	}
}

// ─── Part 3 — ReputationLogEntry JSON ────────────────────────────────────────

func TestReputationLogEntry_MarshalSnakeCase(t *testing.T) {
	entry := nip.ReputationLogEntry{
		V:              1,
		LogID:          "urn:nps:org:log.example",
		Seq:            42,
		Timestamp:      "2026-01-01T00:00:00Z",
		SubjectNid:     "urn:nps:org:subject.test",
		Incident:       nip.IncidentTosViolation,
		Severity:       nip.SeverityMajor,
		Window:         &nip.ObservationWindow{Start: "2026-01-01T00:00:00Z", End: "2026-01-02T00:00:00Z"},
		EvidenceRef:    "https://example.com/evidence",
		EvidenceSha256: "abc123",
		IssuerNid:      "urn:nps:org:issuer.test",
		Signature:      "ed25519:somesig",
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	snakeCaseKeys := []string{
		"v", "log_id", "seq", "timestamp", "subject_nid",
		"incident", "severity", "window", "evidence_ref", "evidence_sha256",
		"issuer_nid", "signature",
	}
	for _, k := range snakeCaseKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("expected snake_case key %q not found in marshaled JSON", k)
		}
	}
}

func TestReputationLogEntry_RoundTrip(t *testing.T) {
	obs := json.RawMessage(`{"count":5}`)
	entry := nip.ReputationLogEntry{
		V:              1,
		LogID:          "urn:nps:org:log.example",
		Seq:            7,
		Timestamp:      "2026-05-01T12:00:00Z",
		SubjectNid:     "urn:nps:org:subject.roundtrip",
		Incident:       nip.IncidentPaymentDefault,
		Severity:       nip.SeverityMinor,
		Window:         &nip.ObservationWindow{Start: "2026-05-01T00:00:00Z", End: "2026-05-01T12:00:00Z"},
		Observation:    obs,
		EvidenceRef:    "https://example.com/ev",
		EvidenceSha256: "deadbeef",
		IssuerNid:      "urn:nps:org:issuer.roundtrip",
		Signature:      "ed25519:roundtripsig",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got nip.ReputationLogEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.V != entry.V {
		t.Errorf("V: got %d, want %d", got.V, entry.V)
	}
	if got.LogID != entry.LogID {
		t.Errorf("LogID: got %q, want %q", got.LogID, entry.LogID)
	}
	if got.Seq != entry.Seq {
		t.Errorf("Seq: got %d, want %d", got.Seq, entry.Seq)
	}
	if got.Timestamp != entry.Timestamp {
		t.Errorf("Timestamp: got %q, want %q", got.Timestamp, entry.Timestamp)
	}
	if got.SubjectNid != entry.SubjectNid {
		t.Errorf("SubjectNid: got %q, want %q", got.SubjectNid, entry.SubjectNid)
	}
	if got.Incident != entry.Incident {
		t.Errorf("Incident: got %q, want %q", got.Incident, entry.Incident)
	}
	if got.Severity != entry.Severity {
		t.Errorf("Severity: got %v, want %v", got.Severity, entry.Severity)
	}
	if got.Window == nil || got.Window.Start != entry.Window.Start || got.Window.End != entry.Window.End {
		t.Errorf("Window: got %+v, want %+v", got.Window, entry.Window)
	}
	if string(got.Observation) != string(entry.Observation) {
		t.Errorf("Observation: got %s, want %s", got.Observation, entry.Observation)
	}
	if got.EvidenceRef != entry.EvidenceRef {
		t.Errorf("EvidenceRef: got %q, want %q", got.EvidenceRef, entry.EvidenceRef)
	}
	if got.EvidenceSha256 != entry.EvidenceSha256 {
		t.Errorf("EvidenceSha256: got %q, want %q", got.EvidenceSha256, entry.EvidenceSha256)
	}
	if got.IssuerNid != entry.IssuerNid {
		t.Errorf("IssuerNid: got %q, want %q", got.IssuerNid, entry.IssuerNid)
	}
	if got.Signature != entry.Signature {
		t.Errorf("Signature: got %q, want %q", got.Signature, entry.Signature)
	}
}

func TestReputationLogEntry_OmitemptyFields(t *testing.T) {
	// Zero-value / omitempty fields must not appear in the JSON output.
	entry := nip.ReputationLogEntry{
		V:          1,
		LogID:      "urn:nps:org:log.min",
		Seq:        1,
		Timestamp:  "2026-01-01T00:00:00Z",
		SubjectNid: "urn:nps:org:subject.min",
		Incident:   nip.IncidentOther,
		Severity:   nip.SeverityInfo,
		IssuerNid:  "urn:nps:org:issuer.min",
		Signature:  "ed25519:minsig",
		// Window, Observation, EvidenceRef, EvidenceSha256 intentionally zero
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, absent := range []string{"window", "observation", "evidence_ref", "evidence_sha256"} {
		if _, ok := m[absent]; ok {
			t.Errorf("expected key %q to be omitted when empty, but it was present", absent)
		}
	}
}

// ─── Part 4 — SignEntry / VerifyEntry ────────────────────────────────────────

func TestSignEntry_VerifyEntry_Valid(t *testing.T) {
	priv, pub := makeKey(t)
	entry := makeUnsigned("urn:nps:org:subject.sign")

	signed, err := nip.SignEntry(priv, entry)
	if err != nil {
		t.Fatalf("SignEntry: %v", err)
	}
	if signed.Signature == "" {
		t.Fatal("Signature should not be empty after signing")
	}
	if !nip.VerifyEntry(pub, signed) {
		t.Error("VerifyEntry should return true for a valid signature")
	}
}

func TestVerifyEntry_TamperedSubjectNid(t *testing.T) {
	priv, pub := makeKey(t)
	signed, err := nip.SignEntry(priv, makeUnsigned("urn:nps:org:subject.original"))
	if err != nil {
		t.Fatalf("SignEntry: %v", err)
	}
	signed.SubjectNid = "urn:nps:org:subject.tampered"
	if nip.VerifyEntry(pub, signed) {
		t.Error("VerifyEntry should return false after tampering subject_nid")
	}
}

func TestVerifyEntry_WrongPublicKey(t *testing.T) {
	priv, _ := makeKey(t)
	_, wrongPub := makeKey(t)
	signed, err := nip.SignEntry(priv, makeUnsigned("urn:nps:org:subject.wrongkey"))
	if err != nil {
		t.Fatalf("SignEntry: %v", err)
	}
	if nip.VerifyEntry(wrongPub, signed) {
		t.Error("VerifyEntry should return false when using a different public key")
	}
}

func TestSignEntry_Deterministic(t *testing.T) {
	priv, _ := makeKey(t)
	entry := makeUnsigned("urn:nps:org:subject.det")

	signed1, err := nip.SignEntry(priv, entry)
	if err != nil {
		t.Fatalf("SignEntry (1): %v", err)
	}
	signed2, err := nip.SignEntry(priv, entry)
	if err != nil {
		t.Fatalf("SignEntry (2): %v", err)
	}
	if signed1.Signature != signed2.Signature {
		t.Errorf("SignEntry should be deterministic: got different signatures\n  1: %s\n  2: %s",
			signed1.Signature, signed2.Signature)
	}
}

// ─── Part 5 — VerifyInclusion (Merkle) ───────────────────────────────────────

func TestVerifyInclusion_SingleLeaf(t *testing.T) {
	entry := makeUnsigned("urn:nps:org:subject.single")
	lh := leafHash(t, entry)

	proof := nip.InclusionProof{
		Seq:       1,
		LeafIndex: 0,
		TreeSize:  1,
		LeafHash:  b64url(lh),
		AuditPath: nil,
	}
	sth := nip.SignedTreeHead{
		LogID:          "urn:nps:org:log.test",
		TreeSize:       1,
		Timestamp:      "2026-01-01T00:00:00Z",
		Sha256RootHash: b64url(lh),
		Signature:      "",
	}
	if !nip.VerifyInclusion(proof, sth, entry) {
		t.Error("VerifyInclusion: expected true for single-leaf tree")
	}
}

func TestVerifyInclusion_TwoLeafTree(t *testing.T) {
	e0 := makeUnsigned("urn:nps:org:subject.two0")
	e1 := makeUnsigned("urn:nps:org:subject.two1")
	e1.Seq = 2

	h0 := leafHash(t, e0)
	h1 := leafHash(t, e1)
	root := nodeHashTest(h0, h1)

	sth := nip.SignedTreeHead{
		Sha256RootHash: b64url(root),
	}

	// verify leaf 0 (index 0); sibling is h1
	proof0 := nip.InclusionProof{
		LeafIndex: 0,
		TreeSize:  2,
		LeafHash:  b64url(h0),
		AuditPath: []string{b64url(h1)},
	}
	if !nip.VerifyInclusion(proof0, sth, e0) {
		t.Error("VerifyInclusion: leaf 0 of 2-leaf tree should verify")
	}

	// verify leaf 1 (index 1); sibling is h0
	proof1 := nip.InclusionProof{
		LeafIndex: 1,
		TreeSize:  2,
		LeafHash:  b64url(h1),
		AuditPath: []string{b64url(h0)},
	}
	if !nip.VerifyInclusion(proof1, sth, e1) {
		t.Error("VerifyInclusion: leaf 1 of 2-leaf tree should verify")
	}
}

func TestVerifyInclusion_FourLeafTree(t *testing.T) {
	// Build 4 entries and their leaf hashes.
	entries := make([]nip.ReputationLogEntry, 4)
	hashes := make([][]byte, 4)
	for i := 0; i < 4; i++ {
		e := makeUnsigned("urn:nps:org:subject.four" + string(rune('0'+i)))
		e.Seq = uint64(i + 1)
		entries[i] = e
		hashes[i] = leafHash(t, e)
	}

	// Level-1 internal nodes:
	n01 := nodeHashTest(hashes[0], hashes[1])
	n23 := nodeHashTest(hashes[2], hashes[3])
	root := nodeHashTest(n01, n23)

	sth := nip.SignedTreeHead{Sha256RootHash: b64url(root)}

	// Each leaf's audit_path:
	//   leaf 0 (index 0): siblings = [h1, n23]
	//   leaf 1 (index 1): siblings = [h0, n23]
	//   leaf 2 (index 2): siblings = [h3, n01]
	//   leaf 3 (index 3): siblings = [h2, n01]
	auditPaths := [4][]string{
		{b64url(hashes[1]), b64url(n23)},
		{b64url(hashes[0]), b64url(n23)},
		{b64url(hashes[3]), b64url(n01)},
		{b64url(hashes[2]), b64url(n01)},
	}

	for i := 0; i < 4; i++ {
		proof := nip.InclusionProof{
			LeafIndex: uint64(i),
			TreeSize:  4,
			LeafHash:  b64url(hashes[i]),
			AuditPath: auditPaths[i],
		}
		if !nip.VerifyInclusion(proof, sth, entries[i]) {
			t.Errorf("VerifyInclusion: leaf %d of 4-leaf tree should verify", i)
		}
	}
}

func TestVerifyInclusion_ReturnsFalseOnTamperedEntry(t *testing.T) {
	entry := makeUnsigned("urn:nps:org:subject.tamper")
	lh := leafHash(t, entry)

	proof := nip.InclusionProof{
		LeafIndex: 0,
		TreeSize:  1,
		LeafHash:  b64url(lh),
		AuditPath: nil,
	}
	sth := nip.SignedTreeHead{Sha256RootHash: b64url(lh)}

	entry.SubjectNid = "urn:nps:org:subject.tampered"
	if nip.VerifyInclusion(proof, sth, entry) {
		t.Error("VerifyInclusion: should return false after tampering subject_nid")
	}
}

func TestVerifyInclusion_ReturnsFalseOnWrongRoot(t *testing.T) {
	entry := makeUnsigned("urn:nps:org:subject.wrongroot")
	lh := leafHash(t, entry)

	proof := nip.InclusionProof{
		LeafIndex: 0,
		TreeSize:  1,
		LeafHash:  b64url(lh),
		AuditPath: nil,
	}
	zeroRoot := make([]byte, 32) // all zeros
	sth := nip.SignedTreeHead{Sha256RootHash: b64url(zeroRoot)}

	if nip.VerifyInclusion(proof, sth, entry) {
		t.Error("VerifyInclusion: should return false when root is all-zeros")
	}
}

func TestVerifyInclusion_ReturnsFalseOnWrongLeafHash(t *testing.T) {
	entry := makeUnsigned("urn:nps:org:subject.wrongleaf")
	lh := leafHash(t, entry)

	// Use a different hash as the LeafHash in the proof
	wrongLeaf := make([]byte, 32)
	wrongLeaf[0] = 0xff

	proof := nip.InclusionProof{
		LeafIndex: 0,
		TreeSize:  1,
		LeafHash:  b64url(wrongLeaf), // wrong
		AuditPath: nil,
	}
	sth := nip.SignedTreeHead{Sha256RootHash: b64url(lh)}

	if nip.VerifyInclusion(proof, sth, entry) {
		t.Error("VerifyInclusion: should return false when leaf_hash in proof doesn't match entry")
	}
}

func TestVerifyInclusion_ReturnsFalseOnCorruptedAuditPath(t *testing.T) {
	e0 := makeUnsigned("urn:nps:org:subject.corrupt0")
	e1 := makeUnsigned("urn:nps:org:subject.corrupt1")
	e1.Seq = 2

	h0 := leafHash(t, e0)
	h1 := leafHash(t, e1)
	root := nodeHashTest(h0, h1)

	sth := nip.SignedTreeHead{Sha256RootHash: b64url(root)}

	// Provide a zero-byte sibling instead of the real sibling
	zeroSibling := make([]byte, 32)
	proof := nip.InclusionProof{
		LeafIndex: 0,
		TreeSize:  2,
		LeafHash:  b64url(h0),
		AuditPath: []string{b64url(zeroSibling)}, // corrupted: should be h1
	}

	if nip.VerifyInclusion(proof, sth, e0) {
		t.Error("VerifyInclusion: should return false when audit path sibling is zeroed out")
	}
}
