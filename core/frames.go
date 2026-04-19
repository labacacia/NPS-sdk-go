// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"encoding/binary"
	"fmt"
)

// ── FrameType ──────────────────────────────────────────────────────────────────

type FrameType uint8

const (
	FrameTypeAnchor      FrameType = 0x01
	FrameTypeDiff        FrameType = 0x02
	FrameTypeStream      FrameType = 0x03
	FrameTypeCaps        FrameType = 0x04
	FrameTypeHello       FrameType = 0x06
	FrameTypeQuery       FrameType = 0x10
	FrameTypeAction      FrameType = 0x11
	FrameTypeIdent       FrameType = 0x20
	FrameTypeTrust       FrameType = 0x21
	FrameTypeRevoke      FrameType = 0x22
	FrameTypeAnnounce    FrameType = 0x30
	FrameTypeResolve     FrameType = 0x31
	FrameTypeGraph       FrameType = 0x32
	FrameTypeTask        FrameType = 0x40
	FrameTypeDelegate    FrameType = 0x41
	FrameTypeSync        FrameType = 0x42
	FrameTypeAlignStream FrameType = 0x43
	FrameTypeError       FrameType = 0xFE
)

var knownFrameTypes = map[FrameType]bool{
	FrameTypeAnchor: true, FrameTypeDiff: true, FrameTypeStream: true,
	FrameTypeCaps: true, FrameTypeHello: true, FrameTypeQuery: true, FrameTypeAction: true,
	FrameTypeIdent: true, FrameTypeTrust: true, FrameTypeRevoke: true,
	FrameTypeAnnounce: true, FrameTypeResolve: true, FrameTypeGraph: true,
	FrameTypeTask: true, FrameTypeDelegate: true, FrameTypeSync: true,
	FrameTypeAlignStream: true, FrameTypeError: true,
}

func FrameTypeFromByte(b byte) (FrameType, error) {
	ft := FrameType(b)
	if knownFrameTypes[ft] {
		return ft, nil
	}
	return 0, fmt.Errorf("unknown frame type: 0x%02X", b)
}

// ── EncodingTier ───────────────────────────────────────────────────────────────

type EncodingTier uint8

const (
	EncodingTierJSON    EncodingTier = 0
	EncodingTierMsgPack EncodingTier = 1
)

// ── FrameHeader ────────────────────────────────────────────────────────────────

// FrameHeader represents the NPS wire-format frame header.
//
// Default (EXT=0): 4 bytes — [frame_type, flags, len_hi, len_lo]
// Extended (EXT=1): 8 bytes — [frame_type, flags, 0, 0, len_b3, len_b2, len_b1, len_b0]
//
// Flags:
//
//	bit 7 (0x80) — TIER: 0=JSON, 1=MsgPack
//	bit 6 (0x40) — FINAL: 1=last frame in stream
//	bit 0 (0x01) — EXT: 1=8-byte extended header
type FrameHeader struct {
	FrameType     FrameType
	Flags         uint8
	PayloadLength uint64
	IsExtended    bool
}

func NewFrameHeader(ft FrameType, tier EncodingTier, isFinal bool, payloadLen uint64) FrameHeader {
	isExt := payloadLen > 0xFFFF
	var flags uint8
	if tier == EncodingTierMsgPack {
		flags |= 0x80
	}
	if isFinal {
		flags |= 0x40
	}
	if isExt {
		flags |= 0x01
	}
	return FrameHeader{FrameType: ft, Flags: flags, PayloadLength: payloadLen, IsExtended: isExt}
}

func (h FrameHeader) EncodingTier() EncodingTier {
	if h.Flags&0x80 != 0 {
		return EncodingTierMsgPack
	}
	return EncodingTierJSON
}

func (h FrameHeader) IsFinal() bool { return h.Flags&0x40 != 0 }

func (h FrameHeader) HeaderSize() int {
	if h.IsExtended {
		return 8
	}
	return 4
}

func (h FrameHeader) ToBytes() []byte {
	if h.IsExtended {
		b := make([]byte, 8)
		b[0] = byte(h.FrameType)
		b[1] = h.Flags
		binary.BigEndian.PutUint32(b[4:], uint32(h.PayloadLength))
		return b
	}
	b := make([]byte, 4)
	b[0] = byte(h.FrameType)
	b[1] = h.Flags
	binary.BigEndian.PutUint16(b[2:], uint16(h.PayloadLength))
	return b
}

func ParseFrameHeader(wire []byte) (FrameHeader, error) {
	if len(wire) < 4 {
		return FrameHeader{}, fmt.Errorf("buffer too small for frame header")
	}
	ft, err := FrameTypeFromByte(wire[0])
	if err != nil {
		return FrameHeader{}, err
	}
	flags := wire[1]
	isExt := flags&0x01 != 0
	if isExt {
		if len(wire) < 8 {
			return FrameHeader{}, fmt.Errorf("buffer too small for extended header")
		}
		plen := uint64(binary.BigEndian.Uint32(wire[4:8]))
		return FrameHeader{FrameType: ft, Flags: flags, PayloadLength: plen, IsExtended: true}, nil
	}
	plen := uint64(binary.BigEndian.Uint16(wire[2:4]))
	return FrameHeader{FrameType: ft, Flags: flags, PayloadLength: plen, IsExtended: false}, nil
}
