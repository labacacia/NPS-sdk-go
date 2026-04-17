// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package ca

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iters = 600_000
	saltLen     = 16
	nonceLen    = 12
	keyLen      = 32
)

// ── Key Management ────────────────────────────────────────────────────────────

// GenerateKey creates a new random Ed25519 signing key.
func GenerateKey() (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	return priv, err
}

// PubKeyString returns the "ed25519:<hex>" encoded public key.
func PubKeyString(pub ed25519.PublicKey) string {
	return "ed25519:" + hex.EncodeToString(pub)
}

// SaveKey encrypts sk with passphrase (PBKDF2+AES-256-GCM) and writes to path.
func SaveKey(sk ed25519.PrivateKey, path, passphrase string) error {
	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	dk := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iters, keyLen, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Ed25519 private key seed = first 32 bytes
	seed := []byte(sk)[:32]
	ct := gcm.Seal(nil, nonce, seed, nil)

	pub := sk.Public().(ed25519.PublicKey)
	envelope := map[string]any{
		"version":    1,
		"algorithm":  "ed25519",
		"pub_key":    PubKeyString(pub),
		"salt":       hex.EncodeToString(salt),
		"nonce":      hex.EncodeToString(nonce),
		"ciphertext": hex.EncodeToString(ct),
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return nil
}

// LoadKey decrypts the key file at path using passphrase.
func LoadKey(path, passphrase string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env map[string]any
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	salt, err := hex.DecodeString(env["salt"].(string))
	if err != nil {
		return nil, err
	}
	nonce, err := hex.DecodeString(env["nonce"].(string))
	if err != nil {
		return nil, err
	}
	ct, err := hex.DecodeString(env["ciphertext"].(string))
	if err != nil {
		return nil, err
	}

	dk := pbkdf2.Key([]byte(passphrase), salt, pbkdf2Iters, keyLen, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	seed, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("key decryption failed — wrong passphrase?")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// ── Signing ───────────────────────────────────────────────────────────────────

// CanonicalJSON serialises obj with keys sorted alphabetically.
func CanonicalJSON(obj map[string]any) []byte {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf := []byte{'{'}
	for i, k := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		kBytes, _ := json.Marshal(k)
		vBytes, _ := json.Marshal(obj[k])
		buf = append(buf, kBytes...)
		buf = append(buf, ':')
		buf = append(buf, vBytes...)
	}
	buf = append(buf, '}')
	return buf
}

// SignDict signs the canonical JSON of obj with sk and returns "ed25519:<base64>".
func SignDict(sk ed25519.PrivateKey, obj map[string]any) string {
	msg := CanonicalJSON(obj)
	sig := ed25519.Sign(sk, msg)
	return "ed25519:" + base64.StdEncoding.EncodeToString(sig)
}

// ── Certificate Issuance ──────────────────────────────────────────────────────

// GenerateNID creates a random NID for entity_type under domain.
func GenerateNID(domain, entityType string) string {
	uid := make([]byte, 8)
	_, _ = io.ReadFull(rand.Reader, uid)
	return fmt.Sprintf("urn:nps:%s:%s:%s", entityType, domain, hex.EncodeToString(uid))
}

// IssueCert signs and returns an IdentFrame dict.
func IssueCert(
	sk ed25519.PrivateKey,
	caNID, subjectNID, subjectPubKey string,
	capabilities []string,
	scope map[string]any,
	validityDays int,
	serial string,
	metadata map[string]any,
) map[string]any {
	now := time.Now().UTC()
	exp := now.Add(time.Duration(validityDays) * 24 * time.Hour)
	issuedAt := now.Format(time.RFC3339)
	expiresAt := exp.Format(time.RFC3339)

	if capabilities == nil {
		capabilities = []string{}
	}
	if scope == nil {
		scope = map[string]any{}
	}

	caps := make([]any, len(capabilities))
	for i, c := range capabilities {
		caps[i] = c
	}

	unsigned := map[string]any{
		"capabilities": caps,
		"expires_at":   expiresAt,
		"issued_at":    issuedAt,
		"issued_by":    caNID,
		"nid":          subjectNID,
		"pub_key":      subjectPubKey,
		"scope":        scope,
		"serial":       serial,
	}
	sig := SignDict(sk, unsigned)
	cert := map[string]any{
		"capabilities": caps,
		"expires_at":   expiresAt,
		"issued_at":    issuedAt,
		"issued_by":    caNID,
		"nid":          subjectNID,
		"pub_key":      subjectPubKey,
		"scope":        scope,
		"serial":       serial,
		"signature":    sig,
	}
	if metadata != nil {
		cert["metadata"] = metadata
	}
	return cert
}
