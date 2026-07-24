// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import "runtime"

// OrchestratorOptions configures a NopOrchestrator (mirror of .NET NopOrchestratorOptions).
type OrchestratorOptions struct {
	// MaxConcurrentNodes is the max DAG nodes that may execute concurrently per task.
	// Defaults to GOMAXPROCS × 2.
	MaxConcurrentNodes int
	// ValidateSenderNID validates each frame's SenderNID against the node's Agent NID.
	ValidateSenderNID bool
	// EnableCallback POSTs the result to callback_url on completion (fire-and-forget).
	EnableCallback bool
	// CallbackTimeoutMs is the HTTP client timeout for callback POSTs (ms).
	CallbackTimeoutMs int
	// CallbackRetryBaseDelayMs is the base delay for exponential backoff between
	// callback retries. Set to 0 in tests to avoid real delays.
	CallbackRetryBaseDelayMs int
	// DefaultAggregateStrategy is applied to end nodes when no SyncFrame is present.
	DefaultAggregateStrategy string
}

// DefaultOrchestratorOptions returns options matching the .NET defaults.
func DefaultOrchestratorOptions() *OrchestratorOptions {
	return &OrchestratorOptions{
		MaxConcurrentNodes:       runtime.GOMAXPROCS(0) * 2,
		ValidateSenderNID:        true,
		EnableCallback:           true,
		CallbackTimeoutMs:        10000,
		CallbackRetryBaseDelayMs: 1000,
		DefaultAggregateStrategy: AggregateStrategyMerge,
	}
}

func (o *OrchestratorOptions) normalized() *OrchestratorOptions {
	if o == nil {
		return DefaultOrchestratorOptions()
	}
	out := *o
	if out.MaxConcurrentNodes <= 0 {
		out.MaxConcurrentNodes = runtime.GOMAXPROCS(0) * 2
	}
	if out.DefaultAggregateStrategy == "" {
		out.DefaultAggregateStrategy = AggregateStrategyMerge
	}
	if out.CallbackTimeoutMs <= 0 {
		out.CallbackTimeoutMs = 10000
	}
	return &out
}
