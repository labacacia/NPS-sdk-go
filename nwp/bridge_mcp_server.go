// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"context"
	"encoding/json"
	"strings"
)

// McpServerProtocolVersion is the MCP protocol version implemented by the
// Bridge server adapter.
const McpServerProtocolVersion = "2024-11-05"

// ── MCP server wire types ─────────────────────────────────────────────────────

type mcpInitializeResult struct {
	ProtocolVersion string                `json:"protocolVersion"`
	ServerInfo      mcpServerInfo         `json:"serverInfo"`
	Capabilities    mcpServerCapabilities `json:"capabilities"`
}

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpServerCapabilities struct {
	Tools *mcpToolCapabilities `json:"tools,omitempty"`
}

type mcpToolCapabilities struct {
	ListChanged bool `json:"listChanged"`
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpToolListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type mcpToolCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// McpServerBridge is the inbound MCP adapter that exposes local NPS actions as
// MCP tools.
type McpServerBridge struct {
	options *BridgeServerOptions
}

// NewMcpServerBridge creates an MCP server bridge.
func NewMcpServerBridge(options *BridgeServerOptions) *McpServerBridge {
	return &McpServerBridge{options: options}
}

// Dispatch dispatches one MCP JSON-RPC request.
func (b *McpServerBridge) Dispatch(ctx context.Context, req *BridgeJSONRPCRequest) *BridgeJSONRPCResponse {
	if req == nil {
		return bridgeJSONRPCError(nil, JSONRPCInvalidRequest, "JSON-RPC request is required.", nil)
	}
	switch req.Method {
	case "initialize":
		return bridgeJSONRPCSuccess(req, b.initialize())
	case "tools/list":
		return bridgeJSONRPCSuccess(req, b.listTools())
	case "tools/call":
		return b.callTool(ctx, req)
	case "ping":
		return bridgeJSONRPCSuccess(req, struct{}{})
	default:
		return bridgeJSONRPCErrorFor(req, JSONRPCMethodNotFound,
			"MCP method '"+req.Method+"' is not supported by NWP Bridge server.", nil)
	}
}

func (b *McpServerBridge) initialize() mcpInitializeResult {
	return mcpInitializeResult{
		ProtocolVersion: McpServerProtocolVersion,
		ServerInfo: mcpServerInfo{
			Name:    b.options.ServerName,
			Version: b.options.ServerVersion,
		},
		Capabilities: mcpServerCapabilities{
			Tools: &mcpToolCapabilities{ListChanged: false},
		},
	}
}

func (b *McpServerBridge) listTools() mcpToolListResult {
	tools := make([]mcpTool, 0, len(b.options.Actions))
	for i := range b.options.Actions {
		action := &b.options.Actions[i]
		schema := action.InputSchema
		if len(schema) == 0 {
			schema = defaultInputSchema()
		}
		tools = append(tools, mcpTool{
			Name:        action.EffectiveToolName(),
			Description: action.Description,
			InputSchema: schema,
		})
	}
	return mcpToolListResult{Tools: tools}
}

func (b *McpServerBridge) callTool(ctx context.Context, req *BridgeJSONRPCRequest) *BridgeJSONRPCResponse {
	if len(req.Params) == 0 {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams, "MCP tools/call requires params.", nil)
	}

	var call mcpToolCallParams
	if err := json.Unmarshal(req.Params, &call); err != nil {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams, err.Error(), nil)
	}
	if strings.TrimSpace(call.Name) == "" {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams, "MCP tools/call params.name is required.", nil)
	}

	action := b.resolveAction(call.Name)
	if action == nil {
		return bridgeJSONRPCErrorFor(req, JSONRPCToolNotFound,
			"MCP tool '"+call.Name+"' is not exposed by NWP Bridge server.",
			map[string]interface{}{"error": BridgeErrServerToolNotFound, "tool": call.Name})
	}

	frame := &BridgeActionFrame{
		ActionID: action.ActionID,
		Params:   cloneRaw(call.Arguments),
		Async:    action.Async,
	}

	result, err := b.options.invokeAction(ctx, frame)
	if err != nil {
		result = &BridgeServerErrorFrame{
			Status:  "NPS-SERVER-ERROR",
			Error:   BridgeErrServerDispatchFailed,
			Message: err.Error(),
		}
	}
	return bridgeJSONRPCSuccess(req, toToolResult(result))
}

func (b *McpServerBridge) resolveAction(toolName string) *BridgeServerAction {
	for i := range b.options.Actions {
		action := &b.options.Actions[i]
		if strings.EqualFold(action.EffectiveToolName(), toolName) ||
			strings.EqualFold(action.ActionID, toolName) {
			return action
		}
	}
	return nil
}

func toToolResult(frame bridgeServerFrame) mcpToolCallResult {
	return mcpToolCallResult{
		IsError: frame.bridgeIsError(),
		Content: []mcpContent{
			{Type: "text", Text: string(frame.bridgeFrameJSON())},
		},
	}
}

func defaultInputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":true}`)
}
