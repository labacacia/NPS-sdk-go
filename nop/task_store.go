// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// OrchestratorTask is the typed task the orchestrator executes (mirror of the
// .NET NPS.NOP.Frames.TaskFrame, orchestration path).
type OrchestratorTask struct {
	TaskID             string
	Dag                *TaskDag
	TimeoutMs          uint64
	MaxRetries         uint32
	Priority           string
	CallbackURL        string
	CallbackSecret     string
	Preflight          bool
	Context            *TaskContext
	RequestID          string
	DelegateDepth      int
	CompensationPolicy string
}

// effectiveTimeoutMs returns TimeoutMs or the default when unset.
func (t *OrchestratorTask) effectiveTimeoutMs() uint64 {
	if t.TimeoutMs == 0 {
		return DefaultTimeoutMs
	}
	return t.TimeoutMs
}

func (t *OrchestratorTask) effectivePriority() string {
	if t.Priority == "" {
		return TaskPriorityNormal
	}
	return t.Priority
}

func (t *OrchestratorTask) effectiveCompensationPolicy() string {
	if t.CompensationPolicy == "" {
		return CompensationPolicyBestEffort
	}
	return t.CompensationPolicy
}

// NopSubtaskRecord holds the state and result for a single DAG node (subtask).
type NopSubtaskRecord struct {
	NodeID       string
	SubtaskID    string
	State        TaskState
	Result       json.RawMessage
	ErrorCode    string
	ErrorMessage string
	AttemptCount int
}

// NopTaskRecord is the persistent record of a running or completed NOP task.
type NopTaskRecord struct {
	TaskID      string
	Task        *OrchestratorTask
	State       TaskState
	StartedAt   time.Time
	CompletedAt *time.Time
	Subtasks    map[string]*NopSubtaskRecord

	mu sync.Mutex
}

// SagaCompensationResult summarises a Saga compensation run (NPS-5 §3.5).
type SagaCompensationResult struct {
	Attempted     int
	Succeeded     int
	Failed        int
	FailedNodeIDs []string
}

// NopTaskResult is the final result returned by Orchestrator.Execute (NPS-5 §5).
type NopTaskResult struct {
	TaskID           string
	FinalState       TaskState
	AggregatedResult json.RawMessage
	ErrorCode        string
	ErrorMessage     string
	NodeResults      map[string]json.RawMessage
	Compensation     *SagaCompensationResult
}

func newSuccessResult(taskID string, aggregated json.RawMessage, nodeResults map[string]json.RawMessage, comp *SagaCompensationResult) *NopTaskResult {
	return &NopTaskResult{
		TaskID:           taskID,
		FinalState:       TaskStateCompleted,
		AggregatedResult: aggregated,
		NodeResults:      nodeResults,
		Compensation:     comp,
	}
}

func newFailureResult(taskID, errorCode, errorMessage string, comp *SagaCompensationResult) *NopTaskResult {
	return &NopTaskResult{
		TaskID:       taskID,
		FinalState:   TaskStateFailed,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Compensation: comp,
	}
}

// TaskStore is the persistence abstraction for NOP task/subtask state (NPS-5 §5).
type TaskStore interface {
	// Save persists a new task record; returns an error if TaskID already exists.
	Save(ctx context.Context, record *NopTaskRecord) error
	// Get returns the task record or nil if not found.
	Get(ctx context.Context, taskID string) (*NopTaskRecord, error)
	// UpdateState updates the overall task state.
	UpdateState(ctx context.Context, taskID string, state TaskState) error
	// UpdateSubtask creates or updates a subtask record within the task.
	UpdateSubtask(ctx context.Context, upd SubtaskUpdate) error
}

// SubtaskUpdate carries the fields for a subtask create/update.
type SubtaskUpdate struct {
	TaskID       string
	NodeID       string
	SubtaskID    string
	State        TaskState
	Result       json.RawMessage
	ErrorCode    string
	ErrorMessage string
	Attempt      int
}

// InMemoryTaskStore is a volatile, in-memory TaskStore for tests and single-process use.
type InMemoryTaskStore struct {
	mu    sync.Mutex
	tasks map[string]*NopTaskRecord
}

// NewInMemoryTaskStore creates an empty in-memory task store.
func NewInMemoryTaskStore() *InMemoryTaskStore {
	return &InMemoryTaskStore{tasks: make(map[string]*NopTaskRecord)}
}

func (s *InMemoryTaskStore) Save(_ context.Context, record *NopTaskRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[record.TaskID]; exists {
		return fmt.Errorf("task already exists: %s", record.TaskID)
	}
	if record.Subtasks == nil {
		record.Subtasks = make(map[string]*NopSubtaskRecord)
	}
	s.tasks[record.TaskID] = record
	return nil
}

func (s *InMemoryTaskStore) Get(_ context.Context, taskID string) (*NopTaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[taskID], nil
}

func (s *InMemoryTaskStore) UpdateState(_ context.Context, taskID string, state TaskState) error {
	s.mu.Lock()
	rec := s.tasks[taskID]
	s.mu.Unlock()
	if rec != nil {
		rec.mu.Lock()
		rec.State = state
		rec.mu.Unlock()
	}
	return nil
}

func (s *InMemoryTaskStore) UpdateSubtask(_ context.Context, upd SubtaskUpdate) error {
	s.mu.Lock()
	rec := s.tasks[upd.TaskID]
	s.mu.Unlock()
	if rec == nil {
		return nil
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	sub, ok := rec.Subtasks[upd.NodeID]
	if !ok {
		sub = &NopSubtaskRecord{NodeID: upd.NodeID, SubtaskID: upd.SubtaskID}
		rec.Subtasks[upd.NodeID] = sub
	}
	sub.State = upd.State
	sub.AttemptCount = upd.Attempt
	if upd.Result != nil {
		sub.Result = upd.Result
	}
	if upd.ErrorCode != "" {
		sub.ErrorCode = upd.ErrorCode
	}
	if upd.ErrorMessage != "" {
		sub.ErrorMessage = upd.ErrorMessage
	}
	return nil
}
