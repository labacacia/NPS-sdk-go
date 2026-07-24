// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MappingError is returned when an input_mapping path cannot be resolved.
type MappingError struct {
	Message   string
	ErrorCode string
}

func (e *MappingError) Error() string { return e.Message }

func newMappingError(msg string) *MappingError {
	return &MappingError{Message: msg, ErrorCode: ErrInputMappingError}
}

// ResolvePath resolves a single NOP JSONPath expression of the form
// "$.node_id.field.sub" against a context of upstream node results (NPS-5 §3.1.3).
//
// Returns (nil, nil) when the path leads to a missing property. Returns an error
// for malformed paths or depth violations.
func ResolvePath(path string, context map[string]json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, newMappingError("Input mapping path must not be empty.")
	}
	if !strings.HasPrefix(path, "$.") {
		return nil, newMappingError(fmt.Sprintf("Input mapping path must start with '$.' — got: %s", path))
	}

	// Split into non-empty parts: "$", "node_id", "field", "sub", ...
	rawParts := strings.Split(path, ".")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		if p != "" {
			parts = append(parts, p)
		}
	}

	if len(parts) > MaxInputMappingDepth+1 {
		return nil, newMappingError(fmt.Sprintf(
			"Input mapping path depth %d exceeds maximum %d: %s", len(parts)-1, MaxInputMappingDepth, path))
	}

	if len(parts) == 1 {
		// Just "$" → serialize the entire context as a JSON object.
		out, err := json.Marshal(context)
		if err != nil {
			return nil, newMappingError(fmt.Sprintf("failed to serialize context: %v", err))
		}
		return json.RawMessage(out), nil
	}

	nodeID := parts[1]
	nodeResult, ok := context[nodeID]
	if !ok {
		return nil, nil
	}

	if len(parts) == 2 {
		return nodeResult, nil // "$.node_id" → full result
	}

	// Navigate deeper.
	current := nodeResult
	for i := 2; i < len(parts); i++ {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(current, &obj); err != nil {
			return nil, nil // not an object → missing
		}
		next, ok := obj[parts[i]]
		if !ok {
			return nil, nil
		}
		current = next
	}
	return current, nil
}

// BuildParams builds a DelegateFrame.params object by resolving all input_mapping
// entries against the upstream result context. Each entry value may be a string
// JSONPath, an array of JSONPaths, or a literal JSON value.
func BuildParams(inputMapping map[string]json.RawMessage, context map[string]json.RawMessage) (json.RawMessage, error) {
	if len(inputMapping) == 0 {
		return json.RawMessage("{}"), nil
	}

	result := make(map[string]json.RawMessage, len(inputMapping))
	for paramName, pathElement := range inputMapping {
		// Try string JSONPath.
		var s string
		if err := json.Unmarshal(pathElement, &s); err == nil {
			resolved, err := ResolvePath(s, context)
			if err != nil {
				return nil, err
			}
			result[paramName] = jsonOrNull(resolved)
			continue
		}

		// Try array of JSONPaths / literals.
		var arr []json.RawMessage
		if err := json.Unmarshal(pathElement, &arr); err == nil {
			list := make([]json.RawMessage, 0, len(arr))
			for _, p := range arr {
				var ps string
				if err := json.Unmarshal(p, &ps); err == nil {
					resolved, err := ResolvePath(ps, context)
					if err != nil {
						return nil, err
					}
					list = append(list, jsonOrNull(resolved))
				} else {
					list = append(list, p)
				}
			}
			encoded, _ := json.Marshal(list)
			result[paramName] = encoded
			continue
		}

		// Literal value.
		result[paramName] = pathElement
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, newMappingError(fmt.Sprintf("failed to encode params: %v", err))
	}
	return out, nil
}

func jsonOrNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}
