// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"io"
	"net"

	"github.com/labacacia/NPS-sdk-go/core"
)

// Framer is the minimal codec surface an NcpSession needs to send and receive
// frames. *core.NpsFrameCodec satisfies it.
type Framer interface {
	Encode(ft core.FrameType, dict core.FrameDict, tier core.EncodingTier, isFinal bool) ([]byte, error)
	Decode(wire []byte) (core.FrameType, core.FrameDict, error)
}

// Frame is any NCP frame that can be serialised to its dict representation.
type Frame interface {
	FrameType() core.FrameType
	ToDict() core.FrameDict
}

// NcpSession is a live NCP native-mode session established after a successful
// handshake. It wraps the underlying TCP connection and exposes the negotiated
// parameters. Upper-layer protocols (NWP, NIP, …) obtain the raw stream via Conn.
type NcpSession struct {
	conn   net.Conn
	codec  Framer
	closed bool

	// ServerCaps holds the capabilities the peer advertised during the handshake.
	ServerCaps *NcpHandshakeCapsFrame
	// EncodingPolicy is the encoding policy negotiated during the handshake.
	EncodingPolicy NcpEncodingPolicy
}

func newNcpSession(conn net.Conn, caps *NcpHandshakeCapsFrame, policy NcpEncodingPolicy) *NcpSession {
	return &NcpSession{
		conn:           conn,
		ServerCaps:     caps,
		EncodingPolicy: policy,
	}
}

// SetCodec installs a codec for Send/Receive frame exchange. A session created by
// the client or server handshake has no codec until one is set; sessions used
// only for raw stream access do not require one.
func (s *NcpSession) SetCodec(codec Framer) { s.codec = codec }

// NegotiatedTier reports the stable default encoding tier negotiated during the
// handshake.
func (s *NcpSession) NegotiatedTier() core.EncodingTier { return s.EncodingPolicy.DefaultTier }

// Conn returns the underlying transport connection for upper-layer protocol use.
// The connection is owned by this session — do not close it directly.
func (s *NcpSession) Conn() net.Conn { return s.conn }

// Send encodes frame at the given tier and writes it to the connection. The tier
// must be permitted by the negotiated encoding policy.
func (s *NcpSession) Send(frame Frame, tier core.EncodingTier, isFinal bool) error {
	if err := s.EncodingPolicy.EnsureAllows(
		core.NewFrameHeader(frame.FrameType(), tier, isFinal, 0),
	); err != nil {
		return err
	}
	wire, err := s.codec.Encode(frame.FrameType(), frame.ToDict(), tier, isFinal)
	if err != nil {
		return err
	}
	_, err = s.conn.Write(wire)
	return err
}

// SendDefault encodes frame using the negotiated default tier.
func (s *NcpSession) SendDefault(frame Frame, isFinal bool) error {
	return s.Send(frame, s.EncodingPolicy.DefaultTier, isFinal)
}

// Receive reads the next frame from the connection, enforcing the negotiated
// encoding policy, and returns its type and decoded dict.
func (s *NcpSession) Receive() (core.FrameType, core.FrameDict, error) {
	header, err := ReadFrameHeader(s.conn)
	if err != nil {
		return 0, nil, err
	}
	if err := s.EncodingPolicy.EnsureAllows(header); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, header.PayloadLength)
	if _, err := io.ReadFull(s.conn, payload); err != nil {
		return 0, nil, err
	}
	wire := append(header.ToBytes(), payload...)
	return s.codec.Decode(wire)
}

// IsConnected reports whether the session has not yet been closed.
func (s *NcpSession) IsConnected() bool { return !s.closed }

// Close closes the underlying connection.
func (s *NcpSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.conn.Close()
}
