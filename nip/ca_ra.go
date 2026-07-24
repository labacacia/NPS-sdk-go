// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// EnrollmentTier selects the RA gate an inbound registration must pass
// (NPS-CR-0005 §3).
type EnrollmentTier int

const (
	// EnrollmentTierAllowlist — Tier 1: glob allowlist on identifier.
	EnrollmentTierAllowlist EnrollmentTier = 1
	// EnrollmentTierBootstrapToken — Tier 2: single-use bootstrap token.
	EnrollmentTierBootstrapToken EnrollmentTier = 2
	// EnrollmentTierPendingQueue — Tier 3: queue for operator approval.
	EnrollmentTierPendingQueue EnrollmentTier = 3
)

// NipRaPendingError signals that a Tier-3 policy queued the request; the
// router translates this into 202 Accepted.
type NipRaPendingError struct {
	PendingID string
}

func (e *NipRaPendingError) Error() string {
	return fmt.Sprintf("Registration queued with pending id: %s", e.PendingID)
}

// EnrollmentPolicy is the gate that must pass before the CA issues an
// IdentFrame (NPS-CR-0005 §3).
type EnrollmentPolicy interface {
	// Check returns a *NipCaError on denial or *NipRaPendingError when queued.
	Check(entityType, identifier, pubKey string, capabilities []string, scopeJSON, metadataJSON, enrollmentToken string) error
}

// ── Tier 1: Allowlist ──────────────────────────────────────────────────────────

// AllowlistPolicy admits identifiers matching at least one glob pattern.
type AllowlistPolicy struct {
	compiled []*regexp.Regexp
}

// NewAllowlistPolicy compiles the given glob patterns.
func NewAllowlistPolicy(patterns []string) *AllowlistPolicy {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, globToRegex(p))
	}
	return &AllowlistPolicy{compiled: compiled}
}

func (p *AllowlistPolicy) Check(_, identifier, _ string, _ []string, _, _, _ string) error {
	for _, re := range p.compiled {
		if re.MatchString(identifier) {
			return nil
		}
	}
	return newCaErr(ErrRaNidNotAllowed, "Identifier '%s' does not match any enrollment allowlist pattern.", identifier)
}

func globToRegex(pattern string) *regexp.Regexp {
	if pattern == "*" {
		return regexp.MustCompile(".*")
	}
	escaped := regexp.QuoteMeta(pattern)
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")
	return regexp.MustCompile("^" + escaped + "$")
}

// ── Tier 2: Bootstrap token ─────────────────────────────────────────────────────

// BootstrapTokenInfo is public token metadata (token value excluded).
type BootstrapTokenInfo struct {
	Id        string
	Label     string
	CreatedAt time.Time
	ExpiresAt time.Time
	Consumed  bool
	Revoked   bool
}

// BootstrapTokenStore persists single-use enrollment bootstrap tokens
// (NPS-CR-0005 §3.3). Tokens are stored as SHA-256 hashes.
type BootstrapTokenStore interface {
	Create(label string, expiresAt time.Time) (string, error)
	ValidateAndConsume(token string) (bool, error)
	List() ([]BootstrapTokenInfo, error)
	Revoke(tokenID string) (bool, error)
}

// BootstrapTokenPolicy requires a valid single-use bootstrap token (Tier 2).
type BootstrapTokenPolicy struct {
	store BootstrapTokenStore
}

// NewBootstrapTokenPolicy builds a Tier-2 policy.
func NewBootstrapTokenPolicy(store BootstrapTokenStore) *BootstrapTokenPolicy {
	return &BootstrapTokenPolicy{store: store}
}

func (p *BootstrapTokenPolicy) Check(_, _, _ string, _ []string, _, _, enrollmentToken string) error {
	if enrollmentToken == "" || !strings.HasPrefix(enrollmentToken, "nps-bootstrap-") {
		return newCaErr(ErrRaTokenInvalid, "A bootstrap token (prefix 'nps-bootstrap-') is required for enrollment.")
	}
	valid, err := p.store.ValidateAndConsume(enrollmentToken)
	if err != nil {
		return err
	}
	if !valid {
		return newCaErr(ErrRaTokenExpired, "Bootstrap token is invalid, expired, or already consumed.")
	}
	return nil
}

type bootstrapEntry struct {
	id        string
	hash      [32]byte
	label     string
	createdAt time.Time
	expiresAt time.Time
	consumed  bool
	revoked   bool
}

// InMemoryBootstrapTokenStore is an in-memory BootstrapTokenStore.
type InMemoryBootstrapTokenStore struct {
	mu     sync.Mutex
	tokens []*bootstrapEntry
}

// NewInMemoryBootstrapTokenStore builds an empty in-memory token store.
func NewInMemoryBootstrapTokenStore() *InMemoryBootstrapTokenStore {
	return &InMemoryBootstrapTokenStore{}
}

func (s *InMemoryBootstrapTokenStore) Create(label string, expiresAt time.Time) (string, error) {
	raw := "nps-bootstrap-" + randHexN(16)
	hash := sha256.Sum256([]byte(raw))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = append(s.tokens, &bootstrapEntry{
		id: randHexN(16), hash: hash, label: label,
		createdAt: time.Now().UTC(), expiresAt: expiresAt,
	})
	return raw, nil
}

