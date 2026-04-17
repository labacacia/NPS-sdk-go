// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type anchorEntry struct {
	schema  FrameDict
	expires time.Time
}

// AnchorFrameCache stores schema maps keyed by SHA-256 anchor IDs with TTL eviction.
type AnchorFrameCache struct {
	mu    sync.RWMutex
	store map[string]anchorEntry
	// Clock is injectable for testing.
	Clock func() time.Time
}

func NewAnchorFrameCache() *AnchorFrameCache {
	return &AnchorFrameCache{
		store: make(map[string]anchorEntry),
		Clock: time.Now,
	}
}

// ComputeAnchorID returns a deterministic SHA-256 hex anchor ID for a schema map.
func ComputeAnchorID(schema FrameDict) string {
	// Sort keys for canonical JSON
	keys := make([]string, 0, len(schema))
	for k := range schema {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(schema))
	for _, k := range keys {
		ordered[k] = schema[k]
	}
	b, _ := json.Marshal(ordered)
	h := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", h)
}

// Set stores schema with ttlSecs TTL. Returns ErrAnchorPoison if the anchor ID is
// already live with a different schema.
func (c *AnchorFrameCache) Set(schema FrameDict, ttlSecs int64) (string, error) {
	id := ComputeAnchorID(schema)
	now := c.Clock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.store[id]; ok && e.expires.After(now) {
		if !mapsEqual(e.schema, schema) {
			return "", &ErrAnchorPoison{ID: id}
		}
	}
	c.store[id] = anchorEntry{schema: schema, expires: now.Add(time.Duration(ttlSecs) * time.Second)}
	return id, nil
}

// Get returns the schema or nil if expired / missing.
func (c *AnchorFrameCache) Get(id string) FrameDict {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.store[id]
	if !ok || !e.expires.After(c.Clock()) {
		return nil
	}
	return e.schema
}

// GetRequired returns the schema or ErrAnchorNotFound.
func (c *AnchorFrameCache) GetRequired(id string) (FrameDict, error) {
	v := c.Get(id)
	if v == nil {
		return nil, &ErrAnchorNotFound{ID: id}
	}
	return v, nil
}

// Invalidate removes an entry.
func (c *AnchorFrameCache) Invalidate(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, id)
}

// Len returns the count of live (non-expired) entries.
func (c *AnchorFrameCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := c.Clock()
	n := 0
	for _, e := range c.store {
		if e.expires.After(now) {
			n++
		}
	}
	return n
}

func mapsEqual(a, b FrameDict) bool {
	if len(a) != len(b) {
		return false
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}
