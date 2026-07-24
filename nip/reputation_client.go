// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// ─── Incident type ───────────────────────────────────────────────────────────

// IncidentType is the kebab-case wire string for an incident kind.
// Unknown wire values are kept as-is for forward compatibility.
type IncidentType string

const (
	IncidentOther               IncidentType = "other"
	IncidentCertRevoked         IncidentType = "cert-revoked"
	IncidentRateLimitViolation  IncidentType = "rate-limit-violation"
	IncidentTosViolation        IncidentType = "tos-violation"
	IncidentScrapingPattern     IncidentType = "scraping-pattern"
	IncidentPaymentDefault      IncidentType = "payment-default"
	IncidentContractDispute     IncidentType = "contract-dispute"
	IncidentImpersonationClaim  IncidentType = "impersonation-claim"
	IncidentPositiveAttestation IncidentType = "positive-attestation"
)

// ─── Severity ────────────────────────────────────────────────────────────────

// Severity is an ordered integer enum for incident severity.
type Severity int

const (
	SeverityInfo     Severity = 0
	SeverityMinor    Severity = 1
	SeverityModerate Severity = 2
	SeverityMajor    Severity = 3
	SeverityCritical Severity = 4
)

var severityWire = map[Severity]string{
	SeverityInfo:     "info",
	SeverityMinor:    "minor",
	SeverityModerate: "moderate",
	SeverityMajor:    "major",
	SeverityCritical: "critical",
}

var wireToSeverity = map[string]Severity{
	"info":     SeverityInfo,
	"minor":    SeverityMinor,
	"moderate": SeverityModerate,
	"major":    SeverityMajor,
	"critical": SeverityCritical,
}

// MarshalJSON encodes Severity as a wire string.
func (s Severity) MarshalJSON() ([]byte, error) {
	w, ok := severityWire[s]
	if !ok {
		return nil, fmt.Errorf("nip: unknown Severity value %d", int(s))
	}
	return json.Marshal(w)
}

// UnmarshalJSON decodes a wire string into Severity.
func (s *Severity) UnmarshalJSON(data []byte) error {
	var wire string
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	v, ok := wireToSeverity[wire]
	if !ok {
		return fmt.Errorf("nip: unknown severity wire value %q", wire)
	}
	*s = v
	return nil
}

// ─── Core structs ─────────────────────────────────────────────────────────────

// ObservationWindow is an optional time window for an observed incident.
type ObservationWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ReputationLogEntry is a single signed entry in the NPS reputation log.
type ReputationLogEntry struct {
	V              int                `json:"v"`
	LogID          string             `json:"log_id"`
	Seq            uint64             `json:"seq"`
	Timestamp      string             `json:"timestamp"`
	SubjectNid     string             `json:"subject_nid"`
	Incident       IncidentType       `json:"incident"`
	Severity       Severity           `json:"severity"`
	Window         *ObservationWindow `json:"window,omitempty"`
	Observation    json.RawMessage    `json:"observation,omitempty"`
	EvidenceRef    string             `json:"evidence_ref,omitempty"`
	EvidenceSha256 string             `json:"evidence_sha256,omitempty"`
	IssuerNid      string             `json:"issuer_nid"`
	Signature      string             `json:"signature"`
}

// SignedTreeHead is the signed tree head for a Merkle log.
type SignedTreeHead struct {
	LogID          string `json:"log_id"`
	TreeSize       uint64 `json:"tree_size"`
	Timestamp      string `json:"timestamp"`
	Sha256RootHash string `json:"sha256_root_hash"`
	Signature      string `json:"signature"`
}

// InclusionProof is a Merkle inclusion proof for a log entry.
type InclusionProof struct {
	Seq       uint64   `json:"seq"`
	LeafIndex uint64   `json:"leaf_index"`
	TreeSize  uint64   `json:"tree_size"`
	LeafHash  string   `json:"leaf_hash"`
	AuditPath []string `json:"audit_path"`
}

// ReputationLogError is returned by ReputationLogClient on API errors.
type ReputationLogError struct {
	NipErrorCode string
	NpsStatus    string
	Message      string
}

func (e *ReputationLogError) Error() string { return e.Message }

// ─── Signing helpers ──────────────────────────────────────────────────────────

