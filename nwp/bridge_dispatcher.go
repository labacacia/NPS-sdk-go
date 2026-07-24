// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/labacacia/NPS-sdk-go/ncp"
)

// BridgeDispatcher translates one NWP action invocation into a concrete
// non-NPS protocol call (NPS → external direction).
type BridgeDispatcher interface {
	// Protocol is the bridge protocol identifier served by this dispatcher.
	Protocol() string
	// Dispatch dispatches an action frame to the requested external target.
	Dispatch(ctx context.Context, frame *BridgeActionFrame, target *BridgeTarget) (*ncp.CapsFrame, error)
}

// BridgeDispatcherRegistry maps bridge protocol identifiers to dispatchers.
type BridgeDispatcherRegistry struct {
	dispatchers map[string]BridgeDispatcher
}

// NewBridgeDispatcherRegistry creates an empty dispatcher registry.
func NewBridgeDispatcherRegistry() *BridgeDispatcherRegistry {
	return &BridgeDispatcherRegistry{dispatchers: map[string]BridgeDispatcher{}}
}

// NewDefaultBridgeDispatcherRegistry creates a registry with all built-in
// dispatchers: HTTP/HTTPS, gRPC JSON, MCP JSON-RPC, and A2A JSON-RPC.
func NewDefaultBridgeDispatcherRegistry(client *http.Client) *BridgeDispatcherRegistry {
	r := NewBridgeDispatcherRegistry()
	r.Register(NewHTTPBridgeDispatcher(client))
	r.Register(NewGRPCBridgeDispatcher(client))
	r.Register(NewMCPBridgeDispatcher(client))
	r.Register(NewA2ABridgeDispatcher(client))
	return r
}

// Protocols returns the currently registered protocol identifiers, sorted.
func (r *BridgeDispatcherRegistry) Protocols() []string {
	out := make([]string, 0, len(r.dispatchers))
	for k := range r.dispatchers {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// Register registers or replaces the dispatcher for its protocol and returns
// the registry for chaining. It panics when the protocol is empty (matching
// the .NET ArgumentException contract).
func (r *BridgeDispatcherRegistry) Register(d BridgeDispatcher) *BridgeDispatcherRegistry {
	if d == nil {
		panic("bridge dispatcher must not be nil")
	}
	if strings.TrimSpace(d.Protocol()) == "" {
		panic("bridge dispatcher protocol must not be empty")
	}
	r.dispatchers[strings.ToLower(d.Protocol())] = d
	return r
}

// Resolve resolves a dispatcher for the given protocol.
func (r *BridgeDispatcherRegistry) Resolve(protocol string) (BridgeDispatcher, error) {
	if strings.TrimSpace(protocol) == "" {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "bridge_target.protocol is required.")
	}
	if d, ok := r.dispatchers[strings.ToLower(protocol)]; ok {
		return d, nil
	}
	return nil, newBridgeDispatchError(
		BridgeErrProtocolUnsupported,
		"Bridge protocol '"+protocol+"' is not registered.")
}

// BridgeNode is a stateless Bridge Node dispatcher facade. Host transports can
// feed decoded action frames here and write the returned CapsFrame.
type BridgeNode struct {
	registry *BridgeDispatcherRegistry
}

// NewBridgeNode creates a Bridge Node facade over a dispatcher registry.
func NewBridgeNode(registry *BridgeDispatcherRegistry) *BridgeNode {
	if registry == nil {
		panic("bridge dispatcher registry must not be nil")
	}
	return &BridgeNode{registry: registry}
}

// Dispatch parses bridge_target, resolves a protocol dispatcher, and invokes it.
func (n *BridgeNode) Dispatch(ctx context.Context, frame *BridgeActionFrame) (*ncp.CapsFrame, error) {
	if frame == nil {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "params.bridge_target is required.")
	}
	target, err := BridgeTargetFromActionFrame(frame)
	if err != nil {
		return nil, err
	}
	dispatcher, err := n.registry.Resolve(target.Protocol)
	if err != nil {
		return nil, err
	}
	return dispatcher.Dispatch(ctx, frame, target)
}
