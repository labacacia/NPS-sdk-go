// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package ncp

import (
	"testing"

	"github.com/labacacia/NPS-sdk-go/core"
)

func TestNcpErrorCodeValues(t *testing.T) {
	if ErrAnchorNotFound != "NCP-ANCHOR-NOT-FOUND" {
		t.Errorf("ErrAnchorNotFound = %q, want NCP-ANCHOR-NOT-FOUND", ErrAnchorNotFound)
	}
	if ErrPreambleInvalidCode != "NCP-PREAMBLE-INVALID" {
		t.Errorf("ErrPreambleInvalidCode = %q, want NCP-PREAMBLE-INVALID", ErrPreambleInvalidCode)
	}
	if ErrStreamWindowOverflow != "NCP-STREAM-WINDOW-OVERFLOW" {
		t.Errorf("ErrStreamWindowOverflow = %q, want NCP-STREAM-WINDOW-OVERFLOW", ErrStreamWindowOverflow)
	}
}

func TestNcpErrorToNpsStatusNotEmpty(t *testing.T) {
	if len(NcpErrorToNpsStatus) == 0 {
		t.Error("NcpErrorToNpsStatus is empty")
	}
}

func TestNcpErrorToNpsStatusMappings(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{ErrAnchorNotFound, core.NpsClientNotFound},
		{ErrFramePayloadTooLarge, core.NpsLimitPayload},
		{ErrStreamSeqGap, core.NpsStreamSeqGap},
		{ErrVersionIncompatible, core.NpsProtoVersionIncompatible},
		{ErrPreambleInvalidCode, core.NpsProtoPreambleInvalid},
		{ErrEncodingUnsupported, core.NpsServerEncodingUnsupported},
	}
	for _, c := range cases {
		got, ok := NcpErrorToNpsStatus[c.code]
		if !ok {
			t.Errorf("NcpErrorToNpsStatus[%q] not found", c.code)
			continue
		}
		if got != c.want {
			t.Errorf("NcpErrorToNpsStatus[%q] = %q, want %q", c.code, got, c.want)
		}
	}
}
