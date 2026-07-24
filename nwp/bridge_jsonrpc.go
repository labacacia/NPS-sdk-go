// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import "encoding/json"

// BridgeJSONRPCRequest is the JSON-RPC 2.0 request envelope used by MCP and A2A
// Bridge servers.
type BridgeJSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// BridgeJSONRPCResponse is the JSON-RPC 2.0 response envelope used by MCP and
// A2A Bridge servers.
type BridgeJSONRPCResponse struct {
	JSONRPC string              `json:"jsonrpc"`
	ID      json.RawMessage     `json:"id,omitempty"`
	Result  json.RawMessage     `json:"result,omitempty"`
	Error   *BridgeJSONRPCError `json:"error,omitempty"`
}

// BridgeJSONRPCError is a JSON-RPC 2.0 error object.
type BridgeJSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC error codes plus Bridge server application codes.
const (
	JSONRPCParseError     = -32700
	JSONRPCInvalidRequest = -32600
	JSONRPCMethodNotFound = -32601
	JSONRPCInvalidParams  = -32602
	JSONRPCInternalError  = -32603
	JSONRPCUpstreamError  = -32000
	JSONRPCToolNotFound   = -32002
)

// bridgeJSONRPCSuccess builds a success response for a request.
func bridgeJSONRPCSuccess(req *BridgeJSONRPCRequest, result interface{}) *BridgeJSONRPCResponse {
	raw, _ := json.Marshal(result)
	return &BridgeJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      cloneRaw(reqID(req)),
		Result:  raw,
	}
}

// bridgeJSONRPCErrorFor builds an error response for a request.
func bridgeJSONRPCErrorFor(req *BridgeJSONRPCRequest, code int, message string, data interface{}) *BridgeJSONRPCResponse {
	return bridgeJSONRPCError(reqID(req), code, message, data)
}

// bridgeJSONRPCError builds an error response with an explicit id.
func bridgeJSONRPCError(id json.RawMessage, code int, message string, data interface{}) *BridgeJSONRPCResponse {
	var dataRaw json.RawMessage
	if data != nil {
		dataRaw, _ = json.Marshal(data)
	}
	return &BridgeJSONRPCResponse{
		JSONRPC: "2.0",
		ID:      cloneRaw(id),
		Error: &BridgeJSONRPCError{
			Code:    code,
			Message: message,
			Data:    dataRaw,
		},
	}
}

func reqID(req *BridgeJSONRPCRequest) json.RawMessage {
	if req == nil {
		return nil
	}
	return req.ID
}

func cloneRaw(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return nil
	}
	return json.RawMessage(append([]byte(nil), r...))
}
