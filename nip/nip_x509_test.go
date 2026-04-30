// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip_test

import (
	"crypto/ed25519"
	"crypto/rand"
	cryptox509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/labacacia/NPS-sdk-go/core"
	npsnip "github.com/labacacia/NPS-sdk-go/nip"
	npsx509 "github.com/labacacia/NPS-sdk-go/nip/x509"
)

// Go parallel of .NET / Java / Python NipX509Tests per NPS-RFC-0002 §4.
// Covers the 5 verification scenarios documented in the .NET reference.

func TestRegisterX509RoundTrip_VerifierAccepts(t *testing.T) {
	caNid    := "urn:nps:ca:test"
	agentNid := "urn:nps:agent:happy:1"

	caPub, caPriv, _    := ed25519.GenerateKey(rand.Reader)
	agentPub, _, _      := ed25519.GenerateKey(rand.Reader)

	root := mustIssueRoot(t, caPriv, caNid, big.NewInt(1))
	leaf := mustIssueLeaf(t, agentNid, agentPub, caPriv, caNid,
		npsx509.LeafRoleAgent, npsnip.AssuranceAttested, big.NewInt(2))

	frame := buildV2Frame(t, agentNid, agentPub, caPriv,
		&npsnip.AssuranceAttested, leaf, root)

	verifier := npsnip.NewNipIdentVerifier(
		npsnip.VerifierOptions{
			TrustedCaPublicKeys: map[string]string{caNid: pubKeyHex(caPub)},
			TrustedX509Roots:    []*cryptox509.Certificate{root},
		},
		x509Adapter,
	)
	r := verifier.Verify(frame, caNid)
	if !r.Valid {
		t.Fatalf("expected valid; got step=%d code=%s msg=%s", r.StepFailed, r.ErrorCode, r.Message)
	}
}

func TestRegisterX509_LeafEkuStripped_RejectsEkuMissing(t *testing.T) {
	caNid    := "urn:nps:ca:test"
	agentNid := "urn:nps:agent:eku-stripped:1"

	caPub, caPriv, _   := ed25519.GenerateKey(rand.Reader)
	agentPub, _, _     := ed25519.GenerateKey(rand.Reader)

	root := mustIssueRoot(t, caPriv, caNid, big.NewInt(1))
	tampered := buildLeafWithoutEku(t, agentNid, agentPub, caPriv, caNid, big.NewInt(99))

	frame := buildV2Frame(t, agentNid, agentPub, caPriv, nil, tampered, root)

	verifier := npsnip.NewNipIdentVerifier(
		npsnip.VerifierOptions{
			TrustedCaPublicKeys: map[string]string{caNid: pubKeyHex(caPub)},
			TrustedX509Roots:    []*cryptox509.Certificate{root},
		},
		x509Adapter,
	)
	r := verifier.Verify(frame, caNid)
	if r.Valid {
		t.Fatalf("expected invalid")
	}
	if r.ErrorCode != npsnip.ErrCertEkuMissing {
		t.Fatalf("want %s, got %s (msg=%s)", npsnip.ErrCertEkuMissing, r.ErrorCode, r.Message)
	}
	if r.StepFailed != 3 {
		t.Fatalf("want step 3, got %d", r.StepFailed)
	}
}

func TestRegisterX509_LeafForDifferentNid_RejectsSubjectMismatch(t *testing.T) {
	caNid     := "urn:nps:ca:test"
	victimNid := "urn:nps:agent:victim:1"
	forgedNid := "urn:nps:agent:attacker:9"

	caPub, caPriv, _   := ed25519.GenerateKey(rand.Reader)
	agentPub, _, _     := ed25519.GenerateKey(rand.Reader)

	root := mustIssueRoot(t, caPriv, caNid, big.NewInt(1))
	// Issue a leaf whose CN/SAN are the *forged* NID, but splice it into a frame
	// claiming the *victim* NID. The IdentFrame v1 signature still asserts victim.
	forgedLeaf := mustIssueLeaf(t, forgedNid, agentPub, caPriv, caNid,
		npsx509.LeafRoleAgent, npsnip.AssuranceAnonymous, big.NewInt(77))

	frame := buildV2Frame(t, victimNid, agentPub, caPriv, nil, forgedLeaf, root)

	verifier := npsnip.NewNipIdentVerifier(
		npsnip.VerifierOptions{
			TrustedCaPublicKeys: map[string]string{caNid: pubKeyHex(caPub)},
			TrustedX509Roots:    []*cryptox509.Certificate{root},
		},
		x509Adapter,
	)
	r := verifier.Verify(frame, caNid)
	if r.Valid {
		t.Fatalf("expected invalid")
	}
	if r.ErrorCode != npsnip.ErrCertSubjectNidMismatch {
		t.Fatalf("want %s, got %s (msg=%s)", npsnip.ErrCertSubjectNidMismatch, r.ErrorCode, r.Message)
	}
	if r.StepFailed != 3 {
		t.Fatalf("want step 3, got %d", r.StepFailed)
	}
}

