// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package acme

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// JWS signing helpers for ACME with Ed25519 (alg="EdDSA" per RFC 8037).
//
// Wire shape (RFC 8555 §6.2 + RFC 7515 flattened JWS JSON serialization):
//
//	{
//	  "protected": base64url(JSON({alg, nonce, url, [jwk|kid]})),
//	  "payload":   base64url(JSON(payload)),
//	  "signature": base64url(Ed25519(protected || "." || payload))
//	}

const (
	AlgEdDSA   = "EdDSA"   // RFC 8037 §3.1
	KtyOkp     = "OKP"     // RFC 8037 §2
	CrvEd25519 = "Ed25519" // RFC 8037 §2
)

// JWK — Ed25519 JSON Web Key (RFC 8037 §2).
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
}

// ProtectedHeader — JWS protected header.
type ProtectedHeader struct {
	Alg   string `json:"alg"`
	Nonce string `json:"nonce"`
	URL   string `json:"url"`
	JWK   *JWK   `json:"jwk,omitempty"`
	Kid   string `json:"kid,omitempty"`
}

// Envelope — flattened JWS JSON serialization.
type Envelope struct {
	Protected string `json:"protected"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// JwkFromPublicKey wraps a 32-byte Ed25519 public key in a JWK.
func JwkFromPublicKey(rawPubKey ed25519.PublicKey) (*JWK, error) {
	if len(rawPubKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("Ed25519 public key must be 32 bytes, got %d", len(rawPubKey))
	}
	return &JWK{Kty: KtyOkp, Crv: CrvEd25519, X: B64uEncode(rawPubKey)}, nil
}

// PublicKeyFromJWK extracts an Ed25519 public key from a JWK.
func PublicKeyFromJWK(jwk *JWK) (ed25519.PublicKey, error) {
	if jwk.Kty != KtyOkp || jwk.Crv != CrvEd25519 {
		return nil, fmt.Errorf("JWK is not OKP/Ed25519: kty=%s crv=%s", jwk.Kty, jwk.Crv)
	}
	raw, err := B64uDecode(jwk.X)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("JWK x decodes to %d bytes, want 32", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// Thumbprint — RFC 7638 §3 thumbprint of an Ed25519 JWK.
func Thumbprint(jwk *JWK) string {
	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s"}`, jwk.Crv, jwk.Kty, jwk.X)
	h := sha256.Sum256([]byte(canonical))
	return B64uEncode(h[:])
}

// Sign creates a flattened JWS envelope. payload may be nil for POST-as-GET.
func Sign(header ProtectedHeader, payload any, privKey ed25519.PrivateKey) (*Envelope, error) {
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("marshal header: %w", err)
	}
	headerB64u := B64uEncode(headerBytes)

	payloadB64u := ""
	if payload != nil {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		payloadB64u = B64uEncode(payloadBytes)
	}

	signingInput := []byte(headerB64u + "." + payloadB64u)
	sig := ed25519.Sign(privKey, signingInput)
	return &Envelope{Protected: headerB64u, Payload: payloadB64u, Signature: B64uEncode(sig)}, nil
}

// Verify validates an envelope against pubKey. Returns the parsed protected
// header on success, or nil + error on failure.
func Verify(env *Envelope, pubKey ed25519.PublicKey) (*ProtectedHeader, error) {
	signingInput := []byte(env.Protected + "." + env.Payload)
	sig, err := B64uDecode(env.Signature)
	if err != nil {
		return nil, fmt.Errorf("signature b64u decode: %w", err)
	}
	if !ed25519.Verify(pubKey, signingInput, sig) {
		return nil, fmt.Errorf("JWS signature verify failed")
	}
	headerBytes, err := B64uDecode(env.Protected)
	if err != nil {
		return nil, fmt.Errorf("protected header b64u decode: %w", err)
	}
	var header ProtectedHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("protected header JSON: %w", err)
	}
	return &header, nil
}

// DecodePayload unmarshals the payload field into out. Returns nil on empty.
func DecodePayload(env *Envelope, out any) error {
	if env.Payload == "" {
		return nil
	}
	b, err := B64uDecode(env.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// B64uEncode — base64url-without-padding encoding.
func B64uEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// B64uDecode — base64url decoding tolerating optional padding.
func B64uDecode(s string) ([]byte, error) {
	if pad := len(s) % 4; pad != 0 {
		s += "===="[:4-pad]
	}
	return base64.URLEncoding.DecodeString(s)
}
