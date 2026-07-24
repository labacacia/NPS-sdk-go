// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// NWP Bridge Node — stateless translator between NPS frames and non-NPS
// protocols (NPS-2 §2A, NPS-CR-0001). Faithful port of the .NET
// NPS.NWP.Bridge subsystem. Outbound dispatchers (HTTP/HTTPS, gRPC-JSON,
// MCP JSON-RPC, A2A JSON-RPC) all speak JSON over HTTP via net/http.

package nwp

import (
	"encoding/json"
	"fmt"
)

// BridgeErrorCodes — NWP error codes used by Bridge dispatchers.
const (
	// BridgeErrTargetInvalid: the invocation does not contain a valid bridge_target.
	BridgeErrTargetInvalid = "NWP-BRIDGE-TARGET-INVALID"
	// BridgeErrProtocolUnsupported: the requested bridge protocol has no dispatcher.
	BridgeErrProtocolUnsupported = "NWP-BRIDGE-PROTOCOL-UNSUPPORTED"
	// BridgeErrEndpointInvalid: the target endpoint is invalid or disallowed.
	BridgeErrEndpointInvalid = "NWP-BRIDGE-ENDPOINT-INVALID"
	// BridgeErrUpstreamFailed: the external call failed or returned an unusable response.
	BridgeErrUpstreamFailed = "NWP-BRIDGE-UPSTREAM-FAILED"
	// BridgeErrServerToolNotFound: an inbound request named a tool/action that is not exposed.
	BridgeErrServerToolNotFound = "NWP-BRIDGE-SERVER-TOOL-NOT-FOUND"
	// BridgeErrServerDispatcherMissing: an inbound server was not configured with a local dispatcher.
	BridgeErrServerDispatcherMissing = "NWP-BRIDGE-SERVER-DISPATCHER-MISSING"
	// BridgeErrServerDispatchFailed: an inbound server local action dispatch failed unexpectedly.
	BridgeErrServerDispatchFailed = "NWP-BRIDGE-SERVER-DISPATCH-FAILED"
)

// BridgeProtocols documentation constants (mirror of .NET BridgeProtocols).
// The wire-string protocol identifiers themselves live in bridge.go as
// BridgeProtocolHTTP / BridgeProtocolGRPC / BridgeProtocolMCP / BridgeProtocolA2A.

// BridgeBuiltInProtocols lists protocols with built-in dispatchers in this package.
var BridgeBuiltInProtocols = []string{BridgeProtocolHTTP, BridgeProtocolGRPC, BridgeProtocolMCP, BridgeProtocolA2A}

// BridgeDispatchError is raised when a Bridge Node cannot parse, route, or
// execute a bridge invocation. ErrorCode carries the NWP-compatible failure code.
type BridgeDispatchError struct {
	ErrorCode string
	Message   string
	Cause     error
}

func (e *BridgeDispatchError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.ErrorCode, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.ErrorCode, e.Message)
}

func (e *BridgeDispatchError) Unwrap() error { return e.Cause }

func newBridgeDispatchError(code, message string) *BridgeDispatchError {
	return &BridgeDispatchError{ErrorCode: code, Message: message}
}

func newBridgeDispatchErrorCause(code, message string, cause error) *BridgeDispatchError {
	return &BridgeDispatchError{ErrorCode: code, Message: message, Cause: cause}
}

// BridgeActionFrame is the inbound action invocation dispatched to an external
// target. It mirrors the fields of the .NET ActionFrame consumed by the Bridge
// dispatchers (params, timeout, request/idempotency ids, async flag).
type BridgeActionFrame struct {
	ActionID       string          `json:"action_id,omitempty"`
	Params         json.RawMessage `json:"params,omitempty"`
	TimeoutMs      uint            `json:"timeout_ms,omitempty"`
	Async          bool            `json:"async,omitempty"`
	RequestID      string          `json:"request_id,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// paramsObject returns the params as a decoded JSON object, or (nil,false) when
// params is absent or not a JSON object.
func (f *BridgeActionFrame) paramsObject() (map[string]json.RawMessage, bool) {
	if len(f.Params) == 0 {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(f.Params, &obj); err != nil || obj == nil {
		return nil, false
	}
	return obj, true
}
