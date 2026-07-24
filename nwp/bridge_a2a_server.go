// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// A2aServerProtocolVersion is the A2A protocol version implemented by the
// Bridge server adapter.
const A2aServerProtocolVersion = "0.2"

// A2A task states.
const (
	A2aTaskStateCompleted = "completed"
	A2aTaskStateFailed    = "failed"
)

// ── A2A server wire types ─────────────────────────────────────────────────────

type A2aAgentCard struct {
	Name               string                  `json:"name"`
	Description        string                  `json:"description,omitempty"`
	URL                string                  `json:"url"`
	Provider           *A2aAgentProvider       `json:"provider,omitempty"`
	Version            string                  `json:"version"`
	Capabilities       A2aAgentCapabilities    `json:"capabilities"`
	Authentication     *A2aAgentAuthentication `json:"authentication,omitempty"`
	DefaultInputModes  []string                `json:"defaultInputModes"`
	DefaultOutputModes []string                `json:"defaultOutputModes"`
	Skills             []A2aAgentSkill         `json:"skills"`
}

type A2aAgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

type A2aAgentCapabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type A2aAgentAuthentication struct {
	Schemes     []string `json:"schemes"`
	Credentials string   `json:"credentials,omitempty"`
}

type A2aAgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

type A2aTask struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId,omitempty"`
	Status    A2aTaskStatus   `json:"status"`
	Artifacts []A2aArtifact   `json:"artifacts,omitempty"`
	History   []A2aMessage    `json:"history,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type A2aTaskStatus struct {
	State     string      `json:"state"`
	Message   *A2aMessage `json:"message,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
}

