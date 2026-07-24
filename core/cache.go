// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
//
// When the schema carries a structured "fields" array (the FrameSchema shape used
// by AnchorFrame — NPS-1 §4.1), the anchor_id is computed over the RFC 8785 JCS
// canonical form matching the .NET reference SDK (AnchorIdComputer). Otherwise, for
// backward compatibility with generic dicts, it falls back to key-sorted json.Marshal.
func ComputeAnchorID(schema FrameDict) string {
	if canonical, ok := canonicalFrameSchemaJSON(schema); ok {
		h := sha256.Sum256([]byte(canonical))
		return fmt.Sprintf("sha256:%x", h)
	}
	// Fallback: key-sorted generic dict.
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

// canonicalFrameSchemaJSON emits the RFC 8785 JCS canonical form of a structured
// FrameSchema: {"fields":[{"name":..,"nullable":true|false,"semantic":..(omitted if
// null/empty),"type":..}, ...]}. Per-field JCS key order is exactly name, nullable,
// semantic (only if non-empty), type. Strings are emitted without HTML-escaping and
// without extra whitespace, matching the .NET AnchorIdComputer.
//
// Returns (canonical, true) only when schema has a "fields" value that is a list of
// field maps; otherwise (\"\", false) so the caller can fall back.
func canonicalFrameSchemaJSON(schema FrameDict) (string, bool) {
	raw, ok := schema["fields"]
	if !ok {
		return "", false
	}
	var fields []any
	switch v := raw.(type) {
	case []any:
		fields = v
	case []map[string]any:
		fields = make([]any, len(v))
		for i := range v {
			fields[i] = v[i]
		}
	default:
		return "", false
	}

	var sb strings.Builder
	sb.WriteString(`{"fields":[`)
	for i, fAny := range fields {
		fm, ok := fAny.(map[string]any)
		if !ok {
			return "", false
		}
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('{')

		// name
		sb.WriteString(`"name":`)
		writeJCSString(&sb, jcsStr(fm["name"]))

		// nullable (defaults to false when absent)
		sb.WriteString(`,"nullable":`)
		if b, _ := fm["nullable"].(bool); b {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}

		// semantic — omitted when null/empty
		if s := jcsStr(fm["semantic"]); s != "" {
			sb.WriteString(`,"semantic":`)
			writeJCSString(&sb, s)
		}

		// type
		sb.WriteString(`,"type":`)
		writeJCSString(&sb, jcsStr(fm["type"]))

		sb.WriteByte('}')
	}
	sb.WriteString("]}")
	return sb.String(), true
}

func jcsStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// writeJCSString appends a JSON string literal with RFC 8259 escaping and WITHOUT
// Go's default HTML escaping of <, >, & (which encoding/json applies). This matches
// System.Text.Json used by the .NET reference.
func writeJCSString(sb *strings.Builder, s string) {
	b, _ := json.Marshal(s)
	sb.WriteString(unescapeHTML(string(b)))
}

// unescapeHTML reverses encoding/json's HTML escaping of <, >, & so the canonical
// form matches the reference SDK. Safe because these are the only three sequences
// json escapes beyond the mandatory JSON escapes.
func unescapeHTML(s string) string {
	r := strings.NewReplacer(`<`, "<", `>`, ">", `&`, "&")
	return r.Replace(s)
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
