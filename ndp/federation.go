// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ndp

import (
	"fmt"
	"strings"
)

const (
	ForwardedByHeader = "ndp-forwarded-by"
	MaxFederationHops = 3
)

func ParseForwardedBy(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if hop := strings.TrimSpace(part); hop != "" {
			out = append(out, hop)
		}
	}
	return out
}

func AppendForwardedBy(ownNID, header string) (nextHeader string, shouldForward bool, err error) {
	hops := ParseForwardedBy(header)
	for _, hop := range hops {
		if hop == ownNID {
			return "", false, fmt.Errorf("%s: own NID already appears in ndp-forwarded-by", ErrFederationLoop)
		}
	}
	if len(hops) >= MaxFederationHops {
		return "", false, nil
	}
	return strings.Join(append(hops, ownNID), ", "), true, nil
}
