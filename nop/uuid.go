// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"crypto/rand"
	"fmt"
)

// newUUID returns a random RFC-4122 v4 UUID string (no external dependencies).
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to a zero UUID rather than panic.
		return "00000000-0000-0000-0000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
