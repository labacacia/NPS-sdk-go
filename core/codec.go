// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"encoding/json"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

const DefaultMaxPayload = 10 * 1024 * 1024 // 10 MiB

// FrameDict is the intermediate representation for all NPS frames.
type FrameDict = map[string]any

// NpsFrameCodec encodes and decodes NPS frames.
type NpsFrameCodec struct {
	Registry   *FrameRegistry
	MaxPayload int64
}

func NewNpsFrameCodec(reg *FrameRegistry) *NpsFrameCodec {
	return &NpsFrameCodec{Registry: reg, MaxPayload: DefaultMaxPayload}
}

// Encode serialises dict with the given tier, wraps in a frame header.
func (c *NpsFrameCodec) Encode(ft FrameType, dict FrameDict, tier EncodingTier, isFinal bool) ([]byte, error) {
	payload, err := encodePayload(dict, tier)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > c.MaxPayload {
		return nil, &ErrCodec{Msg: fmt.Sprintf("payload %d exceeds max %d", len(payload), c.MaxPayload)}
	}
	hdr := NewFrameHeader(ft, tier, isFinal, uint64(len(payload)))
	wire := append(hdr.ToBytes(), payload...)
	return wire, nil
}

// Decode parses wire bytes and returns the frame type and dict.
func (c *NpsFrameCodec) Decode(wire []byte) (FrameType, FrameDict, error) {
	hdr, err := ParseFrameHeader(wire)
	if err != nil {
		return 0, nil, err
	}
	if !c.Registry.IsRegistered(hdr.FrameType) {
		return 0, nil, &ErrFrame{Msg: fmt.Sprintf("unregistered frame type 0x%02X", hdr.FrameType)}
	}
	hdrLen := hdr.HeaderSize()
	pLen := int(hdr.PayloadLength)
	if len(wire) < hdrLen+pLen {
		return 0, nil, &ErrCodec{Msg: "wire too short for declared payload"}
	}
	dict, err := decodePayload(wire[hdrLen:hdrLen+pLen], hdr.EncodingTier())
	if err != nil {
		return 0, nil, err
	}
	return hdr.FrameType, dict, nil
}

// PeekHeader parses only the header, leaving payload untouched.
func PeekHeader(wire []byte) (FrameHeader, error) {
	return ParseFrameHeader(wire)
}

func encodePayload(dict FrameDict, tier EncodingTier) ([]byte, error) {
	switch tier {
	case EncodingTierJSON:
		return json.Marshal(dict)
	case EncodingTierMsgPack:
		return msgpack.Marshal(dict)
	default:
		return nil, &ErrCodec{Msg: fmt.Sprintf("unknown encoding tier %d", tier)}
	}
}

func decodePayload(payload []byte, tier EncodingTier) (FrameDict, error) {
	var dict FrameDict
	switch tier {
	case EncodingTierJSON:
		if err := json.Unmarshal(payload, &dict); err != nil {
			return nil, &ErrCodec{Msg: err.Error()}
		}
	case EncodingTierMsgPack:
		if err := msgpack.Unmarshal(payload, &dict); err != nil {
			return nil, &ErrCodec{Msg: err.Error()}
		}
	default:
		return nil, &ErrCodec{Msg: fmt.Sprintf("unknown encoding tier %d", tier)}
	}
	return dict, nil
}
