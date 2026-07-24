// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labacacia/NPS-sdk-go/ncp"
)

// jsonRPCBridgeDispatcher is the base dispatcher for JSON-RPC 2.0 protocols
// transported over HTTP POST.
type jsonRPCBridgeDispatcher struct {
	client            *http.Client
	protocol          string
	defaultMethod     string
	responseAnchorRef string
}

func (d *jsonRPCBridgeDispatcher) Protocol() string { return d.protocol }

func (d *jsonRPCBridgeDispatcher) Dispatch(ctx context.Context, frame *BridgeActionFrame, target *BridgeTarget) (*ncp.CapsFrame, error) {
	if frame == nil || target == nil {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "frame and target are required.")
	}

	uri, err := bridgeParseHTTPEndpoint(target)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := bridgeTimeoutContext(ctx, frame.TimeoutMs)
	defer cancel()

	body := d.buildRequestBody(frame, target)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, uri.String(), bytes.NewReader(body))
	if err != nil {
		return nil, newBridgeDispatchErrorCause(BridgeErrUpstreamFailed, d.protocol+" JSON-RPC bridge request failed.", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applyBridgeHeaders(req, target)

	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, newBridgeDispatchError(BridgeErrUpstreamFailed, d.protocol+" JSON-RPC bridge request timed out.")
		}
		return nil, newBridgeDispatchErrorCause(BridgeErrUpstreamFailed, d.protocol+" JSON-RPC bridge request failed.", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyText := string(bodyBytes)

	record := buildJSONRPCResponseRecord(resp, bodyText)
	return newBridgeCapsFrame(d.responseAnchorRef, record, estimateTokenCost(bodyText)), nil
}

func (d *jsonRPCBridgeDispatcher) buildRequestBody(frame *BridgeActionFrame, target *BridgeTarget) []byte {
	envelope := map[string]json.RawMessage{
		"jsonrpc": json.RawMessage(`"2.0"`),
		"id":      d.requestID(frame, target),
		"method":  mustMarshalString(d.rpcMethod(frame, target)),
		"params":  d.rpcParams(frame, target),
	}
	out, _ := json.Marshal(envelope)
	return out
}

func (d *jsonRPCBridgeDispatcher) rpcMethod(frame *BridgeActionFrame, target *BridgeTarget) string {
	if m := bridgeTargetString(target, "rpc_method", ""); m != "" {
		return m
	}
	if m := bridgeTargetString(target, "method", ""); m != "" {
		return m
	}
	if obj, ok := frame.paramsObject(); ok {
		if raw, has := obj["rpc_method"]; has {
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return d.defaultMethod
}

func (d *jsonRPCBridgeDispatcher) requestID(frame *BridgeActionFrame, target *BridgeTarget) json.RawMessage {
	if raw, ok := bridgeTargetJSON(target, "id"); ok {
		return cloneRaw(raw)
	}
	if obj, ok := frame.paramsObject(); ok {
		if raw, has := obj["id"]; has {
			return cloneRaw(raw)
		}
	}
	id := frame.RequestID
	if id == "" {
		id = frame.IdempotencyKey
	}
	if id == "" {
		id = newBridgeRequestID()
	}
	return mustMarshalString(id)
}

func (d *jsonRPCBridgeDispatcher) rpcParams(frame *BridgeActionFrame, target *BridgeTarget) json.RawMessage {
	if raw, ok := bridgeTargetJSON(target, "rpc_params"); ok {
		return cloneRaw(raw)
	}
	if raw, ok := bridgeTargetJSON(target, "params"); ok {
		return cloneRaw(raw)
	}

	obj, ok := frame.paramsObject()
	if !ok {
		return json.RawMessage("{}")
	}

	for _, name := range []string{"rpc_params", "params", "body"} {
		if raw, has := obj[name]; has {
			return cloneRaw(raw)
		}
	}

	reserved := map[string]bool{
		"bridge_target": true,
		"rpc_method":    true,
		"method":        true,
		"id":            true,
	}
	filtered := map[string]json.RawMessage{}
	for name, raw := range obj {
		if reserved[name] {
			continue
		}
		filtered[name] = raw
	}
	out, _ := json.Marshal(filtered)
	return out
}

func buildJSONRPCResponseRecord(resp *http.Response, bodyText string) map[string]interface{} {
	contentType := resp.Header.Get("Content-Type")
	record := map[string]interface{}{
		"status_code":  resp.StatusCode,
		"success":      resp.StatusCode >= 200 && resp.StatusCode < 300,
		"content_type": nullableString(contentType),
		"headers":      flattenHeaders(resp.Header),
	}
	writeJSONRPCBody(record, bodyText, contentType)
	return record
}

func writeJSONRPCBody(record map[string]interface{}, bodyText, contentType string) {
	if strings.TrimSpace(bodyText) != "" && strings.Contains(strings.ToLower(contentType), "json") {
		if json.Valid([]byte(bodyText)) {
			record["jsonrpc_response"] = json.RawMessage(bodyText)
			var obj map[string]json.RawMessage
			if json.Unmarshal([]byte(bodyText), &obj) == nil {
				if result, has := obj["result"]; has {
					record["result"] = result
				}
				if errObj, has := obj["error"]; has {
					record["error"] = errObj
				}
			}
			return
		}
	}
	record["body_text"] = bodyText
}

func mustMarshalString(s string) json.RawMessage {
	out, _ := json.Marshal(s)
	return out
}

// newBridgeRequestID returns a 32-char hex id, mirroring Guid("N").
func newBridgeRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// MCPBridgeResponseAnchorRef is the anchor reference used for MCP bridge response records.
const MCPBridgeResponseAnchorRef = "nps://bridge/mcp-jsonrpc-response/v1"

// A2ABridgeResponseAnchorRef is the anchor reference used for A2A bridge response records.
const A2ABridgeResponseAnchorRef = "nps://bridge/a2a-jsonrpc-response/v1"

// MCPBridgeDispatcher is the built-in Bridge dispatcher for MCP JSON-RPC servers over HTTP POST.
type MCPBridgeDispatcher struct{ jsonRPCBridgeDispatcher }

// NewMCPBridgeDispatcher creates an MCP bridge dispatcher over an http.Client.
func NewMCPBridgeDispatcher(client *http.Client) *MCPBridgeDispatcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &MCPBridgeDispatcher{jsonRPCBridgeDispatcher{
		client:            client,
		protocol:          BridgeProtocolMCP,
		defaultMethod:     "tools/call",
		responseAnchorRef: MCPBridgeResponseAnchorRef,
	}}
}

// A2ABridgeDispatcher is the built-in Bridge dispatcher for A2A JSON-RPC endpoints over HTTP POST.
type A2ABridgeDispatcher struct{ jsonRPCBridgeDispatcher }

// NewA2ABridgeDispatcher creates an A2A bridge dispatcher over an http.Client.
func NewA2ABridgeDispatcher(client *http.Client) *A2ABridgeDispatcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &A2ABridgeDispatcher{jsonRPCBridgeDispatcher{
		client:            client,
		protocol:          BridgeProtocolA2A,
		defaultMethod:     "tasks/send",
		responseAnchorRef: A2ABridgeResponseAnchorRef,
	}}
}
