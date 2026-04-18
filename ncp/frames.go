// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"github.com/labacacia/NPS-sdk-go/core"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func str(d core.FrameDict, k string) string {
	if v, ok := d[k].(string); ok {
		return v
	}
	return ""
}

func optStr(d core.FrameDict, k string) *string {
	if v, ok := d[k].(string); ok {
		return &v
	}
	return nil
}

func toUint64(v any) uint64 {
	switch x := v.(type) {
	case int64:
		return uint64(x)
	case uint64:
		return x
	case float64:
		return uint64(x)
	case int:
		return uint64(x)
	}
	return 0
}

// ── AnchorFrame ───────────────────────────────────────────────────────────────

type AnchorFrame struct {
	AnchorID    string
	Schema      core.FrameDict
	Namespace   *string
	Description *string
	NodeType    *string
	TTL         uint64
}

func (f *AnchorFrame) FrameType() core.FrameType { return core.FrameTypeAnchor }

func (f *AnchorFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"anchor_id": f.AnchorID,
		"schema":    f.Schema,
		"ttl":       f.TTL,
	}
	if f.Namespace != nil   { d["namespace"] = *f.Namespace }
	if f.Description != nil { d["description"] = *f.Description }
	if f.NodeType != nil    { d["node_type"] = *f.NodeType }
	return d
}

func AnchorFrameFromDict(d core.FrameDict) *AnchorFrame {
	schema, _ := d["schema"].(map[string]any)
	f := &AnchorFrame{
		AnchorID: str(d, "anchor_id"),
		Schema:   schema,
		TTL:      toUint64(d["ttl"]),
	}
	if f.TTL == 0 { f.TTL = 3600 }
	f.Namespace   = optStr(d, "namespace")
	f.Description = optStr(d, "description")
	f.NodeType    = optStr(d, "node_type")
	return f
}

// ── DiffFrame ─────────────────────────────────────────────────────────────────

type DiffFrame struct {
	AnchorID    string
	NewAnchorID string
	Patch       []any
}

func (f *DiffFrame) FrameType() core.FrameType { return core.FrameTypeDiff }

func (f *DiffFrame) ToDict() core.FrameDict {
	return core.FrameDict{
		"anchor_id":     f.AnchorID,
		"new_anchor_id": f.NewAnchorID,
		"patch":         f.Patch,
	}
}

func DiffFrameFromDict(d core.FrameDict) *DiffFrame {
	var patch []any
	if v, ok := d["patch"].([]any); ok {
		patch = v
	}
	return &DiffFrame{
		AnchorID:    str(d, "anchor_id"),
		NewAnchorID: str(d, "new_anchor_id"),
		Patch:       patch,
	}
}

// ── StreamFrame ───────────────────────────────────────────────────────────────

type StreamFrame struct {
	AnchorID string
	Seq      uint64
	Payload  any
	IsLast   bool
}

func (f *StreamFrame) FrameType() core.FrameType { return core.FrameTypeStream }

func (f *StreamFrame) ToDict() core.FrameDict {
	return core.FrameDict{
		"anchor_id": f.AnchorID,
		"seq":       f.Seq,
		"payload":   f.Payload,
		"is_last":   f.IsLast,
	}
}

func StreamFrameFromDict(d core.FrameDict) *StreamFrame {
	isLast, _ := d["is_last"].(bool)
	return &StreamFrame{
		AnchorID: str(d, "anchor_id"),
		Seq:      toUint64(d["seq"]),
		Payload:  d["payload"],
		IsLast:   isLast,
	}
}

// ── CapsFrame ─────────────────────────────────────────────────────────────────

type CapsFrame struct {
	NodeID    string
	Caps      []string
	AnchorRef *string
	Payload   any
}

func (f *CapsFrame) FrameType() core.FrameType { return core.FrameTypeCaps }

func (f *CapsFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"node_id": f.NodeID, "caps": f.Caps}
	if f.AnchorRef != nil { d["anchor_ref"] = *f.AnchorRef }
	if f.Payload != nil   { d["payload"] = f.Payload }
	return d
}

func CapsFrameFromDict(d core.FrameDict) *CapsFrame {
	var caps []string
	switch v := d["caps"].(type) {
	case []string:
		caps = v
	case []any:
		for _, c := range v {
			if s, ok := c.(string); ok {
				caps = append(caps, s)
			}
		}
	}
	return &CapsFrame{
		NodeID:    str(d, "node_id"),
		Caps:      caps,
		AnchorRef: optStr(d, "anchor_ref"),
		Payload:   d["payload"],
	}
}

// ── ErrorFrame ────────────────────────────────────────────────────────────────

type ErrorFrame struct {
	ErrorCode string
	Message   string
	Detail    any
}

func (f *ErrorFrame) FrameType() core.FrameType { return core.FrameTypeError }

func (f *ErrorFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"error_code": f.ErrorCode, "message": f.Message}
	if f.Detail != nil { d["detail"] = f.Detail }
	return d
}

func ErrorFrameFromDict(d core.FrameDict) *ErrorFrame {
	return &ErrorFrame{
		ErrorCode: str(d, "error_code"),
		Message:   str(d, "message"),
		Detail:    d["detail"],
	}
}
