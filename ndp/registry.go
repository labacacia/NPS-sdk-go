// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp

import (
	"strings"
	"sync"
	"time"
)

// ResolveResult holds the resolved address details.
type ResolveResult struct {
	Host     string
	Port     uint64
	Protocol string
}

type registryEntry struct {
	frame   *AnnounceFrame
	expires time.Time
}

// InMemoryNdpRegistry stores AnnounceFrames with TTL eviction.
type InMemoryNdpRegistry struct {
	mu    sync.RWMutex
	store map[string]*registryEntry
	// Clock is injectable for testing.
	Clock func() time.Time
}

func NewInMemoryNdpRegistry() *InMemoryNdpRegistry {
	return &InMemoryNdpRegistry{
		store: make(map[string]*registryEntry),
		Clock: time.Now,
	}
}

// Announce stores or removes (ttl=0) an AnnounceFrame.
func (r *InMemoryNdpRegistry) Announce(frame *AnnounceFrame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if frame.TTL == 0 {
		delete(r.store, frame.NID)
		return
	}
	expires := r.Clock().Add(time.Duration(frame.TTL) * time.Second)
	r.store[frame.NID] = &registryEntry{frame: frame, expires: expires}
}

// GetByNID returns the frame or nil if not found / expired.
func (r *InMemoryNdpRegistry) GetByNID(nid string) *AnnounceFrame {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.store[nid]
	if !ok || !e.expires.After(r.Clock()) {
		return nil
	}
	return e.frame
}

// Resolve finds a live entry whose NID matches the nwp:// target URL.
func (r *InMemoryNdpRegistry) Resolve(target string) *ResolveResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.Clock()
	for _, e := range r.store {
		if !e.expires.After(now) {
			continue
		}
		if NwpTargetMatchesNID(e.frame.NID, target) && len(e.frame.Addresses) > 0 {
			addr := e.frame.Addresses[0]
			host, _ := addr["host"].(string)
			port := toUint64(addr["port"])
			proto, _ := addr["protocol"].(string)
			return &ResolveResult{Host: host, Port: port, Protocol: proto}
		}
	}
	return nil
}

// GetAll returns all live frames.
func (r *InMemoryNdpRegistry) GetAll() []*AnnounceFrame {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.Clock()
	var out []*AnnounceFrame
	for _, e := range r.store {
		if e.expires.After(now) {
			out = append(out, e.frame)
		}
	}
	return out
}

// NwpTargetMatchesNID matches a nwp://authority/path URL against urn:nps:node:{host}:{path}.
func NwpTargetMatchesNID(nid, target string) bool {
	parts := strings.Split(nid, ":")
	if len(parts) < 5 || parts[0] != "urn" || parts[1] != "nps" || parts[2] != "node" {
		return false
	}
	nidHost := parts[3]
	nidPath := strings.Join(parts[4:], "/")

	rest, ok := strings.CutPrefix(target, "nwp://")
	if !ok {
		return false
	}
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return false
	}
	authority := rest[:slashIdx]
	path := rest[slashIdx+1:]

	if authority != nidHost {
		return false
	}
	return path == nidPath || strings.HasPrefix(path, nidPath+"/")
}
