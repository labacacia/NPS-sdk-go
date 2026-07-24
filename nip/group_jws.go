// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
)

// Group-JWS constants (NPS-CR-0003 §3.5 / §5.1.3).
const (
	GroupJwsExpectedAlg     = "EdDSA"
	GroupJwsExpectedPurpose = "session-issue"
)

// FlattenedJws is the flattened JWS object as it appears on the wire.
type FlattenedJws struct {
	Protected string `json:"protected"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

type jwsHeader struct {
	Alg        string `json:"alg"`
	Kid        string `json:"kid"`
	NpsPurpose string `json:"nps-purpose"`
}

// VerifyGroupJws parses + verifies a flattened group-JWS. On success it returns
// the decoded payload JSON and the asserted kid; on failure it returns a NIP
// error code (ErrCaJwsInvalid).
func VerifyGroupJws(jws FlattenedJws, groupPubKey ed25519.PublicKey) (payloadJSON string, kid string, errorCode string, ok bool) {
	if jws.Protected == "" || jws.Payload == "" || jws.Signature == "" {
		return "", "", ErrCaJwsInvalid, false
	}

	headerBytes, err := decodeBase64Url(jws.Protected)
	if err != nil {
		return "", "", ErrCaJwsInvalid, false
	}
	payloadBytes, err := decodeBase64Url(jws.Payload)
	if err != nil {
		return "", "", ErrCaJwsInvalid, false
	}
	sigBytes, err := decodeBase64Url(jws.Signature)
	if err != nil {
		return "", "", ErrCaJwsInvalid, false
	}

	var header jwsHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return "", "", ErrCaJwsInvalid, false
	}
	if header.Alg != GroupJwsExpectedAlg || header.NpsPurpose != GroupJwsExpectedPurpose || header.Kid == "" {
		return "", "", ErrCaJwsInvalid, false
	}

	// RFC 7515 §3 signing input: ASCII(protected) "." ASCII(payload).
	signingInput := []byte(jws.Protected + "." + jws.Payload)
	if !ed25519.Verify(groupPubKey, signingInput, sigBytes) {
		return "", "", ErrCaJwsInvalid, false
	}

	return string(payloadBytes), header.Kid, "", true
}

// ── base64url + pubkey helpers ──────────────────────────────────────────────────

func base64Url(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

func decodeBase64Url(s string) ([]byte, error) {
	// Accept both padded and unpadded input.
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

// DecodePublicKey parses an "ed25519:<base64url>" public key string. Returns
// nil if the string is not a valid Ed25519 public key.
func DecodePublicKey(encoded string) ed25519.PublicKey {
	const prefix = "ed25519:"
	if len(encoded) <= len(prefix) || encoded[:len(prefix)] != prefix {
		return nil
	}
	raw, err := decodeBase64Url(encoded[len(prefix):])
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(raw)
}
