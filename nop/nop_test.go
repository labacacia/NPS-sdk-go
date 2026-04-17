// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop_test

import (
	"strings"
	"testing"

	"github.com/labacacia/nps/impl/go/nop"
)

// ── BackoffStrategy ───────────────────────────────────────────────────────────

func TestBackoff_Fixed(t *testing.T) {
	d := nop.ComputeDelayMs(nop.BackoffFixed, 100, 5000, 0)
	if d != 100 {
		t.Errorf("fixed attempt 0: got %d", d)
	}
	d = nop.ComputeDelayMs(nop.BackoffFixed, 100, 5000, 5)
	if d != 100 {
		t.Errorf("fixed attempt 5: got %d", d)
	}
}

func TestBackoff_Linear(t *testing.T) {
	d := nop.ComputeDelayMs(nop.BackoffLinear, 100, 5000, 0)
	if d != 100 {
		t.Errorf("linear attempt 0: got %d want 100", d)
	}
	d = nop.ComputeDelayMs(nop.BackoffLinear, 100, 5000, 2)
	if d != 300 {
		t.Errorf("linear attempt 2: got %d want 300", d)
	}
}

func TestBackoff_Exponential(t *testing.T) {
	d0 := nop.ComputeDelayMs(nop.BackoffExponential, 100, 5000, 0)
	if d0 != 100 {
		t.Errorf("exp attempt 0: got %d", d0)
	}
	d1 := nop.ComputeDelayMs(nop.BackoffExponential, 100, 5000, 1)
	if d1 != 200 {
		t.Errorf("exp attempt 1: got %d", d1)
	}
	d3 := nop.ComputeDelayMs(nop.BackoffExponential, 100, 5000, 3)
	if d3 != 800 {
		t.Errorf("exp attempt 3: got %d", d3)
	}
}

func TestBackoff_Cap(t *testing.T) {
	d := nop.ComputeDelayMs(nop.BackoffExponential, 1000, 2000, 10)
	if d > 2000 {
		t.Errorf("expected cap at 2000, got %d", d)
	}
}

// ── TaskState ─────────────────────────────────────────────────────────────────

func TestTaskStateFromString_Valid(t *testing.T) {
	states := []string{"pending", "running", "completed", "failed", "cancelled"}
	for _, s := range states {
		ts, err := nop.TaskStateFromString(s)
		if err != nil {
			t.Errorf("unexpected error for %q: %v", s, err)
		}
		if string(ts) != s {
			t.Errorf("state value mismatch: got %q want %q", ts, s)
		}
	}
}

func TestTaskStateFromString_Invalid(t *testing.T) {
	_, err := nop.TaskStateFromString("unknown")
	if err == nil {
		t.Fatal("expected error for unknown state")
	}
}

func TestTaskState_IsTerminal(t *testing.T) {
	terminal := []nop.TaskState{nop.TaskStateCompleted, nop.TaskStateFailed, nop.TaskStateCancelled}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%q should be terminal", s)
		}
	}
	nonTerminal := []nop.TaskState{nop.TaskStatePending, nop.TaskStateRunning}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%q should not be terminal", s)
		}
	}
}

// ── NopTaskStatus ─────────────────────────────────────────────────────────────

func TestNopTaskStatus_Getters(t *testing.T) {
	raw := map[string]any{
		"task_id": "t-123",
		"state":   "running",
	}
	s := nop.NewNopTaskStatus(raw)
	if s.TaskID() != "t-123" {
		t.Errorf("TaskID: got %q", s.TaskID())
	}
	if s.State() != nop.TaskStateRunning {
		t.Errorf("State: got %q", s.State())
	}
	if s.IsTerminal() {
		t.Error("running should not be terminal")
	}
}

func TestNopTaskStatus_Terminal(t *testing.T) {
	s := nop.NewNopTaskStatus(map[string]any{"task_id": "t", "state": "completed"})
	if !s.IsTerminal() {
		t.Error("completed should be terminal")
	}
}

func TestNopTaskStatus_Error(t *testing.T) {
	s := nop.NewNopTaskStatus(map[string]any{
		"task_id":       "t",
		"state":         "failed",
		"error_code":    "NOP-TIMEOUT",
		"error_message": "timed out",
	})
	if s.ErrorCode() != "NOP-TIMEOUT" {
		t.Errorf("ErrorCode: got %q", s.ErrorCode())
	}
	if s.ErrorMessage() != "timed out" {
		t.Errorf("ErrorMessage: got %q", s.ErrorMessage())
	}
}

func TestNopTaskStatus_NodeResults(t *testing.T) {
	nr := map[string]any{"n1": map[string]any{"output": "hello"}}
	s := nop.NewNopTaskStatus(map[string]any{
		"task_id":      "t",
		"state":        "completed",
		"node_results": nr,
	})
	results := s.NodeResults()
	if results == nil {
		t.Fatal("expected node_results")
	}
	if _, ok := results["n1"]; !ok {
		t.Error("expected key n1 in node_results")
	}
}

func TestNopTaskStatus_String(t *testing.T) {
	s := nop.NewNopTaskStatus(map[string]any{"task_id": "t-1", "state": "pending"})
	str := s.String()
	if !strings.Contains(str, "t-1") || !strings.Contains(str, "pending") {
		t.Errorf("String() missing expected content: %q", str)
	}
}

