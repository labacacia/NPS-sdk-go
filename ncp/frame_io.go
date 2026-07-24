// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"io"

	"github.com/labacacia/NPS-sdk-go/core"
)

// Header sizes on the wire.
const (
	frameHeaderDefaultSize  = 4
	frameHeaderExtendedSize = 8
)

// ReadFrameHeader reads a frame header from r, peeking the EXT flag to determine
// whether to read 4 or 8 bytes total. It mirrors the .NET
// NcpNativeClient.ReadFrameHeaderAsync helper: always read 2 bytes first to
// detect the EXT flag, then read the remaining header bytes.
func ReadFrameHeader(r io.Reader) (core.FrameHeader, error) {
	peek := make([]byte, 2)
	if _, err := io.ReadFull(r, peek); err != nil {
		return core.FrameHeader{}, err
	}

	// FrameFlags EXT bit = 0x80 (see core.FrameHeader flags layout).
	ext := peek[1]&0x80 != 0
	remaining := frameHeaderDefaultSize - 2
	if ext {
		remaining = frameHeaderExtendedSize - 2
	}

	rest := make([]byte, remaining)
	if _, err := io.ReadFull(r, rest); err != nil {
		return core.FrameHeader{}, err
	}

	raw := make([]byte, len(peek)+len(rest))
	copy(raw, peek)
	copy(raw[len(peek):], rest)

	return core.ParseFrameHeader(raw)
}
