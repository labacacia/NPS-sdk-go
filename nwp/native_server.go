// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
	"context"
	"fmt"
	"io"

	"github.com/labacacia/NPS-sdk-go/core"
	"github.com/labacacia/NPS-sdk-go/ncp"
)

type NativeQueryHandler func(context.Context, *QueryFrame) (*ncp.CapsFrame, error)
type NativeActionHandler func(context.Context, *ActionFrame) (any, error)

// NwpNativeNodeServer serves NWP frames over an established native NCP stream.
//
// The caller owns TLS, preamble validation, and Hello negotiation. This type
// only reads complete NPS frames, dispatches QueryFrame/ActionFrame, and writes
// response frames.
type NwpNativeNodeServer struct {
	Codec         *core.NpsFrameCodec
	Tier          core.EncodingTier
	AnchorRef     string
	QueryHandler  NativeQueryHandler
	ActionHandler NativeActionHandler
}

func NewNwpNativeNodeServer() *NwpNativeNodeServer {
	return &NwpNativeNodeServer{
		Codec:     core.NewNpsFrameCodec(core.CreateFullRegistry()),
		Tier:      core.EncodingTierMsgPack,
		AnchorRef: "native:nwp",
	}
}

func (s *NwpNativeNodeServer) DispatchWire(ctx context.Context, wire []byte) ([]byte, error) {
	if s.Codec == nil {
		s.Codec = core.NewNpsFrameCodec(core.CreateFullRegistry())
	}
	if s.Tier != core.EncodingTierJSON && s.Tier != core.EncodingTierMsgPack {
		s.Tier = core.EncodingTierMsgPack
	}
	ft, dict, err := s.Codec.Decode(wire)
	var outType core.FrameType
	var out core.FrameDict
	if err != nil {
		outType, out = core.FrameTypeError, nativeError("NPS-SERVER-INTERNAL", "NWP-NATIVE-DECODE-FAILED", err.Error())
	} else {
		outType, out = s.Dispatch(ctx, ft, dict)
	}
	return s.Codec.Encode(outType, out, s.Tier, true)
}

func (s *NwpNativeNodeServer) Dispatch(ctx context.Context, ft core.FrameType, dict core.FrameDict) (core.FrameType, core.FrameDict) {
	switch ft {
	case core.FrameTypeQuery:
		if s.QueryHandler == nil {
			return core.FrameTypeError, nativeError("NPS-SERVER-INTERNAL", "NWP-NATIVE-DISPATCH-FAILED", "No native NWP query handler configured.")
		}
		caps, err := s.QueryHandler(ctx, QueryFrameFromDict(dict))
		if err != nil {
			return core.FrameTypeError, nativeError("NPS-SERVER-INTERNAL", "NWP-NATIVE-DISPATCH-FAILED", err.Error())
		}
		return core.FrameTypeCaps, caps.ToDict()
	case core.FrameTypeAction:
		if s.ActionHandler == nil {
			return core.FrameTypeError, nativeError("NPS-SERVER-INTERNAL", "NWP-NATIVE-DISPATCH-FAILED", "No native NWP action handler configured.")
		}
		result, err := s.ActionHandler(ctx, ActionFrameFromDict(dict))
		if err != nil {
			return core.FrameTypeError, nativeError("NPS-SERVER-INTERNAL", "NWP-NATIVE-DISPATCH-FAILED", err.Error())
		}
		data := []any{}
		if result != nil {
			data = append(data, result)
		}
		caps := ncp.NewCapsFrame(s.anchorRef(), data)
		tok := uint64(1)
		caps.TokenEst = &tok
		tokenizer := "native-estimate"
		caps.TokenizerUsed = &tokenizer
		return core.FrameTypeCaps, caps.ToDict()
	default:
		return core.FrameTypeError, nativeError(
			"NPS-CLIENT-BAD-FRAME",
			"NWP-NATIVE-FRAME-UNSUPPORTED",
			fmt.Sprintf("Native NWP server does not handle frame type 0x%02X.", ft),
		)
	}
}

func (s *NwpNativeNodeServer) Serve(ctx context.Context, rw io.ReadWriter) error {
	for {
		wire, err := readNativeWireFrame(rw)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		out, err := s.DispatchWire(ctx, wire)
		if err != nil {
			return err
		}
		if _, err := rw.Write(out); err != nil {
			return err
		}
	}
}

func (s *NwpNativeNodeServer) anchorRef() string {
	if s.AnchorRef == "" {
		return "native:nwp"
	}
	return s.AnchorRef
}

func readNativeWireFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	raw := append([]byte{}, header...)
	if header[1]&0x01 != 0 {
		rest := make([]byte, 4)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, err
		}
		raw = append(raw, rest...)
	}
	hdr, err := core.PeekHeader(raw)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, int(hdr.PayloadLength))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return append(raw, payload...), nil
}

func nativeError(status, code, message string) core.FrameDict {
	return core.FrameDict{
		"status":     status,
		"error":      code,
		"error_code": code,
		"message":    message,
	}
}