// ── NOP Frames ────────────────────────────────────────────────────────────────

func TestTaskFrame_Roundtrip(t *testing.T) {
	timeout := uint64(5000)
	cb := "https://example.com/cb"
	pri := "high"
	depth := int64(2)
	f := &nop.TaskFrame{
		TaskID:      "task-1",
		DAG:         map[string]any{"nodes": []any{"n1"}},
		TimeoutMs:   &timeout,
		CallbackURL: &cb,
		Context:     map[string]any{"user": "u1"},
		Priority:    &pri,
		Depth:       &depth,
	}
	d := f.ToDict()
	f2 := nop.TaskFrameFromDict(d)
	if f2.TaskID != "task-1" {
		t.Errorf("TaskID mismatch")
	}
	if f2.TimeoutMs == nil || *f2.TimeoutMs != timeout {
		t.Errorf("TimeoutMs mismatch")
	}
	if f2.CallbackURL == nil || *f2.CallbackURL != cb {
		t.Errorf("CallbackURL mismatch")
	}
	if f2.Priority == nil || *f2.Priority != "high" {
		t.Errorf("Priority mismatch")
	}
	if f2.Depth == nil || *f2.Depth != 2 {
		t.Errorf("Depth mismatch")
	}
}

func TestDelegateFrame_Roundtrip(t *testing.T) {
	ik := "idem-key-123"
	f := &nop.DelegateFrame{
		TaskID:         "t1",
		SubtaskID:      "s1",
		Action:         "process",
		TargetNID:      "urn:nps:node:example.com:worker",
		Inputs:         map[string]any{"data": "value"},
		Config:         map[string]any{"timeout": float64(30)},
		IdempotencyKey: &ik,
	}
	d := f.ToDict()
	f2 := nop.DelegateFrameFromDict(d)
	if f2.TaskID != "t1" || f2.SubtaskID != "s1" {
		t.Errorf("ID mismatch")
	}
	if f2.Action != "process" {
		t.Errorf("Action mismatch")
	}
	if f2.TargetNID != "urn:nps:node:example.com:worker" {
		t.Errorf("TargetNID mismatch")
	}
	if f2.IdempotencyKey == nil || *f2.IdempotencyKey != ik {
		t.Errorf("IdempotencyKey mismatch")
	}
}

func TestSyncFrame_Roundtrip(t *testing.T) {
	timeout := uint64(10000)
	f := &nop.SyncFrame{
		TaskID:      "t1",
		SyncID:      "sync-1",
		SubtaskIDs:  []string{"s1", "s2", "s3"},
		MinRequired: 2,
		Aggregate:   "merge",
		TimeoutMs:   &timeout,
	}
	d := f.ToDict()
	f2 := nop.SyncFrameFromDict(d)
	if f2.SyncID != "sync-1" {
		t.Errorf("SyncID mismatch")
	}
	if len(f2.SubtaskIDs) != 3 {
		t.Errorf("SubtaskIDs length mismatch")
	}
	if f2.MinRequired != 2 {
		t.Errorf("MinRequired mismatch")
	}
	if f2.Aggregate != "merge" {
		t.Errorf("Aggregate mismatch")
	}
}

func TestSyncFrame_DefaultAggregate(t *testing.T) {
	d := map[string]any{
		"task_id":      "t1",
		"sync_id":      "s1",
		"subtask_ids":  []any{"a"},
		"min_required": float64(1),
	}
	f := nop.SyncFrameFromDict(d)
	if f.Aggregate != "merge" {
		t.Errorf("expected default aggregate 'merge', got %q", f.Aggregate)
	}
}

func TestAlignStreamFrame_Roundtrip(t *testing.T) {
	src := "urn:nps:node:example.com:worker"
	ws := uint64(5)
	errMap := map[string]any{"error_code": "NOP-TIMEOUT", "message": "timed out"}
	f := &nop.AlignStreamFrame{
		SyncID:     "sync-1",
		TaskID:     "t1",
		SubtaskID:  "s1",
		Seq:        3,
		IsFinal:    true,
		SourceNID:  &src,
		Result:     map[string]any{"ok": true},
		Error:      errMap,
		WindowSize: &ws,
	}
	d := f.ToDict()
	f2 := nop.AlignStreamFrameFromDict(d)
	if f2.SyncID != "sync-1" {
		t.Errorf("SyncID mismatch")
	}
	if f2.Seq != 3 {
		t.Errorf("Seq mismatch")
	}
	if !f2.IsFinal {
		t.Error("IsFinal should be true")
	}
	if f2.SourceNID == nil || *f2.SourceNID != src {
		t.Errorf("SourceNID mismatch")
	}
	if f2.ErrorCode() != "NOP-TIMEOUT" {
		t.Errorf("ErrorCode mismatch: %q", f2.ErrorCode())
	}
	if f2.ErrorMessage() != "timed out" {
		t.Errorf("ErrorMessage mismatch")
	}
	if f2.WindowSize == nil || *f2.WindowSize != 5 {
		t.Errorf("WindowSize mismatch")
	}
}

func TestAlignStreamFrame_NoError(t *testing.T) {
	f := &nop.AlignStreamFrame{SyncID: "s", TaskID: "t", SubtaskID: "st", Seq: 0}
	if f.ErrorCode() != "" || f.ErrorMessage() != "" {
		t.Error("error fields should be empty when Error is nil")
	}
}