type A2aMessage struct {
	Role     string          `json:"role"`
	Parts    []A2aPart       `json:"parts"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type A2aPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type A2aArtifact struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parts       []A2aPart       `json:"parts"`
	Index       int             `json:"index"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type A2aSendTaskParams struct {
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId,omitempty"`
	Message   A2aMessage      `json:"message"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// A2aServerBridge is the inbound A2A adapter that exposes local NPS actions as
// A2A skills.
type A2aServerBridge struct {
	options *BridgeServerOptions
}

// NewA2aServerBridge creates an A2A server bridge.
func NewA2aServerBridge(options *BridgeServerOptions) *A2aServerBridge {
	return &A2aServerBridge{options: options}
}

// BuildAgentCard builds the A2A AgentCard for the hosted Bridge server.
func (b *A2aServerBridge) BuildAgentCard(endpointURL string) A2aAgentCard {
	card := A2aAgentCard{
		Name:        b.options.ServerName,
		Description: b.options.Description,
		URL:         endpointURL,
		Provider: &A2aAgentProvider{
			Organization: "LabAcacia / INNO LOTUS PTY LTD",
			URL:          "https://github.com/labacacia/nps",
		},
		Version: b.options.ServerVersion,
		Capabilities: A2aAgentCapabilities{
			Streaming:              false,
			PushNotifications:      false,
			StateTransitionHistory: false,
		},
		DefaultInputModes:  []string{"text", "data"},
		DefaultOutputModes: []string{"text", "data"},
	}
	if b.options.RequireAuth {
		card.Authentication = &A2aAgentAuthentication{
			Schemes:     []string{"apikey"},
			Credentials: "X-NWP-Agent",
		}
	}
	skills := make([]A2aAgentSkill, 0, len(b.options.Actions))
	for i := range b.options.Actions {
		action := &b.options.Actions[i]
		skills = append(skills, A2aAgentSkill{
			ID:          action.ActionID,
			Name:        action.EffectiveDisplayName(),
			Description: action.Description,
			Tags:        action.Tags,
			InputModes:  []string{"text", "data"},
			OutputModes: []string{"data"},
		})
	}
	card.Skills = skills
	return card
}

// Dispatch dispatches one A2A JSON-RPC request.
func (b *A2aServerBridge) Dispatch(ctx context.Context, req *BridgeJSONRPCRequest) *BridgeJSONRPCResponse {
	if req == nil {
		return bridgeJSONRPCError(nil, JSONRPCInvalidRequest, "JSON-RPC request is required.", nil)
	}
	switch req.Method {
	case "tasks/send":
		return b.sendTask(ctx, req)
	default:
		return bridgeJSONRPCErrorFor(req, JSONRPCMethodNotFound,
			"A2A method '"+req.Method+"' is not supported by NWP Bridge server.", nil)
	}
}

func (b *A2aServerBridge) sendTask(ctx context.Context, req *BridgeJSONRPCRequest) *BridgeJSONRPCResponse {
	if len(req.Params) == 0 {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams, "A2A tasks/send requires params.", nil)
	}

	var task A2aSendTaskParams
	if err := json.Unmarshal(req.Params, &task); err != nil {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams, err.Error(), nil)
	}
	if strings.TrimSpace(task.ID) == "" {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams, "A2A tasks/send params.id is required.", nil)
	}

	action := b.resolveAction(&task)
	if action == nil {
		return bridgeJSONRPCErrorFor(req, JSONRPCInvalidParams,
			"A2A task metadata must identify an exposed NPS action when multiple actions exist.",
			map[string]interface{}{"error": BridgeErrServerToolNotFound})
	}

	frame := &BridgeActionFrame{
		ActionID:  action.ActionID,
		Params:    extractActionParams(&task),
		Async:     action.Async,
		RequestID: task.ID,
	}

	result, err := b.options.invokeAction(ctx, frame)
	if err != nil {
		result = &BridgeServerErrorFrame{
			Status:  "NPS-SERVER-ERROR",
			Error:   BridgeErrServerDispatchFailed,
			Message: err.Error(),
		}
	}
	return bridgeJSONRPCSuccess(req, toA2aTask(&task, result))
}

func (b *A2aServerBridge) resolveAction(task *A2aSendTaskParams) *BridgeServerAction {
	requested := firstNonEmpty(
		tryGetString(task.Metadata, "action_id", "actionId", "skill_id", "skillId", "skill"),
		tryGetString(task.Message.Metadata, "action_id", "actionId", "skill_id", "skillId", "skill"),
	)

	if strings.TrimSpace(requested) == "" {
		for _, part := range task.Message.Parts {
			requested = firstNonEmpty(
				tryGetString(part.Metadata, "action_id", "actionId", "skill_id", "skillId", "skill"),
				tryGetString(part.Data, "action_id", "actionId", "skill_id", "skillId", "skill"),
			)
			if strings.TrimSpace(requested) != "" {
				break
			}
		}
	}

	if strings.TrimSpace(requested) == "" && len(b.options.Actions) == 1 {
		return &b.options.Actions[0]
	}

	for i := range b.options.Actions {
		action := &b.options.Actions[i]
		if strings.EqualFold(action.ActionID, requested) ||
			strings.EqualFold(action.EffectiveToolName(), requested) {
			return action
		}
	}
	return nil
}

func extractActionParams(task *A2aSendTaskParams) json.RawMessage {
	if v := tryGetElement(task.Metadata, "params", "arguments"); v != nil {
		return v
	}
	if v := tryGetElement(task.Message.Metadata, "params", "arguments"); v != nil {
		return v
	}

	for _, part := range task.Message.Parts {
		if nested := tryGetElement(part.Data, "params", "arguments"); nested != nil {
			return nested
		}
		if strings.EqualFold(part.Type, "data") && len(part.Data) > 0 {
			return cloneRaw(part.Data)
		}
		if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
			out, _ := json.Marshal(map[string]string{"text": part.Text})
			return out
		}
	}
	return nil
}

func toA2aTask(request *A2aSendTaskParams, frame bridgeServerFrame) A2aTask {
	isError := frame.bridgeIsError()
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	payload := frame.bridgeFrameJSON()

	status := A2aTaskStatus{
		State:     a2aState(isError),
		Timestamp: timestamp,
	}
	if isError {
		status.Message = &A2aMessage{
			Role: "agent",
			Parts: []A2aPart{
				{Type: "text", Text: a2aErrorText(frame)},
			},
		}
	}

	artifactName := "nps-result"
	if isError {
		artifactName = "nps-error"
	}

	return A2aTask{
		ID:        request.ID,
		SessionID: request.SessionID,
		Status:    status,
		Artifacts: []A2aArtifact{
			{
				Name:  artifactName,
				Parts: []A2aPart{{Type: "data", Data: payload}},
				Index: 0,
			},
		},
		History: []A2aMessage{request.Message},
	}
}

func a2aState(isError bool) string {
	if isError {
		return A2aTaskStateFailed
	}
	return A2aTaskStateCompleted
}

func a2aErrorText(frame bridgeServerFrame) string {
	if ef, ok := frame.(*BridgeServerErrorFrame); ok {
		if ef.Message != "" {
			return ef.Message
		}
		return ef.Error
	}
	return "NPS action failed."
}

func tryGetString(source json.RawMessage, names ...string) string {
	v := tryGetElement(source, names...)
	if v == nil {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return ""
}

func tryGetElement(source json.RawMessage, names ...string) json.RawMessage {
	if len(source) == 0 {
		return nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(source, &obj) != nil || obj == nil {
		return nil
	}
	for _, name := range names {
		if v, ok := obj[name]; ok {
			return v
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
