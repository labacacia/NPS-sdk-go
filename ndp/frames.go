// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp

import (
	"sort"

	"github.com/labacacia/NPS-sdk-go/core"
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
	NID                 string
	Addresses           []map[string]any
	Caps                []string
	TTL                 uint64
	Timestamp           string
	Signature           string
	NodeType            *string
	NodeRoles           []string
	ClusterAnchor       string
	SpawnSpecRef        map[string]any // NDP v0.9: structured NdpSpawnSpecRef object
	BridgeProtocols     []string
	ActivationMode      string
	ActivationEndpoint  string
	HeartbeatIntervalMs uint64 // NDP v0.9; default 60000
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
	if f.NodeType            != nil  { d["node_type"]             = *f.NodeType }
	if len(f.NodeRoles)      > 0     { d["node_roles"]            = f.NodeRoles }
	if f.ClusterAnchor       != ""   { d["cluster_anchor"]        = f.ClusterAnchor }
	if f.SpawnSpecRef        != nil  { d["spawn_spec_ref"]        = f.SpawnSpecRef }
	if len(f.BridgeProtocols) > 0   { d["bridge_protocols"]      = f.BridgeProtocols }
	if f.ActivationMode      != ""   { d["activation_mode"]       = f.ActivationMode }
	if f.ActivationEndpoint  != ""   { d["activation_endpoint"]   = f.ActivationEndpoint }
	if f.HeartbeatIntervalMs > 0     { d["heartbeat_interval_ms"] = f.HeartbeatIntervalMs }
	return d
}

func AnnounceFrameFromDict(d core.FrameDict) *AnnounceFrame {
	// NDP spec: parsers MUST accept node_kind as a parse-time alias for node_roles.
	nodeRoles := d["node_roles"]
	if nodeRoles == nil {
		nodeRoles = d["node_kind"] // legacy alias
	}
	var spawnSpecRef map[string]any
	if v, ok := d["spawn_spec_ref"].(map[string]any); ok { spawnSpecRef = v }
	hbMs := toUint64(d["heartbeat_interval_ms"])
	if hbMs == 0 { hbMs = 60_000 }
	f := &AnnounceFrame{
		NID:                 str(d, "nid"),
		Addresses:           toSliceMap(d["addresses"]),
		Caps:                toSliceStr(d["caps"]),
		TTL:                 toUint64(d["ttl"]),
		Timestamp:           str(d, "timestamp"),
		Signature:           str(d, "signature"),
		NodeType:            optStr(d, "node_type"),
		NodeRoles:           toSliceStr(nodeRoles),
		ClusterAnchor:       str(d, "cluster_anchor"),
		SpawnSpecRef:        spawnSpecRef,
		BridgeProtocols:     toSliceStr(d["bridge_protocols"]),
		ActivationMode:      str(d, "activation_mode"),
		ActivationEndpoint:  str(d, "activation_endpoint"),
		HeartbeatIntervalMs: hbMs,
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

// GraphNode is a single node entry in a topology snapshot (NPS-4 §5).
type GraphNode struct {
	NID           string   `json:"nid"`
	ClusterAnchor string   `json:"cluster_anchor,omitempty"`
	NodeRoles     []string `json:"node_roles,omitempty"`
}

// NdpGraphEdge describes a directional link between two nodes (NPS-4 §5).
type NdpGraphEdge struct {
	FromNID   string `json:"from_nid"`
	ToNID     string `json:"to_nid"`
	LatencyMs *int   `json:"latency_ms,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

// GraphFrame is a topology snapshot frame (NPS-4 §5, alpha.11 redesign).
type GraphFrame struct {
	GraphID  string         `json:"graph_id"`
	Nodes    []GraphNode    `json:"nodes"`
	Edges    []NdpGraphEdge `json:"edges"`
	TTL      int            `json:"ttl"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func (f *GraphFrame) FrameType() core.FrameType { return core.FrameTypeGraph }

func (f *GraphFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"graph_id": f.GraphID,
		"nodes":    f.Nodes,
		"edges":    f.Edges,
		"ttl":      f.TTL,
	}
	if f.Metadata != nil { d["metadata"] = f.Metadata }
	return d
}

func graphNodeFromMap(m map[string]any) GraphNode {
	n := GraphNode{NID: "", ClusterAnchor: "", NodeRoles: nil}
	if v, ok := m["nid"].(string); ok            { n.NID = v }
	if v, ok := m["cluster_anchor"].(string); ok { n.ClusterAnchor = v }
	n.NodeRoles = toSliceStr(m["node_roles"])
	return n
}

func ndpGraphEdgeFromMap(m map[string]any) NdpGraphEdge {
	e := NdpGraphEdge{}
	if v, ok := m["from_nid"].(string); ok  { e.FromNID = v }
	if v, ok := m["to_nid"].(string); ok    { e.ToNID = v }
	if v, ok := m["protocol"].(string); ok  { e.Protocol = v }
	if v, ok := m["latency_ms"]; ok {
		switch x := v.(type) {
		case float64:
			i := int(x); e.LatencyMs = &i
		case int:
			e.LatencyMs = &x
		case int64:
			i := int(x); e.LatencyMs = &i
		}
	}
	return e
}

func GraphFrameFromDict(d core.FrameDict) *GraphFrame {
	var nodes []GraphNode
	if arr, ok := d["nodes"].([]any); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				nodes = append(nodes, graphNodeFromMap(m))
			}
		}
	}
	var edges []NdpGraphEdge
	if arr, ok := d["edges"].([]any); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				edges = append(edges, ndpGraphEdgeFromMap(m))
			}
		}
	}
	var metadata map[string]any
	if v, ok := d["metadata"].(map[string]any); ok { metadata = v }
	ttl := 0
	switch x := d["ttl"].(type) {
	case float64: ttl = int(x)
	case int:     ttl = x
	case int64:   ttl = int(x)
	}
	return &GraphFrame{
		GraphID:  str(d, "graph_id"),
		Nodes:    nodes,
		Edges:    edges,
		TTL:      ttl,
		Metadata: metadata,
	}
}
