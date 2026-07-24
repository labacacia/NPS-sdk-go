// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// bridgeServerFrame is any frame an inbound Bridge server dispatcher may return:
// a *ncp.CapsFrame on success or a *BridgeServerErrorFrame on failure.
type bridgeServerFrame interface {
	bridgeFrameJSON() json.RawMessage
	bridgeIsError() bool
}

// BridgeServerErrorFrame is the inbound Bridge server error frame. Its wire
// shape (status/error/message) matches the .NET ErrorFrame consumed by the
// MCP/A2A server adapters.
type BridgeServerErrorFrame struct {
	Status  string `json:"status"`
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func (f *BridgeServerErrorFrame) bridgeFrameJSON() json.RawMessage {
	out, _ := json.Marshal(f)
	return out
}

func (f *BridgeServerErrorFrame) bridgeIsError() bool { return true }

// bridgeCapsResult wraps a *ncp.CapsFrame as a bridgeServerFrame.
type bridgeCapsResult struct{ raw json.RawMessage }

func (r bridgeCapsResult) bridgeFrameJSON() json.RawMessage { return r.raw }
func (r bridgeCapsResult) bridgeIsError() bool              { return false }

// BridgeServerAgentVerifier is an optional per-request verifier for inbound
// Bridge server callers.
type BridgeServerAgentVerifier func(agentNid string, r *http.Request) bool

// BridgeServerActionDispatcher is the local NPS action dispatcher used by
// inbound Bridge server adapters. It returns a frame result: on success a
// *ncp.CapsFrame, otherwise an error.
type BridgeServerActionDispatcher func(ctx context.Context, frame *BridgeActionFrame) (bridgeServerFrame, error)

// BridgeServerAction is an action exposed by inbound MCP/A2A Bridge server adapters.
type BridgeServerAction struct {
	// ActionID is the NPS action identifier dispatched to the local node.
	ActionID string
	// ToolName is the protocol-safe MCP tool name. Defaults to a sanitized ActionID.
	ToolName string
	// DisplayName is a human-readable display name for A2A AgentCard entries.
	DisplayName string
	// Description is a short action/tool description.
	Description string
	// InputSchema is a JSON Schema describing input arguments.
	InputSchema json.RawMessage
	// Async requests async execution on generated action frames.
	Async bool
	// Tags are optional A2A skill tags.
	Tags []string
}

// EffectiveToolName returns the effective MCP tool name for this action.
func (a *BridgeServerAction) EffectiveToolName() string {
	if strings.TrimSpace(a.ToolName) == "" {
		return BridgeActionToToolName(a.ActionID)
	}
	return a.ToolName
}

// EffectiveDisplayName returns the effective display name for A2A AgentCard skills.
func (a *BridgeServerAction) EffectiveDisplayName() string {
	if strings.TrimSpace(a.DisplayName) == "" {
		return a.ActionID
	}
	return a.DisplayName
}

// BridgeActionToToolName returns a protocol-safe MCP tool name for an NPS action id.
func BridgeActionToToolName(actionID string) string {
	if strings.TrimSpace(actionID) == "" {
		return "action"
	}
	var sb strings.Builder
	for _, ch := range strings.TrimSpace(actionID) {
		if isASCIILetterOrDigit(ch) || ch == '_' || ch == '-' {
			sb.WriteRune(ch)
		} else {
			sb.WriteRune('_')
		}
	}
	name := strings.Trim(sb.String(), "_")
	if strings.TrimSpace(name) == "" {
		return "action"
	}
	return name
}

func isASCIILetterOrDigit(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// BridgeServerOptions configures inbound MCP/A2A Bridge server hosting.
type BridgeServerOptions struct {
	// NodeID is the Bridge server identifier surfaced in protocol metadata.
	NodeID string
	// PathPrefix is the path prefix for inbound endpoints. Empty means root.
	PathPrefix string
	// McpPath is the MCP HTTP endpoint under PathPrefix.
	McpPath string
	// A2aPath is the A2A JSON-RPC endpoint under PathPrefix.
	A2aPath string
	// A2aAgentCardPath is the A2A AgentCard endpoint under PathPrefix.
	A2aAgentCardPath string
	// RequireAuth requires a valid X-NWP-Agent NID header before dispatching.
	RequireAuth bool
	// VerifyAgent binds X-NWP-Agent to policy; required when RequireAuth is true.
	VerifyAgent BridgeServerAgentVerifier
	// ServerName returned by MCP initialize and A2A AgentCard.
	ServerName string
	// ServerVersion returned by MCP initialize and A2A AgentCard.
	ServerVersion string
	// Description returned by A2A AgentCard.
	Description string
	// Actions are exposed as MCP tools and A2A skills.
	Actions []BridgeServerAction
	// Dispatch is the local NPS action dispatcher used by inbound adapters.
	Dispatch BridgeServerActionDispatcher
	// MaxRequestBodyBytes is the max inbound JSON-RPC body size. 0 disables the limit.
	MaxRequestBodyBytes int64
	// DispatchTimeoutMs is the max MCP/A2A dispatch time. 0 disables the timeout.
	DispatchTimeoutMs uint
}

// NewBridgeServerOptions returns options populated with the .NET defaults.
func NewBridgeServerOptions() *BridgeServerOptions {
	return &BridgeServerOptions{
		NodeID:              "nps-bridge-server",
		PathPrefix:          "",
		McpPath:             "/mcp",
		A2aPath:             "/a2a",
		A2aAgentCardPath:    "/.well-known/agent.json",
		RequireAuth:         true,
		ServerName:          "nps-bridge-server",
		ServerVersion:       "1.0.0-alpha.15",
		Description:         "NPS Bridge server ingress.",
		MaxRequestBodyBytes: 1 * 1024 * 1024,
		DispatchTimeoutMs:   30000,
	}
}

// AddAction adds an exposed local action and returns the options for chaining.
func (o *BridgeServerOptions) AddAction(action BridgeServerAction) *BridgeServerOptions {
	o.Actions = append(o.Actions, action)
	return o
}

// invokeAction invokes a local NPS action for inbound Bridge server adapters.
// It returns the ServerDispatcherMissing error frame when Dispatch is unset.
func (o *BridgeServerOptions) invokeAction(ctx context.Context, frame *BridgeActionFrame) (bridgeServerFrame, error) {
	if o.Dispatch == nil {
		return &BridgeServerErrorFrame{
			Status:  "NPS-SERVER-NOT-IMPLEMENTED",
			Error:   BridgeErrServerDispatcherMissing,
			Message: "BridgeServerOptions.Dispatch must be configured before handling inbound Bridge calls.",
		}, nil
	}
	return o.Dispatch(ctx, frame)
}

// CapsResult wraps a CapsFrame's JSON as a successful Bridge server frame result.
// Applications implementing BridgeServerActionDispatcher can use this to return
// a CapsFrame body.
func CapsResult(raw json.RawMessage) bridgeServerFrame { return bridgeCapsResult{raw: raw} }
