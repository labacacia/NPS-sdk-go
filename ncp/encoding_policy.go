// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"fmt"
	"strings"

	"github.com/labacacia/NPS-sdk-go/core"
)

// EncodingToken maps an EncodingTier to its wire token string, matching the
// .NET NcpEncodingPolicy.EncodingToken switch.
func EncodingToken(tier core.EncodingTier) string {
	switch tier {
	case core.EncodingTierJSON:
		return "json"
	case core.EncodingTierMsgPack:
		return "msgpack"
	case core.EncodingTierBinaryVector:
		return "binary_vector.v1"
	default:
		return fmt.Sprintf("unknown:%d", byte(tier))
	}
}

// NcpEncodingPolicy is the encoding policy negotiated for an established NCP
// native-mode session. The default tier is stable for ordinary frames; Tier-3
// BinaryVector is an optional extension for frame classes that explicitly bind
// to it (currently the Query frame).
type NcpEncodingPolicy struct {
	DefaultTier         core.EncodingTier
	BinaryVectorEnabled bool
}

// NewNcpEncodingPolicy constructs a policy with the given stable default tier.
func NewNcpEncodingPolicy(defaultTier core.EncodingTier, binaryVectorEnabled bool) NcpEncodingPolicy {
	return NcpEncodingPolicy{DefaultTier: defaultTier, BinaryVectorEnabled: binaryVectorEnabled}
}

// EnabledEncodings returns the tokens of every encoding this policy enables.
func (p NcpEncodingPolicy) EnabledEncodings() []string {
	if p.BinaryVectorEnabled {
		return []string{EncodingToken(p.DefaultTier), "binary_vector.v1"}
	}
	return []string{EncodingToken(p.DefaultTier)}
}

// Allows reports whether a frame of frameType may use the given encoding tier
// under this policy.
func (p NcpEncodingPolicy) Allows(tier core.EncodingTier, frameType core.FrameType) bool {
	if tier == p.DefaultTier {
		return true
	}
	return tier == core.EncodingTierBinaryVector && p.BinaryVectorEnabled && isBinaryVectorFrame(frameType)
}

// EnsureAllows returns an error if the header's encoding is not permitted by the
// negotiated policy.
func (p NcpEncodingPolicy) EnsureAllows(header core.FrameHeader) error {
	if p.Allows(header.EncodingTier(), header.FrameType) {
		return nil
	}
	return &core.ErrCodec{Msg: fmt.Sprintf(
		"frame type 0x%02X used %s, but the negotiated session policy allows %s",
		byte(header.FrameType),
		EncodingToken(header.EncodingTier()),
		strings.Join(p.EnabledEncodings(), ", "),
	)}
}

// NcpEncodingPolicyFromEnabledEncodings builds a policy from a stable default
// tier and the list of enabled encoding tokens (as advertised in a CapsFrame).
func NcpEncodingPolicyFromEnabledEncodings(defaultTier core.EncodingTier, enabledEncodings []string) NcpEncodingPolicy {
	return NcpEncodingPolicy{
		DefaultTier:         defaultTier,
		BinaryVectorEnabled: containsToken(enabledEncodings, "binary_vector.v1"),
	}
}

func isBinaryVectorFrame(frameType core.FrameType) bool {
	return frameType == core.FrameTypeQuery
}

func containsToken(tokens []string, want string) bool {
	for _, t := range tokens {
		if t == want {
			return true
		}
	}
	return false
}
