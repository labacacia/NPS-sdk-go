// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"encoding/json"
	"strings"
)

// BridgeTargetFromActionFrame parses params.bridge_target from an action frame.
// If params itself carries a "bridge_target" object, that nested object is used;
// otherwise params is treated as the bridge target object directly (matching .NET).
func BridgeTargetFromActionFrame(frame *BridgeActionFrame) (*BridgeTarget, error) {
	if frame == nil || len(frame.Params) == 0 {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "params.bridge_target is required.")
	}

	target := frame.Params
	if obj, ok := frame.paramsObject(); ok {
		if nested, has := obj["bridge_target"]; has {
			target = nested
		}
	}

	return BridgeTargetFromJSON(target)
}

// BridgeTargetFromJSON parses a bridge target JSON object.
func BridgeTargetFromJSON(raw json.RawMessage) (*BridgeTarget, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "bridge_target must be an object.")
	}

	protocol, err := readRequiredString(obj, "protocol")
	if err != nil {
		return nil, err
	}
	endpoint, err := readRequiredString(obj, "endpoint")
	if err != nil {
		return nil, err
	}

	extras := map[string]interface{}{}
	for name, value := range obj {
		if strings.EqualFold(name, "protocol") || strings.EqualFold(name, "endpoint") {
			continue
		}
		if strings.EqualFold(name, "extras") {
			var nested map[string]json.RawMessage
			if json.Unmarshal(value, &nested) == nil && nested != nil {
				for en, ev := range nested {
					extras[en] = json.RawMessage(append([]byte(nil), ev...))
				}
				continue
			}
		}
		extras[name] = json.RawMessage(append([]byte(nil), value...))
	}

	t := &BridgeTarget{Protocol: protocol, Endpoint: endpoint}
	if len(extras) > 0 {
		t.Extras = extras
	}
	return t, nil
}

// bridgeExtraRaw returns the raw JSON for a target extra, case-insensitively.
func bridgeExtraRaw(t *BridgeTarget, name string) (json.RawMessage, bool) {
	if t == nil || t.Extras == nil {
		return nil, false
	}
	for k, v := range t.Extras {
		if !strings.EqualFold(k, name) {
			continue
		}
		switch x := v.(type) {
		case json.RawMessage:
			return x, true
		case nil:
			return nil, false
		default:
			if b, err := json.Marshal(x); err == nil {
				return b, true
			}
		}
	}
	return nil, false
}

// bridgeTargetString reads a string extra from a target, returning def when absent.
func bridgeTargetString(t *BridgeTarget, name, def string) string {
	raw, ok := bridgeExtraRaw(t, name)
	if !ok {
		return def
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return def
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return strings.TrimSpace(string(raw))
	case nil:
		return def
	default:
		return strings.TrimSpace(string(raw))
	}
}

// bridgeTargetJSON tries to read a JSON extra from a target. The returned raw
// message is the extra's JSON text.
func bridgeTargetJSON(t *BridgeTarget, name string) (json.RawMessage, bool) {
	raw, ok := bridgeExtraRaw(t, name)
	if !ok || len(raw) == 0 {
		return nil, false
	}
	return raw, true
}

// bridgeTargetBool reads a boolean extra, honouring true/false and "true"/"false"
// string forms; returns def when absent or unparseable.
func bridgeTargetBool(t *BridgeTarget, name string, def bool) bool {
	raw, ok := bridgeTargetJSON(t, name)
	if !ok {
		return def
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true":
			return true
		case "false":
			return false
		}
	}
	return def
}

// bridgeTargetStringList reads a string or string-array extra.
func bridgeTargetStringList(t *BridgeTarget, name string) []string {
	raw, ok := bridgeTargetJSON(t, name)
	if !ok {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) != "" {
			return []string{x}
		}
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func readRequiredString(obj map[string]json.RawMessage, name string) (string, error) {
	raw, ok := obj[name]
	if !ok {
		return "", newBridgeDispatchError(BridgeErrTargetInvalid, "bridge_target."+name+" is required.")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || strings.TrimSpace(s) == "" {
		return "", newBridgeDispatchError(BridgeErrTargetInvalid, "bridge_target."+name+" is required.")
	}
	return s, nil
}
