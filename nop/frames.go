// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import "github.com/labacacia/NPS-sdk-go/core"

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
	}
	return 0
}
func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:   return x
	case float64: return int64(x)
	}
	return 0
}
func optUint64(d core.FrameDict, k string) *uint64 {
	if v, ok := d[k]; ok {
		u := toUint64(v)
		return &u
	}
	return nil
}
func optInt64(d core.FrameDict, k string) *int64 {
	if v, ok := d[k]; ok {
		i := toInt64(v)
		return &i
	}
	return nil
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

// ── TaskFrame ─────────────────────────────────────────────────────────────────

type TaskFrame struct {
	TaskID      string
	DAG         any
	TimeoutMs   *uint64
	CallbackURL *string
	Context     any
	Priority    *string
	Depth       *int64
}

func (f *TaskFrame) FrameType() core.FrameType { return core.FrameTypeTask }

func (f *TaskFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"task_id": f.TaskID, "dag": f.DAG}
	if f.TimeoutMs   != nil { d["timeout_ms"]   = *f.TimeoutMs }
	if f.CallbackURL != nil { d["callback_url"]  = *f.CallbackURL }
	if f.Context     != nil { d["context"]       = f.Context }
	if f.Priority    != nil { d["priority"]      = *f.Priority }
	if f.Depth       != nil { d["depth"]         = *f.Depth }
	return d
}

func TaskFrameFromDict(d core.FrameDict) *TaskFrame {
	return &TaskFrame{
		TaskID:      str(d, "task_id"),
		DAG:         d["dag"],
		TimeoutMs:   optUint64(d, "timeout_ms"),
		CallbackURL: optStr(d, "callback_url"),
		Context:     d["context"],
		Priority:    optStr(d, "priority"),
		Depth:       optInt64(d, "depth"),
	}
}

// ── DelegateFrame ─────────────────────────────────────────────────────────────

type DelegateFrame struct {
	TaskID         string
	SubtaskID      string
	Action         string
	TargetNID      string
	Inputs         any
	Config         any
	IdempotencyKey *string
}

func (f *DelegateFrame) FrameType() core.FrameType { return core.FrameTypeDelegate }

func (f *DelegateFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"task_id":    f.TaskID,
		"subtask_id": f.SubtaskID,
		"action":     f.Action,
		"target_nid": f.TargetNID,
	}
	if f.Inputs          != nil { d["inputs"]           = f.Inputs }
	if f.Config          != nil { d["config"]           = f.Config }
	if f.IdempotencyKey  != nil { d["idempotency_key"]  = *f.IdempotencyKey }
	return d
}

func DelegateFrameFromDict(d core.FrameDict) *DelegateFrame {
	return &DelegateFrame{
		TaskID:         str(d, "task_id"),
		SubtaskID:      str(d, "subtask_id"),
		Action:         str(d, "action"),
		TargetNID:      str(d, "target_nid"),
		Inputs:         d["inputs"],
		Config:         d["config"],
		IdempotencyKey: optStr(d, "idempotency_key"),
	}
}

// ── SyncFrame ─────────────────────────────────────────────────────────────────

type SyncFrame struct {
	TaskID      string
	SyncID      string
	SubtaskIDs  []string
	MinRequired int64
	Aggregate   string
	TimeoutMs   *uint64
}

func (f *SyncFrame) FrameType() core.FrameType { return core.FrameTypeSync }

func (f *SyncFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"task_id":      f.TaskID,
		"sync_id":      f.SyncID,
		"subtask_ids":  f.SubtaskIDs,
		"min_required": f.MinRequired,
		"aggregate":    f.Aggregate,
	}
	if f.TimeoutMs != nil { d["timeout_ms"] = *f.TimeoutMs }
	return d
}

func SyncFrameFromDict(d core.FrameDict) *SyncFrame {
	agg := str(d, "aggregate")
	if agg == "" { agg = "merge" }
	return &SyncFrame{
		TaskID:      str(d, "task_id"),
		SyncID:      str(d, "sync_id"),
		SubtaskIDs:  toSliceStr(d["subtask_ids"]),
		MinRequired: toInt64(d["min_required"]),
		Aggregate:   agg,
		TimeoutMs:   optUint64(d, "timeout_ms"),
	}
}

// ── AlignStreamFrame ──────────────────────────────────────────────────────────

type AlignStreamFrame struct {
	SyncID     string
	TaskID     string
	SubtaskID  string
	Seq        uint64
	IsFinal    bool
	SourceNID  *string
	Result     any
	Error      map[string]any
	WindowSize *uint64
}

func (f *AlignStreamFrame) FrameType() core.FrameType { return core.FrameTypeAlignStream }

func (f *AlignStreamFrame) ErrorCode() string {
	if f.Error == nil { return "" }
	v, _ := f.Error["error_code"].(string)
	return v
}

func (f *AlignStreamFrame) ErrorMessage() string {
	if f.Error == nil { return "" }
	v, _ := f.Error["message"].(string)
	return v
}

func (f *AlignStreamFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"sync_id":    f.SyncID,
		"task_id":    f.TaskID,
		"subtask_id": f.SubtaskID,
		"seq":        f.Seq,
		"is_final":   f.IsFinal,
	}
	if f.SourceNID  != nil { d["source_nid"]  = *f.SourceNID }
	if f.Result     != nil { d["result"]      = f.Result }
	if f.Error      != nil { d["error"]       = f.Error }
	if f.WindowSize != nil { d["window_size"] = *f.WindowSize }
	return d
}

func AlignStreamFrameFromDict(d core.FrameDict) *AlignStreamFrame {
	isFinal, _ := d["is_final"].(bool)
	var errMap map[string]any
	if v, ok := d["error"].(map[string]any); ok { errMap = v }
	return &AlignStreamFrame{
		SyncID:     str(d, "sync_id"),
		TaskID:     str(d, "task_id"),
		SubtaskID:  str(d, "subtask_id"),
		Seq:        toUint64(d["seq"]),
		IsFinal:    isFinal,
		SourceNID:  optStr(d, "source_nid"),
		Result:     d["result"],
		Error:      errMap,
		WindowSize: optUint64(d, "window_size"),
	}
}
