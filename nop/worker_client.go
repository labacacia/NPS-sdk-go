// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"context"
	"encoding/json"
)

// DelegateRequest is the typed payload the orchestrator dispatches to a Worker
// Agent for a single DAG node (mirror of .NET DelegateFrame, orchestration path).
type DelegateRequest struct {
	ParentTaskID   string
	SubtaskID      string
	NodeID         string
	TargetAgentNID string
	Action         string
	Params         json.RawMessage
	DelegatedScope json.RawMessage
	DeadlineAt     string
	IdempotencyKey string
	Priority       string
	Context        *TaskContext
	DelegateDepth  int
}

// WorkerStreamFrame is a single frame streamed back from a Worker Agent in
// response to a delegation (mirror of .NET AlignStreamFrame, orchestration path).
type WorkerStreamFrame struct {
	StreamID  string
	TaskID    string
	SubtaskID string
	Seq       uint64
	Data      json.RawMessage
	IsFinal   bool
	SenderNID string
	Error     *StreamError
}

// PreflightResult is a Worker Agent's response to a preflight probe (NPS-5 §4.3).
type PreflightResult struct {
	AgentNID          string
	Available         bool
	AvailableCGN      *int64
	EstimatedQueueMs  *int
	Capabilities      []string
	UnavailableReason string
}

// WorkerClient dispatches DelegateRequests to Worker Agents and receives streamed
// results (NPS-5 §3.2, §3.4). Implement this to connect the orchestrator to real
// agents (HTTP/NWP, in-process, or mock in tests).
type WorkerClient interface {
	// Delegate dispatches a request to the target Worker Agent and returns a
	// channel of streamed frames. The final frame has IsFinal == true. The channel
	// MUST be closed when the stream ends. Implementations should honour ctx.
	Delegate(ctx context.Context, req *DelegateRequest) (<-chan *WorkerStreamFrame, error)

	// Preflight sends a lightweight probe to agentNID to confirm resource
	// availability before committing to full execution (NPS-5 §4).
	Preflight(ctx context.Context, agentNID, action string) (*PreflightResult, error)
}
