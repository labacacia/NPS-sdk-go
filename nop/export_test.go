// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

// Test-only shims exposing internal orchestrator helpers to the external
// nop_test package. Compiled only under `go test`.

// BuildCallbackSignatureForTest exposes buildCallbackSignature.
func BuildCallbackSignatureForTest(secret string, payload []byte) string {
	return buildCallbackSignature(secret, payload)
}

// FireCallbackForTest exposes fireCallback synchronously.
func FireCallbackForTest(o *NopOrchestrator, url, secret string, result *NopTaskResult) {
	o.fireCallback(url, secret, result)
}
