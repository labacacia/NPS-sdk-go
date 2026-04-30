// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import (
	"crypto/ed25519"
	cryptox509 "crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// VerifierOptions — configuration for NipIdentVerifier per NPS-RFC-0002 §8.1.
type VerifierOptions struct {
	// TrustedCaPublicKeys maps issuer NID → CA public key string ("ed25519:<hex>").
	TrustedCaPublicKeys map[string]string
	// TrustedX509Roots — empty/nil makes Step 3b skip even for v2 frames.
	TrustedX509Roots []*cryptox509.Certificate
	// MinAssuranceLevel — when non-nil, frames below this rank are rejected at Step 2.
	MinAssuranceLevel *AssuranceLevel
}

// IdentVerifyResult — outcome of NipIdentVerifier.Verify.
type IdentVerifyResult struct {
	Valid      bool
	StepFailed int // 0 none, 1 sig, 2 assurance, 3 X.509
	ErrorCode  string
	Message    string
}

// X509ChainVerifier — pluggable hook so this package doesn't import nip/x509
// (which itself imports nip — would cycle). Tests / callers wire up a verifier
// that delegates to nip/x509.Verify.
type X509ChainVerifier func(
	chainBase64UrlDer []string,
	assertedNid string,
	assertedAssuranceLevel *AssuranceLevel,
	trustedRoots []*cryptox509.Certificate,
) (valid bool, errorCode, message string)

// NipIdentVerifier — Phase 1 dual-trust IdentFrame verifier.
type NipIdentVerifier struct {
	Options       VerifierOptions
	X509Verifier  X509ChainVerifier // optional — required only for Step 3b
}

func NewNipIdentVerifier(opts VerifierOptions, x509Verifier X509ChainVerifier) *NipIdentVerifier {
	return &NipIdentVerifier{Options: opts, X509Verifier: x509Verifier}
}

// Verify runs the three-step Phase 1 dual-trust check.
func (v *NipIdentVerifier) Verify(frame *IdentFrame, issuerNid string) IdentVerifyResult {
	// Step 1: v1 Ed25519 signature check ─────────────────────────────────────
	caPubKeyStr, ok := v.Options.TrustedCaPublicKeys[issuerNid]
	if !ok {
		return fail(1, ErrCertUntrustedIssuer,
			fmt.Sprintf("no trusted CA public key for issuer: %s", issuerNid))
	}
	if frame.Signature == nil || !strings.HasPrefix(*frame.Signature, "ed25519:") {
		return fail(1, ErrCertSignatureInvalid, "missing or malformed signature")
	}
	caPubBytes, err := parsePubKeyString(caPubKeyStr)
	if err != nil {
		return fail(1, ErrCertSignatureInvalid, err.Error())
	}
	sigB64 := (*frame.Signature)[len("ed25519:"):]
	sigBytes, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fail(1, ErrCertSignatureInvalid, fmt.Sprintf("base64 decode: %v", err))
	}
	canonical := canonicalSorted(frame.UnsignedDict())
	if !ed25519.Verify(caPubBytes, []byte(canonical), sigBytes) {
		return fail(1, ErrCertSignatureInvalid,
			"v1 Ed25519 signature did not verify against issuer CA key")
	}

	// Step 2: minimum assurance level ────────────────────────────────────────
	if v.Options.MinAssuranceLevel != nil {
		got := AssuranceAnonymous
		if frame.AssuranceLevel != nil {
			got = *frame.AssuranceLevel
		}
		if !got.MeetsOrExceeds(*v.Options.MinAssuranceLevel) {
			return fail(2, ErrAssuranceMismatch,
				fmt.Sprintf("assurance_level (%s) below required minimum (%s)",
					got.Wire, v.Options.MinAssuranceLevel.Wire))
		}
	}

	// Step 3b: X.509 chain check (only if both opt-ins present) ──────────────
	hasV2Trust := len(v.Options.TrustedX509Roots) > 0 && v.X509Verifier != nil
	isV2Frame := frame.CertFormat != nil && *frame.CertFormat == CertFormatV2X509
	if hasV2Trust && isV2Frame {
		ok, code, msg := v.X509Verifier(
			frame.CertChain, frame.NID, frame.AssuranceLevel, v.Options.TrustedX509Roots)
		if !ok {
			if code == "" {
				code = ErrCertFormatInvalid
			}
			return fail(3, code, msg)
		}
	}

	return IdentVerifyResult{Valid: true}
}

func fail(step int, code, msg string) IdentVerifyResult {
	return IdentVerifyResult{Valid: false, StepFailed: step, ErrorCode: code, Message: msg}
}

func parsePubKeyString(s string) (ed25519.PublicKey, error) {
	if !strings.HasPrefix(s, "ed25519:") {
		return nil, fmt.Errorf("unsupported public key format: %s", s)
	}
	raw, err := hex.DecodeString(s[len("ed25519:"):])
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key wrong size: %d", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// canonicalSorted matches NipIdentity.canonicalJSON — sorted top-level keys.
func canonicalSorted(d map[string]any) string {
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
