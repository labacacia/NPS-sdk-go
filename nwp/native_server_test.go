// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp_test

import (
	"context"
	"testing"

	"github.com/labacacia/NPS-sdk-go/core"
	"github.com/labacacia/NPS-sdk-go/ncp"
	"github.com/labacacia/NPS-sdk-go/nwp"
)

func TestNativeServerDispatchWireQuery(t *testing.T) {
	codec := core.NewNpsFrameCodec(core.CreateFullRegistry())
	server := nwp.NewNwpNativeNodeServer()
	server.QueryHandler = func(_ context.Context, _ *nwp.QueryFrame) (*ncp.CapsFrame, error) {
		return ncp.NewCapsFrame("native:test", []any{map[string]any{"id": 42}}), nil
	}
	wire, err := codec.Encode(core.FrameTypeQuery, (&nwp.QueryFrame{AnchorRef: "sha256:a"}).ToDict(), core.EncodingTierMsgPack, true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := server.DispatchWire(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	ft, dict, err := codec.Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	if ft != core.FrameTypeCaps {
		t.Fatalf("expected CapsFrame, got 0x%02X", ft)
	}
	caps := ncp.CapsFrameFromDict(dict)
	if caps.Count != 1 {
		t.Fatalf("count = %d", caps.Count)
	}
}

func TestNativeServerDispatchWireAcceptsActionID(t *testing.T) {
	codec := core.NewNpsFrameCodec(core.CreateFullRegistry())
	server := nwp.NewNwpNativeNodeServer()
	server.ActionHandler = func(_ context.Context, frame *nwp.ActionFrame) (any, error) {
		return map[string]any{"action": frame.Action}, nil
	}
	wire, err := codec.Encode(core.FrameTypeAction, core.FrameDict{"action_id": "ping"}, core.EncodingTierMsgPack, true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := server.DispatchWire(context.Background(), wire)
	if err != nil {
		t.Fatal(err)
	}
	_, dict, err := codec.Decode(out)
	if err != nil {
		t.Fatal(err)
	}
	caps := ncp.CapsFrameFromDict(dict)
	if caps.Data[0].(map[string]any)["action"] != "ping" {
		t.Fatalf("unexpected caps data: %+v", caps.Data)
	}
}