func TestV1OnlyVerifier_AcceptsV2FrameByIgnoringChain(t *testing.T) {
	caNid    := "urn:nps:ca:test"
	agentNid := "urn:nps:agent:v1-compat:1"

	caPub, caPriv, _   := ed25519.GenerateKey(rand.Reader)
	agentPub, _, _     := ed25519.GenerateKey(rand.Reader)

	root := mustIssueRoot(t, caPriv, caNid, big.NewInt(1))
	leaf := mustIssueLeaf(t, agentNid, agentPub, caPriv, caNid,
		npsx509.LeafRoleAgent, npsnip.AssuranceAnonymous, big.NewInt(2))

	frame := buildV2Frame(t, agentNid, agentPub, caPriv, nil, leaf, root)

	// Verifier WITHOUT TrustedX509Roots — Step 3b skipped entirely.
	verifier := npsnip.NewNipIdentVerifier(
		npsnip.VerifierOptions{
			TrustedCaPublicKeys: map[string]string{caNid: pubKeyHex(caPub)},
		},
		nil, // no X.509 verifier wired
	)
	r := verifier.Verify(frame, caNid)
	if !r.Valid {
		t.Fatalf("v1-only verifier MUST accept v2 frames; got code=%s msg=%s", r.ErrorCode, r.Message)
	}
}

func TestV2Verifier_RejectsV2FrameWhenTrustedRootsMissing(t *testing.T) {
	caNid    := "urn:nps:ca:test"
	agentNid := "urn:nps:agent:wrong-trust:1"

	caPub, caPriv, _   := ed25519.GenerateKey(rand.Reader)
	agentPub, _, _     := ed25519.GenerateKey(rand.Reader)

	root := mustIssueRoot(t, caPriv, caNid, big.NewInt(1))
	leaf := mustIssueLeaf(t, agentNid, agentPub, caPriv, caNid,
		npsx509.LeafRoleAgent, npsnip.AssuranceAnonymous, big.NewInt(2))

	frame := buildV2Frame(t, agentNid, agentPub, caPriv, nil, leaf, root)

	// Different unrelated CA root — chain won't anchor.
	_, otherCaPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherRoot := mustIssueRoot(t, otherCaPriv, "urn:nps:ca:other", big.NewInt(1))

	verifier := npsnip.NewNipIdentVerifier(
		npsnip.VerifierOptions{
			TrustedCaPublicKeys: map[string]string{caNid: pubKeyHex(caPub)},
			TrustedX509Roots:    []*cryptox509.Certificate{otherRoot},
		},
		x509Adapter,
	)
	r := verifier.Verify(frame, caNid)
	if r.Valid {
		t.Fatalf("expected invalid")
	}
	if r.ErrorCode != npsnip.ErrCertFormatInvalid {
		t.Fatalf("want %s, got %s (msg=%s)", npsnip.ErrCertFormatInvalid, r.ErrorCode, r.Message)
	}
	if r.StepFailed != 3 {
		t.Fatalf("want step 3, got %d", r.StepFailed)
	}
}

// ── Test helpers ────────────────────────────────────────────────────────────

// x509Adapter delegates Step 3b to the nip/x509 verifier — wired into the
// NipIdentVerifier without creating an import cycle.
func x509Adapter(
	chain []string, assertedNid string, asserted *npsnip.AssuranceLevel,
	roots []*cryptox509.Certificate,
) (bool, string, string) {
	r := npsx509.Verify(npsx509.VerifyOptions{
		CertChainBase64UrlDer:  chain,
		AssertedNID:            assertedNid,
		AssertedAssuranceLevel: asserted,
		TrustedRootCerts:       roots,
	})
	return r.Valid, r.ErrorCode, r.Message
}

