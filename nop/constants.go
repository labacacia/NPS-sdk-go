// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

// Protocol-level limits defined by NPS-5 §8.2 (mirror of .NET NopConstants).
const (
	// MaxDagNodes is the maximum number of nodes in a single DAG.
	MaxDagNodes = 32

	// MaxDelegateChainDepth is the maximum delegation chain depth
	// (Orchestrator → Worker → Sub-Worker).
	MaxDelegateChainDepth = 3

	// MaxConditionLength is the maximum length of a CEL condition expression.
	MaxConditionLength = 512

	// MaxInputMappingDepth is the maximum JSONPath nesting depth in input_mapping values.
	MaxInputMappingDepth = 8

	// DefaultTimeoutMs is the default task timeout in milliseconds.
	DefaultTimeoutMs uint64 = 30000

	// MaxTimeoutMs is the maximum task timeout in milliseconds (1 hour).
	MaxTimeoutMs uint64 = 3600000

	// DefaultAnchorTTL is the default AnchorFrame TTL in seconds.
	DefaultAnchorTTL uint64 = 3600

	// CallbackMaxRetries is the maximum number of callback POST attempts with
	// exponential backoff (NPS-5 §8.4).
	CallbackMaxRetries = 3
)
