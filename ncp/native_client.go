// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/labacacia/NPS-sdk-go/core"
)

// ErrHandshakeUnexpectedFrame is the error code raised when the server responds
// with a frame other than Caps or Error during the native-mode handshake.
const ErrHandshakeUnexpectedFrame = "NCP-HANDSHAKE-UNEXPECTED-FRAME"

// NcpHandshakeError is returned when the server rejects the handshake (via an
// ErrorFrame) or sends an unexpected frame. Its ErrorCode/Message mirror the
// wire ErrorFrame for cross-SDK interop.
type NcpHandshakeError struct {
	ErrorCode string
	Message   string
}

func (e *NcpHandshakeError) Error() string {
	return fmt.Sprintf("NCP handshake failed [%s]: %s", e.ErrorCode, e.Message)
}

// NcpNativeClient performs the NCP native-mode 3-step handshake
// (preamble → HelloFrame → NcpHandshakeCapsFrame) per NPS-1 §4.6 and returns a
// live NcpSession.
type NcpNativeClient struct {
	codec *core.NpsFrameCodec
}

// NewNcpNativeClient constructs a client that uses codec to encode the outbound
// HelloFrame and decode the server response.
func NewNcpNativeClient(codec *core.NpsFrameCodec) *NcpNativeClient {
	return &NcpNativeClient{codec: codec}
}

// Connect dials host:port, performs the NCP native-mode handshake, and returns a
// live session. On any failure the underlying connection is closed.
func (c *NcpNativeClient) Connect(host string, port int, hello *HelloFrame) (*NcpSession, error) {
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	sess, err := c.Handshake(conn, hello)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return sess, nil
}

// Handshake performs the client-side handshake over an already-connected
// net.Conn. The caller retains ownership of conn on error; on success the
// returned session owns it. Exposed to allow custom transports (e.g. TLS).
func (c *NcpNativeClient) Handshake(conn net.Conn, hello *HelloFrame) (*NcpSession, error) {
	// 1 — preamble (Tier-1 JSON encoding not yet negotiated).
	if err := WritePreamble(conn); err != nil {
		return nil, err
	}

	// 2 — HelloFrame (always JSON per spec — encoding not yet agreed).
	helloWire, err := c.codec.Encode(hello.FrameType(), hello.ToDict(), core.EncodingTierJSON, true)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(helloWire); err != nil {
		return nil, err
	}

	// 3 — read server response header (handles EXT flag).
	header, err := ReadFrameHeader(conn)
	if err != nil {
		return nil, err
	}

	// 4 — read payload.
	payload := make([]byte, header.PayloadLength)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}

	// Reconstruct the full wire buffer so the codec's tier dispatch and registry
	// validation operate on canonical input.
	wire := append(header.ToBytes(), payload...)

	// 5 — ErrorFrame → return NcpHandshakeError.
	if header.FrameType == core.FrameTypeError {
		_, dict, derr := c.codec.Decode(wire)
		if derr != nil {
			return nil, derr
		}
		errFrame := ErrorFrameFromDict(dict)
		return nil, &NcpHandshakeError{ErrorCode: errFrame.ErrorCode, Message: errFrame.Message}
	}

	if header.FrameType != core.FrameTypeCaps {
		return nil, &NcpHandshakeError{
			ErrorCode: ErrHandshakeUnexpectedFrame,
			Message: fmt.Sprintf("Expected CapsFrame (0x%02X), got 0x%02X.",
				byte(core.FrameTypeCaps), byte(header.FrameType)),
		}
	}

	// 6 — decode NcpHandshakeCapsFrame using the negotiated tier the server
	// signalled as the stable default in the response header flags.
	negotiatedTier := header.EncodingTier()
	_, dict, err := c.codec.Decode(wire)
	if err != nil {
		return nil, err
	}
	caps := NcpHandshakeCapsFrameFromDict(dict)
	policy := NcpEncodingPolicyFromEnabledEncodings(negotiatedTier, caps.EnabledEncodings)

	return newNcpSession(conn, caps, policy), nil
}
