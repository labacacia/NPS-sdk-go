// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import (
	"fmt"
	"time"
)

// TrustFrameValidationContext carries the inputs for ValidateTrustFrame — a
// mirror of the .NET TrustFrameValidationContext.
type TrustFrameValidationContext struct {
	// TrustedGrantors are the grantor CA NIDs this node pins as anchors.
	TrustedGrantors map[string]struct{}
	// ExpectedGranteeCA is the CA NID expected to be authorized by the frame.
	ExpectedGranteeCA string
	// RequiredCapabilities required for the current request (checked against trust_scope).
	RequiredCapabilities []string
	// TargetNodePath required for the current request (checked against nodes).
	TargetNodePath string
	// AsOf overrides the clock for the expiry check. Zero = time.Now().
	AsOf time.Time
}

// ValidateTrustFrame is a basic open TrustFrame validator for self-hosted
// deployments that pin trusted grantor anchors explicitly. It checks frame
// shape, expiry, grantor/grantee membership, required capability scope, and
// target node scope (mirror of the .NET TrustFrameValidator.Validate).
func ValidateTrustFrame(frame *TrustFrame, ctx TrustFrameValidationContext) IdentVerifyResult {
	if frame.GrantorNID == "" ||
		frame.GranteeCA == "" ||
		frame.IssuedAt == "" ||
		frame.ExpiresAt == "" ||
		frame.Serial == "" ||
		frame.SignerNID == "" ||
		frame.Signature == "" ||
		len(frame.TrustScope) == 0 ||
		len(frame.Nodes) == 0 {
		return fail(3, ErrTrustFrameInvalid,
			"TrustFrame is missing grantor, grantee, issued_at, expires_at, serial, signer_nid, signature, trust_scope, or nodes")
	}

	if _, err := time.Parse(time.RFC3339, frame.IssuedAt); err != nil {
		return fail(3, ErrTrustFrameInvalid,
			fmt.Sprintf("TrustFrame issued_at is not a valid timestamp: %s", frame.IssuedAt))
	}

	expiresAt, err := time.Parse(time.RFC3339, frame.ExpiresAt)
	if err != nil {
		return fail(3, ErrTrustFrameInvalid,
			fmt.Sprintf("TrustFrame expires_at is not a valid timestamp: %s", frame.ExpiresAt))
	}

	now := ctx.AsOf
	if now.IsZero() {
		now = time.Now()
	}
	if !expiresAt.After(now) {
		return fail(3, ErrTrustFrameExpired,
			fmt.Sprintf("TrustFrame expired at %s", frame.ExpiresAt))
	}

	if _, ok := ctx.TrustedGrantors[frame.GrantorNID]; !ok {
		return fail(3, ErrCertUntrustedIssuer,
			fmt.Sprintf("TrustFrame grantor '%s' is not a trusted grantor", frame.GrantorNID))
	}

	if frame.GranteeCA != ctx.ExpectedGranteeCA {
		return fail(3, ErrTrustFrameInvalid,
			fmt.Sprintf("TrustFrame grantee '%s' does not match expected CA '%s'",
				frame.GranteeCA, ctx.ExpectedGranteeCA))
	}

	if len(ctx.RequiredCapabilities) > 0 {
		granted := make(map[string]struct{}, len(frame.TrustScope))
		for _, c := range frame.TrustScope {
			granted[c] = struct{}{}
		}
		var missing []string
		for _, c := range ctx.RequiredCapabilities {
			if _, ok := granted[c]; !ok {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			return fail(5, ErrTrustFrameScopeExceedsGrantor,
				fmt.Sprintf("TrustFrame is missing required capabilities: %s", join(missing)))
		}
	}

	if ctx.TargetNodePath != "" {
		covered := false
		for _, pattern := range frame.Nodes {
			if NwpPathMatches(pattern, ctx.TargetNodePath) {
				covered = true
				break
			}
		}
		if !covered {
			return fail(6, ErrCertScopeViolation,
				fmt.Sprintf("target path '%s' is not covered by the TrustFrame node scope", ctx.TargetNodePath))
		}
	}

	return IdentVerifyResult{Valid: true}
}

func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
