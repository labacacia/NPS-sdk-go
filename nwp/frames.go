// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
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
func optUint64(d core.FrameDict, k string) *uint64 {
	switch x := d[k].(type) {
	case int64:   v := uint64(x); return &v
	case uint64:  return &x
	case float64: v := uint64(x); return &v
	}
	return nil
}
func optBool(d core.FrameDict, k string) bool {
	v, _ := d[k].(bool)
	return v
}

// ── QueryFrame ────────────────────────────────────────────────────────────────

type QueryFrame struct {
	AnchorRef   string
	Filter      any
	Order       any
	TokenBudget *uint64
	Limit       *uint64
	Cursor      string `json:"cursor,omitempty"`
}

func (f *QueryFrame) FrameType() core.FrameType { return core.FrameTypeQuery }

func (f *QueryFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"anchor_ref": f.AnchorRef}
	if f.Filter != nil      { d["filter"] = f.Filter }
	if f.Order != nil       { d["order"] = f.Order }
	if f.TokenBudget != nil { d["token_budget"] = *f.TokenBudget }
	if f.Limit != nil       { d["limit"] = *f.Limit }
	if f.Cursor != ""       { d["cursor"] = f.Cursor }
	return d
}

func QueryFrameFromDict(d core.FrameDict) *QueryFrame {
	return &QueryFrame{
		AnchorRef:   str(d, "anchor_ref"),
		Filter:      d["filter"],
		Order:       d["order"],
		TokenBudget: optUint64(d, "token_budget"),
		Limit:       optUint64(d, "limit"),
		Cursor:      str(d, "cursor"),
	}
}

// ── ActionFrame ───────────────────────────────────────────────────────────────

type ActionFrame struct {
	Action      string
	Params      any
	AnchorRef   *string
	Async       bool
	CallbackURL string `json:"callback_url,omitempty"`
	Priority    string `json:"priority,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
}

func (f *ActionFrame) FrameType() core.FrameType { return core.FrameTypeAction }

func (f *ActionFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"action": f.Action, "async": f.Async}
	if f.Params != nil       { d["params"] = f.Params }
	if f.AnchorRef != nil    { d["anchor_ref"] = *f.AnchorRef }
	if f.CallbackURL != ""   { d["callback_url"] = f.CallbackURL }
	if f.Priority != ""      { d["priority"] = f.Priority }
	if f.RequestID != ""     { d["request_id"] = f.RequestID }
	return d
}

func ActionFrameFromDict(d core.FrameDict) *ActionFrame {
	return &ActionFrame{
		Action:      str(d, "action"),
		Params:      d["params"],
		AnchorRef:   optStr(d, "anchor_ref"),
		Async:       optBool(d, "async"),
		CallbackURL: str(d, "callback_url"),
		Priority:    str(d, "priority"),
		RequestID:   str(d, "request_id"),
	}
}

// ── SubscribeFrame ────────────────────────────────────────────────────────────

type SubscribeFrame struct {
	Action            string `json:"action"`
	StreamID          string `json:"stream_id"`
	AnchorRef         string `json:"anchor_ref,omitempty"`
	Filter            any    `json:"filter,omitempty"`
	HeartbeatInterval uint32 `json:"heartbeat_interval,omitempty"`
	ResumeFromSeq     uint64 `json:"resume_from_seq,omitempty"`
	Type              string `json:"type,omitempty"`
}

func (f *SubscribeFrame) FrameType() core.FrameType { return core.FrameTypeSubscribe }

func (f *SubscribeFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"action":    f.Action,
		"stream_id": f.StreamID,
	}
	if f.AnchorRef != ""         { d["anchor_ref"] = f.AnchorRef }
	if f.Filter != nil           { d["filter"] = f.Filter }
	if f.HeartbeatInterval != 0  { d["heartbeat_interval"] = f.HeartbeatInterval }
	if f.ResumeFromSeq != 0      { d["resume_from_seq"] = f.ResumeFromSeq }
	if f.Type != ""              { d["type"] = f.Type }
	return d
}

// ── AsyncActionResponse ───────────────────────────────────────────────────────

type AsyncActionResponse struct {
	TaskID      string
	StatusURL   *string
	CallbackURL *string
}

func AsyncActionResponseFromDict(d core.FrameDict) *AsyncActionResponse {
	return &AsyncActionResponse{
		TaskID:      str(d, "task_id"),
		StatusURL:   optStr(d, "status_url"),
		CallbackURL: optStr(d, "callback_url"),
	}
}
