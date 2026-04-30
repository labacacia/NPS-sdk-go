// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/labacacia/NPS-sdk-go/ncp"
)

var specBytes = []byte{0x4E, 0x50, 0x53, 0x2F, 0x31, 0x2E, 0x30, 0x0A}

func TestPreambleBytesAreExactlyTheSpecConstant(t *testing.T) {
	if ncp.PreambleLength != 8 {
		t.Fatalf("length: got %d, want 8", ncp.PreambleLength)
	}
	if ncp.PreambleLiteral != "NPS/1.0\n" {
		t.Fatalf("literal: got %q", ncp.PreambleLiteral)
	}
	if !bytes.Equal([]byte(ncp.PreambleLiteral), specBytes) {
		t.Fatalf("bytes: got %x, want %x", []byte(ncp.PreambleLiteral), specBytes)
	}
}

func TestPreambleMatchesExactPreamble(t *testing.T) {
	if !ncp.PreambleMatches([]byte(ncp.PreambleLiteral)) {
		t.Fatal("expected match on exact preamble")
	}
}

func TestPreambleMatchesAtStartOfLongerBuffer(t *testing.T) {
	combined := make([]byte, 16)
	copy(combined, ncp.PreambleLiteral)
	combined[8] = 0x06
	if !ncp.PreambleMatches(combined) {
		t.Fatal("expected match on combined buffer")
	}
}

func TestPreambleShortReadsDoNotMatch(t *testing.T) {
	for _, n := range []int{0, 1, 7} {
		if ncp.PreambleMatches([]byte(ncp.PreambleLiteral)[:n]) {
			t.Fatalf("length=%d should not match", n)
		}
	}
}

func TestValidatePreambleAcceptsExact(t *testing.T) {
	if err := ncp.ValidatePreamble([]byte(ncp.PreambleLiteral)); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidatePreambleRejectsShortRead(t *testing.T) {
	err := ncp.ValidatePreamble([]byte{0, 0, 0})
	var pe *ncp.ErrPreambleInvalid
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ErrPreambleInvalid, got %T", err)
	}
	if !strings.Contains(pe.Reason, "short read") || !strings.Contains(pe.Reason, "3/8") {
		t.Fatalf("reason missing expected text: %q", pe.Reason)
	}
}

func TestValidatePreambleRejectsGarbage(t *testing.T) {
	err := ncp.ValidatePreamble([]byte("GET / HTT"))
	var pe *ncp.ErrPreambleInvalid
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ErrPreambleInvalid, got %T", err)
	}
	if strings.Contains(pe.Reason, "future") || !strings.Contains(pe.Reason, "not speaking NPS") {
		t.Fatalf("reason: %q", pe.Reason)
	}
}

func TestValidatePreambleFlagsFutureMajor(t *testing.T) {
	err := ncp.ValidatePreamble([]byte("NPS/2.0\n"))
	var pe *ncp.ErrPreambleInvalid
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ErrPreambleInvalid, got %T", err)
	}
	if !strings.Contains(pe.Reason, "future-major") {
		t.Fatalf("reason: %q", pe.Reason)
	}
}

func TestWritePreambleEmitsExactlyTheConstantBytes(t *testing.T) {
	var buf bytes.Buffer
	if err := ncp.WritePreamble(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), specBytes) {
		t.Fatalf("got %x, want %x", buf.Bytes(), specBytes)
	}
}

func TestPreambleErrorAndStatusCodeConstants(t *testing.T) {
	if ncp.PreambleErrorCode != "NCP-PREAMBLE-INVALID" {
		t.Fatalf("error code: %q", ncp.PreambleErrorCode)
	}
	if ncp.PreambleStatusCode != "NPS-PROTO-PREAMBLE-INVALID" {
		t.Fatalf("status code: %q", ncp.PreambleStatusCode)
	}
}
