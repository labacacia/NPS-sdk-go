// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"fmt"

	"github.com/labacacia/NPS-sdk-go/core"
)

// DiffFrame.PatchFormat value constants (NPS-1 §4.2).
const (
	// PatchFormatJSONPatch is the default format: patch is an RFC 6902 JSON Patch
	// array. Compatible with all encoding tiers.
	PatchFormatJSONPatch = "json_patch"

	// PatchFormatBinaryBitset is the compact binary format: binary_patch contains
	// a changed-fields bitset followed by MsgPack-encoded new values. MUST only be
	// used in Tier-2 (MsgPack) frames.
	PatchFormatBinaryBitset = "binary_bitset"
)

// ValidatePatchFormat validates a patch-format token against the frame's
// encoding tier, enforcing the Tier-2-only rule for binary_bitset. It returns
// an error for unknown formats or a binary_bitset patch used outside MsgPack.
func ValidatePatchFormat(patchFormat string, tier core.EncodingTier) error {
	switch patchFormat {
	case PatchFormatJSONPatch:
		return nil
	case PatchFormatBinaryBitset:
		if tier != core.EncodingTierMsgPack {
			return &core.ErrFrame{Msg: fmt.Sprintf(
				"patch format %q MUST only be used in Tier-2 (MsgPack) frames, got %s",
				patchFormat, EncodingToken(tier),
			)}
		}
		return nil
	default:
		return &core.ErrFrame{Msg: fmt.Sprintf("unsupported patch format %q", patchFormat)}
	}
}
