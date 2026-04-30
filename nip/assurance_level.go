// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import "fmt"

// AssuranceLevel — Agent identity assurance level per NPS-RFC-0003 §5.1.1.
type AssuranceLevel struct {
	Wire string
	Rank int
}

var (
	AssuranceAnonymous = AssuranceLevel{Wire: "anonymous", Rank: 0}
	AssuranceAttested  = AssuranceLevel{Wire: "attested",  Rank: 1}
	AssuranceVerified  = AssuranceLevel{Wire: "verified",  Rank: 2}
)

// MeetsOrExceeds reports whether l satisfies the required minimum.
func (l AssuranceLevel) MeetsOrExceeds(required AssuranceLevel) bool {
	return l.Rank >= required.Rank
}

// AssuranceFromWire parses a wire string. Empty input maps to ANONYMOUS.
func AssuranceFromWire(wire string) (AssuranceLevel, error) {
	if wire == "" {
		return AssuranceAnonymous, nil
	}
	for _, l := range []AssuranceLevel{AssuranceAnonymous, AssuranceAttested, AssuranceVerified} {
		if l.Wire == wire {
			return l, nil
		}
	}
	return AssuranceLevel{}, fmt.Errorf("unknown assurance_level: %q", wire)
}

// AssuranceFromRank parses an integer rank (0/1/2).
func AssuranceFromRank(rank int) (AssuranceLevel, error) {
	for _, l := range []AssuranceLevel{AssuranceAnonymous, AssuranceAttested, AssuranceVerified} {
		if l.Rank == rank {
			return l, nil
		}
	}
	return AssuranceLevel{}, fmt.Errorf("unknown assurance_level rank: %d", rank)
}
