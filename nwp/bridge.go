// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

// NodeTypeBridge is the NWM node_type wire value for Bridge Node (NPS-2 §2A.1).
const NodeTypeBridge = "bridge"

// Standard bridge_protocols wire-string constants (NPS-CR-0001 §3).
const (
	BridgeProtocolHTTP = "http"
	BridgeProtocolGRPC = "grpc"
	BridgeProtocolMCP  = "mcp"
	BridgeProtocolA2A  = "a2a"
)

// BridgeStandardProtocols is the full set of standard bridge_protocols at alpha.11.
var BridgeStandardProtocols = []string{BridgeProtocolHTTP, BridgeProtocolGRPC, BridgeProtocolMCP, BridgeProtocolA2A}

// BridgeNodeDescriptor declares which external protocols a Bridge Node can reach.
type BridgeNodeDescriptor struct {
	Nid                string
	SupportedProtocols []string
}

// BridgeTarget is the inbound parameter object for a bridge invocation.
type BridgeTarget struct {
	Protocol string                 `json:"protocol"`
	Endpoint string                 `json:"endpoint"`
	Extras   map[string]interface{} `json:"extras,omitempty"`
}