func (s *InMemoryBootstrapTokenStore) ValidateAndConsume(token string) (bool, error) {
	hash := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.tokens {
		if e.consumed || e.revoked {
			continue
		}
		if time.Now().UTC().After(e.expiresAt) {
			continue
		}
		if subtle.ConstantTimeCompare(hash[:], e.hash[:]) != 1 {
			continue
		}
		e.consumed = true
		return true, nil
	}
	return false, nil
}

func (s *InMemoryBootstrapTokenStore) List() ([]BootstrapTokenInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]BootstrapTokenInfo, 0, len(s.tokens))
	for _, e := range s.tokens {
		out = append(out, BootstrapTokenInfo{
			Id: e.id, Label: e.label, CreatedAt: e.createdAt,
			ExpiresAt: e.expiresAt, Consumed: e.consumed, Revoked: e.revoked,
		})
	}
	return out, nil
}

func (s *InMemoryBootstrapTokenStore) Revoke(tokenID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.tokens {
		if e.id != tokenID {
			continue
		}
		if e.consumed || e.revoked {
			return false, nil
		}
		e.revoked = true
		return true, nil
	}
	return false, nil
}

// ── Tier 3: Pending queue ───────────────────────────────────────────────────────

// PendingStatus is the lifecycle state of a pending registration.
type PendingStatus int

const (
	PendingStatusPending  PendingStatus = 0
	PendingStatusApproved PendingStatus = 1
	PendingStatusRejected PendingStatus = 2
)

// String returns the lowercase wire label.
func (s PendingStatus) String() string {
	switch s {
	case PendingStatusApproved:
		return "approved"
	case PendingStatusRejected:
		return "rejected"
	default:
		return "pending"
	}
}

// PendingRegistration is a registration request awaiting operator approval.
type PendingRegistration struct {
	Id           string
	EntityType   string
	Identifier   string
	PubKey       string
	Capabilities []string
	ScopeJson    string
	MetadataJson string
	RequestedAt  time.Time
	Status       PendingStatus
	RejectReason string
}

// PendingStore holds pending registration requests (NPS-CR-0005 §3.4).
type PendingStore interface {
	Enqueue(request PendingRegistration) (string, error)
	List() ([]PendingRegistration, error)
	Get(id string) (*PendingRegistration, error)
	Approve(id string) (bool, error)
	Reject(id, reason string) (bool, error)
	PendingCount() int
}

// PendingQueuePolicy queues every registration as pending (Tier 3).
type PendingQueuePolicy struct {
	store   PendingStore
	maxSize int
}

// NewPendingQueuePolicy builds a Tier-3 policy.
func NewPendingQueuePolicy(store PendingStore, maxSize int) *PendingQueuePolicy {
	return &PendingQueuePolicy{store: store, maxSize: maxSize}
}

func (p *PendingQueuePolicy) Check(entityType, identifier, pubKey string, capabilities []string, scopeJSON, metadataJSON, _ string) error {
	if p.store.PendingCount() >= p.maxSize {
		return newCaErr(ErrRaTokenInvalid, "Pending enrollment queue is full (max %d). Retry later.", p.maxSize)
	}
	id := randHexN(16)
	req := PendingRegistration{
		Id: id, EntityType: entityType, Identifier: identifier, PubKey: pubKey,
		Capabilities: capabilities, ScopeJson: scopeJSON, MetadataJson: metadataJSON,
		RequestedAt: time.Now().UTC(), Status: PendingStatusPending,
	}
	if _, err := p.store.Enqueue(req); err != nil {
		return err
	}
	return &NipRaPendingError{PendingID: id}
}

// InMemoryPendingStore is an in-memory PendingStore. A best-effort sweep on
// each write removes records older than maxAge to bound memory growth.
type InMemoryPendingStore struct {
	mu      sync.Mutex
	records map[string]PendingRegistration
	maxAge  time.Duration
}

// NewInMemoryPendingStore builds an in-memory pending store.
func NewInMemoryPendingStore(maxAge time.Duration) *InMemoryPendingStore {
	return &InMemoryPendingStore{records: map[string]PendingRegistration{}, maxAge: maxAge}
}

func (s *InMemoryPendingStore) sweepLocked() {
	if s.maxAge <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-s.maxAge)
	for id, r := range s.records {
		if r.Status != PendingStatusPending && r.RequestedAt.Before(cutoff) {
			delete(s.records, id)
		}
	}
}

func (s *InMemoryPendingStore) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.records {
		if r.Status == PendingStatusPending {
			n++
		}
	}
	return n
}

func (s *InMemoryPendingStore) Enqueue(request PendingRegistration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	s.records[request.Id] = request
	return request.Id, nil
}

func (s *InMemoryPendingStore) List() ([]PendingRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PendingRegistration, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	return out, nil
}

func (s *InMemoryPendingStore) Get(id string) (*PendingRegistration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.records[id]; ok {
		return &r, nil
	}
	return nil, nil
}

func (s *InMemoryPendingStore) Approve(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok || r.Status != PendingStatusPending {
		return false, nil
	}
	r.Status = PendingStatusApproved
	s.records[id] = r
	return true, nil
}

func (s *InMemoryPendingStore) Reject(id, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok || r.Status != PendingStatusPending {
		return false, nil
	}
	r.Status = PendingStatusRejected
	r.RejectReason = reason
	s.records[id] = r
	return true, nil
}
