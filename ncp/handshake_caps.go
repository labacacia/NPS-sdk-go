// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import "github.com/labacacia/NPS-sdk-go/core"

// NcpHandshakeCapsFrame is the server's capability response to a HelloFrame in
// native mode (NPS-1 §4.6). It carries the server NID and its capability list
// and uses frame type 0x04 (Caps) on the wire.
//
// The response frame header determines the stable default encoding; optional
// payload fields echo the full enabled-encoding policy for extensions such as
// BinaryVector. This mirrors the .NET NcpHandshakeCapsFrame record exactly for
// cross-SDK interop.
type NcpHandshakeCapsFrame struct {
	// NodeID is the server's NID in urn:nps:agent:{domain}:{id} format.
	NodeID string
	// Caps lists protocols and capabilities the server supports, e.g. ["ncp","nwp","nip"].
	Caps []string
	// NegotiatedEncoding is the stable default encoding for ordinary session frames.
	NegotiatedEncoding *string
	// EnabledEncodings is every encoding enabled by the negotiated policy, including extensions.
	EnabledEncodings []string
	// AnchorRef is an optional anchor reference for the server's first offered schema.
	AnchorRef *string
	// Payload is optional additional metadata (implementation-defined).
	Payload any
}

func (f *NcpHandshakeCapsFrame) FrameType() core.FrameType { return core.FrameTypeCaps }

func (f *NcpHandshakeCapsFrame) ToDict() core.FrameDict {
	d := core.FrameDict{
		"node_id": f.NodeID,
		"caps":    f.Caps,
	}
	if f.NegotiatedEncoding != nil {
		d["negotiated_encoding"] = *f.NegotiatedEncoding
	}
	if f.EnabledEncodings != nil {
		d["enabled_encodings"] = f.EnabledEncodings
	}
	if f.AnchorRef != nil {
		d["anchor_ref"] = *f.AnchorRef
	}
	if f.Payload != nil {
		d["payload"] = f.Payload
	}
	return d
}

func NcpHandshakeCapsFrameFromDict(d core.FrameDict) *NcpHandshakeCapsFrame {
	asStringSlice := func(v any) []string {
		switch x := v.(type) {
		case []string:
			return x
		case []any:
			out := make([]string, 0, len(x))
			for _, e := range x {
				if s, ok := e.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
		return nil
	}
	f := &NcpHandshakeCapsFrame{
		NodeID:             str(d, "node_id"),
		Caps:               asStringSlice(d["caps"]),
		NegotiatedEncoding: optStr(d, "negotiated_encoding"),
		AnchorRef:          optStr(d, "anchor_ref"),
		Payload:            d["payload"],
	}
	if _, present := d["enabled_encodings"]; present {
		f.EnabledEncodings = asStringSlice(d["enabled_encodings"])
	}
	return f
}