// canonicalJSONMap produces deterministic JSON with sorted keys from a map.
func canonicalJSONMap(m map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valBytes, err := json.Marshal(m[k])
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// entryToMap marshals entry to JSON then back into map[string]any.
func entryToMap(entry ReputationLogEntry) (map[string]any, error) {
	raw, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// SignEntry signs entry with the given Ed25519 private key.
// Returns a new entry with Signature populated.
func SignEntry(privKey ed25519.PrivateKey, entry ReputationLogEntry) (ReputationLogEntry, error) {
	entry.Signature = ""
	m, err := entryToMap(entry)
	if err != nil {
		return entry, fmt.Errorf("nip: SignEntry marshal: %w", err)
	}
	delete(m, "signature")

	canonical, err := canonicalJSONMap(m)
	if err != nil {
		return entry, fmt.Errorf("nip: SignEntry canonical: %w", err)
	}

	sig := ed25519.Sign(privKey, canonical)
	entry.Signature = "ed25519:" + base64.RawURLEncoding.EncodeToString(sig)
	return entry, nil
}

// VerifyEntry verifies the issuer signature on entry.
func VerifyEntry(pubKey ed25519.PublicKey, entry ReputationLogEntry) bool {
	sigStr := entry.Signature
	if !strings.HasPrefix(sigStr, "ed25519:") {
		return false
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(sigStr[8:])
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return false
	}

	entry.Signature = ""
	m, err := entryToMap(entry)
	if err != nil {
		return false
	}
	delete(m, "signature")

	canonical, err := canonicalJSONMap(m)
	if err != nil {
		return false
	}

	return ed25519.Verify(pubKey, canonical, sigBytes)
}

// ─── Merkle verification ──────────────────────────────────────────────────────

// VerifyInclusion verifies that entry is included in the tree described by sth.
func VerifyInclusion(proof InclusionProof, sth SignedTreeHead, entry ReputationLogEntry) bool {
	m, err := entryToMap(entry)
	if err != nil {
		return false
	}
	leafJSON, err := canonicalJSONMap(m)
	if err != nil {
		return false
	}

	leafInput := append([]byte{0x00}, leafJSON...)
	h := sha256.Sum256(leafInput)
	nodeHash := h[:]

	expectedLeaf, err := base64.RawURLEncoding.DecodeString(proof.LeafHash)
	if err != nil {
		return false
	}
	if !bytes.Equal(nodeHash, expectedLeaf) {
		return false
	}

	for i, step := range proof.AuditPath {
		sibling, err := base64.RawURLEncoding.DecodeString(step)
		if err != nil {
			return false
		}
		buf := make([]byte, 65)
		buf[0] = 0x01
		if (proof.LeafIndex>>uint(i))&1 == 0 {
			copy(buf[1:], nodeHash)
			copy(buf[33:], sibling)
		} else {
			copy(buf[1:], sibling)
			copy(buf[33:], nodeHash)
		}
		h = sha256.Sum256(buf)
		nodeHash = h[:]
	}

	rootHash, err := base64.RawURLEncoding.DecodeString(sth.Sha256RootHash)
	if err != nil {
		return false
	}
	return bytes.Equal(nodeHash, rootHash)
}

// ─── HTTP client ──────────────────────────────────────────────────────────────

// ReputationLogClientOptions configures a ReputationLogClient.
type ReputationLogClientOptions struct {
	BaseURL    string
	HTTPClient *http.Client // nil → use http.DefaultClient
}

// ReputationLogClient is an HTTP client for the NPS-RFC-0004 reputation log API.
type ReputationLogClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewReputationLogClient creates a new ReputationLogClient.
func NewReputationLogClient(opts ReputationLogClientOptions) *ReputationLogClient {
	hc := opts.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &ReputationLogClient{
		baseURL:    strings.TrimRight(opts.BaseURL, "/"),
		httpClient: hc,
	}
}

// doRequest executes an HTTP request and returns the response body.
// On non-2xx responses, it attempts to parse an error envelope.
func (c *ReputationLogClient) doRequest(ctx context.Context, method, url string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errEnvelope struct {
			Error   string `json:"error"`
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &errEnvelope)
		return nil, &ReputationLogError{
			NipErrorCode: errEnvelope.Error,
			NpsStatus:    errEnvelope.Status,
			Message:      errEnvelope.Message,
		}
	}

	return data, nil
}

// Submit posts a new entry to the reputation log.
// POST /v1/log/entries
func (c *ReputationLogClient) Submit(ctx context.Context, entry ReputationLogEntry) (ReputationLogEntry, error) {
	body, err := json.Marshal(entry)
	if err != nil {
		return ReputationLogEntry{}, err
	}

	data, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/v1/log/entries", bytes.NewReader(body))
	if err != nil {
		return ReputationLogEntry{}, err
	}

	var result ReputationLogEntry
	if err := json.Unmarshal(data, &result); err != nil {
		return ReputationLogEntry{}, fmt.Errorf("nip: Submit decode: %w", err)
	}
	return result, nil
}

// Query fetches log entries for a subject NID.
// GET /v1/log/entries?nid=<nid>[&since=<seq>]
func (c *ReputationLogClient) Query(ctx context.Context, nid string, sinceSeq uint64, hasSince bool) ([]ReputationLogEntry, error) {
	url := fmt.Sprintf("%s/v1/log/entries?nid=%s", c.baseURL, nid)
	if hasSince {
		url = fmt.Sprintf("%s&since=%d", url, sinceSeq)
	}

	data, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Entries []ReputationLogEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("nip: Query decode: %w", err)
	}
	return envelope.Entries, nil
}

// GetSTH fetches the current Signed Tree Head.
// GET /v1/log/sth
func (c *ReputationLogClient) GetSTH(ctx context.Context) (SignedTreeHead, error) {
	data, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/v1/log/sth", nil)
	if err != nil {
		return SignedTreeHead{}, err
	}

	var sth SignedTreeHead
	if err := json.Unmarshal(data, &sth); err != nil {
		return SignedTreeHead{}, fmt.Errorf("nip: GetSTH decode: %w", err)
	}
	return sth, nil
}

// GetProof fetches the Merkle inclusion proof for a log entry by seq.
// GET /v1/log/proof?seq=<seq>
func (c *ReputationLogClient) GetProof(ctx context.Context, seq uint64) (InclusionProof, error) {
	url := fmt.Sprintf("%s/v1/log/proof?seq=%d", c.baseURL, seq)
	data, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return InclusionProof{}, err
	}

	var proof InclusionProof
	if err := json.Unmarshal(data, &proof); err != nil {
		return InclusionProof{}, fmt.Errorf("nip: GetProof decode: %w", err)
	}
	return proof, nil
}

// GetGossipSTH fetches the gossip-layer Signed Tree Head.
// GET /v1/log/gossip/sth
func (c *ReputationLogClient) GetGossipSTH(ctx context.Context) (SignedTreeHead, error) {
	data, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/v1/log/gossip/sth", nil)
	if err != nil {
		return SignedTreeHead{}, err
	}

	var sth SignedTreeHead
	if err := json.Unmarshal(data, &sth); err != nil {
		return SignedTreeHead{}, fmt.Errorf("nip: GetGossipSTH decode: %w", err)
	}
	return sth, nil
}
