// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import "testing"

func TestNpsStatusCodeValues(t *testing.T) {
	if NpsOk != "NPS-OK" {
		t.Errorf("NpsOk = %q, want NPS-OK", NpsOk)
	}
	if NpsAuthForbidden != "NPS-AUTH-FORBIDDEN" {
		t.Errorf("NpsAuthForbidden = %q, want NPS-AUTH-FORBIDDEN", NpsAuthForbidden)
	}
	if NpsClientRateLimited != "NPS-CLIENT-RATE-LIMITED" {
		t.Errorf("NpsClientRateLimited = %q, want NPS-CLIENT-RATE-LIMITED", NpsClientRateLimited)
	}
	if NpsLimitExceeded != "NPS-LIMIT-EXCEEDED" {
		t.Errorf("NpsLimitExceeded = %q, want NPS-LIMIT-EXCEEDED", NpsLimitExceeded)
	}
	if NpsClientRequestTooLarge != "NPS-CLIENT-REQUEST-TOO-LARGE" {
		t.Errorf("NpsClientRequestTooLarge = %q, want NPS-CLIENT-REQUEST-TOO-LARGE", NpsClientRequestTooLarge)
	}
}

func TestHttpStatusMapNotEmpty(t *testing.T) {
	if len(HttpStatusMap) == 0 {
		t.Error("HttpStatusMap is empty")
	}
}

func TestToHttpStatus(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{NpsOk, 200},
		{NpsClientNotFound, 404},
		{NpsAuthForbidden, 403},
		{NpsAuthUnauthenticated, 401},
		{NpsLimitPayload, 413},
		{NpsServerInternal, 500},
		{NpsProtoVersionIncompatible, 426},
		{"NPS-UNKNOWN-CODE", 500},
	}
	for _, c := range cases {
		got := ToHttpStatus(c.code)
		if got != c.want {
			t.Errorf("ToHttpStatus(%q) = %d, want %d", c.code, got, c.want)
		}
	}
}
