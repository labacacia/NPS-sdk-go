// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import "fmt"

// ── BackoffStrategy ───────────────────────────────────────────────────────────

type BackoffStrategy int

const (
	BackoffFixed       BackoffStrategy = iota
	BackoffLinear
	BackoffExponential
)

// ComputeDelayMs returns the retry delay in milliseconds for the given attempt.
func ComputeDelayMs(strategy BackoffStrategy, baseMs, maxMs int64, attempt int) int64 {
	var raw int64
	switch strategy {
	case BackoffFixed:
		raw = baseMs
	case BackoffLinear:
		raw = baseMs * int64(attempt+1)
	case BackoffExponential:
		raw = baseMs << uint(attempt) // baseMs * 2^attempt
	}
	if raw > maxMs {
		return maxMs
	}
	return raw
}

// ── TaskState ─────────────────────────────────────────────────────────────────

type TaskState string

const (
	TaskStatePending   TaskState = "pending"
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCancelled TaskState = "cancelled"
)

func TaskStateFromString(s string) (TaskState, error) {
	switch TaskState(s) {
	case TaskStatePending, TaskStateRunning, TaskStateCompleted, TaskStateFailed, TaskStateCancelled:
		return TaskState(s), nil
	}
	return "", fmt.Errorf("unknown task state: %s", s)
}

func (s TaskState) IsTerminal() bool {
	return s == TaskStateCompleted || s == TaskStateFailed || s == TaskStateCancelled
}

// ── NopTaskStatus ─────────────────────────────────────────────────────────────

type NopTaskStatus struct {
	raw map[string]any
}

func NewNopTaskStatus(raw map[string]any) *NopTaskStatus {
	return &NopTaskStatus{raw: raw}
}

func (s *NopTaskStatus) TaskID() string {
	v, _ := s.raw["task_id"].(string)
	return v
}

func (s *NopTaskStatus) State() TaskState {
	v, _ := s.raw["state"].(string)
	return TaskState(v)
}

func (s *NopTaskStatus) IsTerminal() bool { return s.State().IsTerminal() }

func (s *NopTaskStatus) ErrorCode() string {
	v, _ := s.raw["error_code"].(string)
	return v
}

func (s *NopTaskStatus) ErrorMessage() string {
	v, _ := s.raw["error_message"].(string)
	return v
}

func (s *NopTaskStatus) NodeResults() map[string]any {
	v, _ := s.raw["node_results"].(map[string]any)
	return v
}

func (s *NopTaskStatus) String() string {
	return fmt.Sprintf("NopTaskStatus(task_id=%s, state=%s)", s.TaskID(), s.State())
}
