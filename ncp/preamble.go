// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"fmt"
	"io"
)

// NCP native-mode connection preamble — the 8-byte ASCII constant
// "NPS/1.0\n" that every native-mode client MUST emit immediately
// after the transport handshake and before its first HelloFrame.
// Defined by NPS-RFC-0001 and NPS-1 NCP §2.6.1.
//
// HTTP-mode connections do not use the preamble.

const (
	PreambleLiteral = "NPS/1.0\n"
	PreambleLength  = 8

	PreambleErrorCode  = "NCP-PREAMBLE-INVALID"
	PreambleStatusCode = "NPS-PROTO-PREAMBLE-INVALID"

	// PreambleReadTimeoutSecs is the validation timeout per NPS-RFC-0001 §4.1.
	PreambleReadTimeoutSecs = 10
	// PreambleCloseDeadlineMs is the max delay before closing on mismatch.
	PreambleCloseDeadlineMs = 500
)

var preambleBytes = []byte(PreambleLiteral)

// ErrPreambleInvalid is returned by ValidatePreamble when the bytes do not match.
type ErrPreambleInvalid struct {
	Reason string
}

func (e *ErrPreambleInvalid) Error() string {
	return fmt.Sprintf("NCP preamble invalid: %s", e.Reason)
}

// PreambleMatches returns true iff buf starts with the 8-byte NPS/1.0 preamble.
// Safe to call with shorter buffers.
func PreambleMatches(buf []byte) bool {
	if len(buf) < PreambleLength {
		return false
	}
	for i := 0; i < PreambleLength; i++ {
		if buf[i] != preambleBytes[i] {
			return false
		}
	}
	return true
}

// ValidatePreamble validates a presumed-preamble buffer.
// Returns nil on success or *ErrPreambleInvalid on failure.
func ValidatePreamble(buf []byte) error {
	if len(buf) < PreambleLength {
		return &ErrPreambleInvalid{
			Reason: fmt.Sprintf("short read (%d/%d bytes); peer is not speaking NCP", len(buf), PreambleLength),
		}
	}
	if !PreambleMatches(buf) {
		if len(buf) >= 4 && buf[0] == 'N' && buf[1] == 'P' && buf[2] == 'S' && buf[3] == '/' {
			return &ErrPreambleInvalid{
				Reason: "future-major-version NPS preamble; close with NPS-PREAMBLE-UNSUPPORTED-VERSION diagnostic",
			}
		}
		return &ErrPreambleInvalid{Reason: "preamble mismatch; peer is not speaking NPS/1.x"}
	}
	return nil
}

// WritePreamble writes the preamble bytes to w.
func WritePreamble(w io.Writer) error {
	_, err := w.Write(preambleBytes)
	return err
}
