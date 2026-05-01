// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

// NWP error code wire constants — mirror of spec/error-codes.md NWP section.
const (
	// Auth.
	ErrAuthNidScopeViolation   = "NWP-AUTH-NID-SCOPE-VIOLATION"
	ErrAuthNidExpired          = "NWP-AUTH-NID-EXPIRED"
	ErrAuthNidRevoked          = "NWP-AUTH-NID-REVOKED"
	ErrAuthNidUntrustedIssuer  = "NWP-AUTH-NID-UNTRUSTED-ISSUER"
	ErrAuthNidCapabilityMissing = "NWP-AUTH-NID-CAPABILITY-MISSING"
	ErrAuthAssuranceTooLow     = "NWP-AUTH-ASSURANCE-TOO-LOW"
	ErrAuthReputationBlocked   = "NWP-AUTH-REPUTATION-BLOCKED"

	// Query.
	ErrQueryFilterInvalid       = "NWP-QUERY-FILTER-INVALID"
	ErrQueryFieldUnknown        = "NWP-QUERY-FIELD-UNKNOWN"
	ErrQueryCursorInvalid       = "NWP-QUERY-CURSOR-INVALID"
	ErrQueryRegexUnsafe         = "NWP-QUERY-REGEX-UNSAFE"
	ErrQueryVectorUnsupported   = "NWP-QUERY-VECTOR-UNSUPPORTED"
	ErrQueryAggregateUnsupported = "NWP-QUERY-AGGREGATE-UNSUPPORTED"
	ErrQueryAggregateInvalid    = "NWP-QUERY-AGGREGATE-INVALID"
	ErrQueryStreamUnsupported   = "NWP-QUERY-STREAM-UNSUPPORTED"

	// Action.
	ErrActionNotFound           = "NWP-ACTION-NOT-FOUND"
	ErrActionParamsInvalid      = "NWP-ACTION-PARAMS-INVALID"
	ErrActionIdempotencyConflict = "NWP-ACTION-IDEMPOTENCY-CONFLICT"

	// Task.
	ErrTaskNotFound        = "NWP-TASK-NOT-FOUND"
	ErrTaskAlreadyCancelled = "NWP-TASK-ALREADY-CANCELLED"
	ErrTaskAlreadyCompleted = "NWP-TASK-ALREADY-COMPLETED"
	ErrTaskAlreadyFailed   = "NWP-TASK-ALREADY-FAILED"

	// Subscribe.
	ErrSubscribeStreamNotFound  = "NWP-SUBSCRIBE-STREAM-NOT-FOUND"
	ErrSubscribeLimitExceeded   = "NWP-SUBSCRIBE-LIMIT-EXCEEDED"
	ErrSubscribeFilterUnsupported = "NWP-SUBSCRIBE-FILTER-UNSUPPORTED"
	ErrSubscribeInterrupted     = "NWP-SUBSCRIBE-INTERRUPTED"
	ErrSubscribeSeqTooOld       = "NWP-SUBSCRIBE-SEQ-TOO-OLD"

	// Infrastructure.
	ErrBudgetExceeded    = "NWP-BUDGET-EXCEEDED"
	ErrDepthExceeded     = "NWP-DEPTH-EXCEEDED"
	ErrGraphCycle        = "NWP-GRAPH-CYCLE"
	ErrNodeUnavailable   = "NWP-NODE-UNAVAILABLE"
	ErrRateLimitExceeded = "NWP-RATE-LIMIT-EXCEEDED"

	// Manifest.
	ErrManifestVersionUnsupported = "NWP-MANIFEST-VERSION-UNSUPPORTED"
	ErrManifestNodeTypeRemoved    = "NWP-MANIFEST-NODE-TYPE-REMOVED"
	ErrManifestNodeTypeUnknown    = "NWP-MANIFEST-NODE-TYPE-UNKNOWN"

	// Topology (alpha.4+).
	ErrTopologyUnauthorized      = "NWP-TOPOLOGY-UNAUTHORIZED"
	ErrTopologyUnsupportedScope  = "NWP-TOPOLOGY-UNSUPPORTED-SCOPE"
	ErrTopologyDepthUnsupported  = "NWP-TOPOLOGY-DEPTH-UNSUPPORTED"
	ErrTopologyFilterUnsupported = "NWP-TOPOLOGY-FILTER-UNSUPPORTED"

	// Reserved type (alpha.5+).
	ErrReservedTypeUnsupported = "NWP-RESERVED-TYPE-UNSUPPORTED"
)
