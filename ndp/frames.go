// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp

import (
	"sort"

	"github.com/labacacia/nps/impl/go/core"
)

func str(d core.FrameDict, k string) string {
	if v, ok := d[k].(string); ok { return v }
	return ""
}
func optStr(d core.FrameDict, k string) *string {
	if v, ok := d[k].(string); ok { return &v }
	return nil
}
func toUint64(v any) uint64 {
	switch x := v.(type) {
	case int64:   return uint64(x)
	case uint64:  return x
	case float64: return uint64(x)
	case int:     return uint64(x)
	}
	return 0
}
func toSliceStr(v any) []string {
	if ss, ok := v.([]string); ok { return ss }
	arr, ok := v.([]any)
	if !ok { return nil }
	out := make([]string, 0, len(arr))
	for _, a := range arr {
		if s, ok := a.(string); ok { out = append(out, s) }
	}
	return out
}
func toSliceMap(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok { return nil }
	out := make([]map[string]any, 0, len(arr))
	for _, a := range arr {
		if m, ok := a.(map[string]any); ok { out = append(out, m) }
	}
	return out
}

// ── AnnounceFrame ─────────────────────────────────────────────────────────────

type AnnounceFrame struct {
	NID       string
	Addresses []map[string]any
	Caps      []string
	TTL       uint64
	Timestamp string
	Signature string
	NodeType  *string
}

func (f *AnnounceFrame) FrameType() core.FrameType { return core.FrameTypeAnnounce }

// UnsignedDict returns the canonical dict without signature (for signing).
func (f *AnnounceFrame) UnsignedDict() core.FrameDict {
	d := core.FrameDict{
		"nid":       f.NID,
		"addresses": f.Addresses,
		"caps":      f.Caps,
		"ttl":       f.TTL,
		"timestamp": f.Timestamp,
		"node_type": nil,
	}
	// Sort keys for canonical representation
	return sortedDict(d)
}

func (f *AnnounceFrame) ToDict() core.FrameDict {
	d := f.UnsignedDict()
	d["signature"] = f.Signature
	if f.NodeType != nil { d["node_type"] = *f.NodeType }
	return d
}

func AnnounceFrameFromDict(d core.FrameDict) *AnnounceFrame {
	f := &AnnounceFrame{
		NID:       str(d, "nid"),
		Addresses: toSliceMap(d["addresses"]),
		Caps:      toSliceStr(d["caps"]),
		TTL:       toUint64(d["ttl"]),
		Timestamp: str(d, "timestamp"),
		Signature: str(d, "signature"),
		NodeType:  optStr(d, "node_type"),
	}
	if f.TTL == 0 { f.TTL = 300 }
	return f
}

func sortedDict(d core.FrameDict) core.FrameDict {
	keys := make([]string, 0, len(d))
	for k := range d { keys = append(keys, k) }
	sort.Strings(keys)
	out := make(core.FrameDict, len(d))
	for _, k := range keys { out[k] = d[k] }
	return out
}

// ── ResolveFrame ──────────────────────────────────────────────────────────────

type ResolveFrame struct {
	Target       string
	RequesterNID *string
	Resolved     map[string]any
}

func (f *ResolveFrame) FrameType() core.FrameType { return core.FrameTypeResolve }

func (f *ResolveFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"target": f.Target}
	if f.RequesterNID != nil { d["requester_nid"] = *f.RequesterNID }
	if f.Resolved != nil    { d["resolved"] = f.Resolved }
	return d
}

func ResolveFrameFromDict(d core.FrameDict) *ResolveFrame {
	var resolved map[string]any
	if v, ok := d["resolved"].(map[string]any); ok { resolved = v }
	return &ResolveFrame{
		Target:       str(d, "target"),
		RequesterNID: optStr(d, "requester_nid"),
		Resolved:     resolved,
	}
}

// ── GraphFrame ────────────────────────────────────────────────────────────────

type GraphFrame struct {
	Seq         uint64
	InitialSync bool
	Nodes       []any
	Patch       []any
}

func (f *GraphFrame) FrameType() core.FrameType { return core.FrameTypeGraph }

func (f *GraphFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"seq":          f.Seq,
		"initial_sync": f.InitialSync,
		"nodes":        f.Nodes,
	}
	if f.Patch != nil { d["patch"] = f.Patch }
	return d
}

func GraphFrameFromDict(d core.FrameDict) *GraphFrame {
	iSync, _ := d["initial_sync"].(bool)
	var nodes, patch []any
	if v, ok := d["nodes"].([]any); ok { nodes = v }
	if v, ok := d["patch"].([]any); ok  { patch = v }
	return &GraphFrame{
		Seq:         toUint64(d["seq"]),
		InitialSync: iSync,
		Nodes:       nodes,
		Patch:       patch,
	}
}