func mustIssueRoot(t *testing.T, caPriv ed25519.PrivateKey, caNid string, serial *big.Int) *cryptox509.Certificate {
	t.Helper()
	now := time.Now()
	root, err := npsx509.IssueRoot(npsx509.IssueRootOptions{
		CANID: caNid, CAPrivateKey: caPriv,
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour),
		SerialNumber: serial,
	})
	if err != nil {
		t.Fatalf("issue root: %v", err)
	}
	return root
}

func mustIssueLeaf(t *testing.T, nid string, subjectPub ed25519.PublicKey,
	caPriv ed25519.PrivateKey, caNid string, role npsx509.LeafRole,
	level npsnip.AssuranceLevel, serial *big.Int) *cryptox509.Certificate {
	t.Helper()
	now := time.Now()
	leaf, err := npsx509.IssueLeaf(npsx509.IssueLeafOptions{
		SubjectNID: nid, SubjectPublicKey: subjectPub,
		CAPrivateKey: caPriv, IssuerNID: caNid, Role: role,
		AssuranceLevel: level,
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(30 * 24 * time.Hour),
		SerialNumber: serial,
	})
	if err != nil {
		t.Fatalf("issue leaf: %v", err)
	}
	return leaf
}

// buildV2Frame builds an IdentFrame including the v1 Ed25519 CA signature
// covering UnsignedDict(), and attaches the X.509 chain (leaf + root).
func buildV2Frame(t *testing.T, nid string, subjectPub ed25519.PublicKey,
	caPriv ed25519.PrivateKey, level *npsnip.AssuranceLevel,
	leaf, root *cryptox509.Certificate) *npsnip.IdentFrame {
	t.Helper()
	pubKeyStr := pubKeyHex(subjectPub)
	frame := &npsnip.IdentFrame{
		NID:            nid,
		PubKey:         pubKeyStr,
		Meta:           map[string]any{"issued_by": "test-ca"},
		AssuranceLevel: level,
	}
	canonical := canonicalSorted(frame.UnsignedDict())
	sig := ed25519.Sign(caPriv, []byte(canonical))
	sigWire := "ed25519:" + base64.StdEncoding.EncodeToString(sig)
	frame.Signature = &sigWire

	v2 := npsnip.CertFormatV2X509
	frame.CertFormat = &v2
	frame.CertChain = []string{
		base64.RawURLEncoding.EncodeToString(leaf.Raw),
		base64.RawURLEncoding.EncodeToString(root.Raw),
	}
	return frame
}

// buildLeafWithoutEku constructs a leaf cert whose ExtendedKeyUsage extension
// has been deliberately omitted — exercises the verifier's EKU presence check.
func buildLeafWithoutEku(t *testing.T, nid string, subjectPub ed25519.PublicKey,
	caPriv ed25519.PrivateKey, caNid string, serial *big.Int) *cryptox509.Certificate {
	t.Helper()
	uri, err := url.Parse(nid)
	if err != nil {
		t.Fatalf("parse nid: %v", err)
	}
	now := time.Now()
	tmpl := &cryptox509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: nid},
		Issuer:                pkix.Name{CommonName: caNid},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(30 * 24 * time.Hour),
		KeyUsage:              cryptox509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  false,
		URIs:                  []*url.URL{uri},
		// ★ Deliberately NO EKU extension.
	}
	parent := &cryptox509.Certificate{Subject: pkix.Name{CommonName: caNid}}
	der, err := cryptox509.CreateCertificate(rand.Reader, tmpl, parent,
		subjectPub, caPriv)
	if err != nil {
		t.Fatalf("create no-eku leaf: %v", err)
	}
	c, err := cryptox509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse no-eku leaf: %v", err)
	}
	return c
}

func pubKeyHex(pub ed25519.PublicKey) string {
	return "ed25519:" + hex.EncodeToString(pub)
}

// canonicalSorted matches NipIdentity.canonicalJSON / verifier.canonicalSorted —
// re-implemented here so tests don't depend on package-private helpers.
func canonicalSorted(d core.FrameDict) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(d))
	for _, k := range keys {
		ordered[k] = d[k]
	}
	b, _ := json.Marshal(ordered)
	return string(b)
}
