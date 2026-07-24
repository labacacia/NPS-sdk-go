// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labacacia/NPS-sdk-go/ncp"
)

// HTTPBridgeResponseAnchorRef is the anchor reference used for HTTP bridge
// response records.
const HTTPBridgeResponseAnchorRef = "nps://bridge/http-response/v1"

// HTTPBridgeDispatcher is the built-in Bridge dispatcher for HTTP and HTTPS
// endpoints.
type HTTPBridgeDispatcher struct {
	client *http.Client
}

// NewHTTPBridgeDispatcher creates an HTTP bridge dispatcher over an http.Client.
func NewHTTPBridgeDispatcher(client *http.Client) *HTTPBridgeDispatcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPBridgeDispatcher{client: client}
}

// Protocol returns "http".
func (d *HTTPBridgeDispatcher) Protocol() string { return BridgeProtocolHTTP }

// Dispatch performs the outbound HTTP request and maps the response into a CapsFrame.
func (d *HTTPBridgeDispatcher) Dispatch(ctx context.Context, frame *BridgeActionFrame, target *BridgeTarget) (*ncp.CapsFrame, error) {
	if frame == nil || target == nil {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "frame and target are required.")
	}

	uri, err := bridgeParseHTTPEndpoint(target)
	if err != nil {
		return nil, err
	}

	method := parseHTTPMethod(bridgeTargetString(target, "method", "POST"))

	var body io.Reader
	contentType := ""
	if method != http.MethodGet && method != http.MethodHead {
		if raw, ok := httpBridgeBody(frame, target); ok {
			body = bytes.NewReader(raw)
			contentType = bridgeTargetString(target, "content_type", "application/json")
		}
	}

	reqCtx, cancel := bridgeTimeoutContext(ctx, frame.TimeoutMs)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, uri.String(), body)
	if err != nil {
		return nil, newBridgeDispatchErrorCause(BridgeErrUpstreamFailed, "HTTP bridge request failed.", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	applyBridgeHeaders(req, target)

	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, newBridgeDispatchError(BridgeErrUpstreamFailed, "HTTP bridge request timed out.")
		}
		return nil, newBridgeDispatchErrorCause(BridgeErrUpstreamFailed, "HTTP bridge request failed.", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyText := string(bodyBytes)

	record := buildHTTPResponseRecord(resp, bodyText)
	return newBridgeCapsFrame(HTTPBridgeResponseAnchorRef, record, estimateTokenCost(bodyText)), nil
}

func parseHTTPMethod(method string) string {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		return http.MethodPost
	}
	return m
}

func httpBridgeBody(frame *BridgeActionFrame, target *BridgeTarget) (json.RawMessage, bool) {
	if obj, ok := frame.paramsObject(); ok {
		if b, has := obj["body"]; has {
			return b, true
		}
	}
	if b, ok := bridgeTargetJSON(target, "body"); ok {
		return b, true
	}
	return nil, false
}

func applyBridgeHeaders(req *http.Request, target *BridgeTarget) {
	raw, ok := bridgeTargetJSON(target, "headers")
	if !ok {
		return
	}
	var headers map[string]json.RawMessage
	if json.Unmarshal(raw, &headers) != nil {
		return
	}
	for name, rawVal := range headers {
		var value string
		if json.Unmarshal(rawVal, &value) != nil || value == "" {
			continue
		}
		req.Header.Set(name, value)
	}
}

func buildHTTPResponseRecord(resp *http.Response, bodyText string) map[string]interface{} {
	contentType := resp.Header.Get("Content-Type")
	record := map[string]interface{}{
		"status_code":   resp.StatusCode,
		"reason_phrase": httpReasonPhrase(resp),
		"success":       resp.StatusCode >= 200 && resp.StatusCode < 300,
		"content_type":  nullableString(contentType),
		"headers":       flattenHeaders(resp.Header),
	}
	writeBridgeBody(record, bodyText, contentType)
	return record
}

func writeBridgeBody(record map[string]interface{}, bodyText, contentType string) {
	if strings.TrimSpace(bodyText) != "" && strings.Contains(strings.ToLower(contentType), "json") {
		if json.Valid([]byte(bodyText)) {
			record["body"] = json.RawMessage(bodyText)
			return
		}
	}
	record["body_text"] = bodyText
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ",")
	}
	return out
}

func httpReasonPhrase(resp *http.Response) interface{} {
	// resp.Status is e.g. "200 OK"; strip the leading code to obtain the phrase.
	parts := strings.SplitN(resp.Status, " ", 2)
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}
	return nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// estimateTokenCost mirrors the .NET char-length/4 heuristic.
func estimateTokenCost(bodyText string) uint64 {
	if bodyText == "" {
		return 0
	}
	n := len([]rune(bodyText)) / 4
	if n < 1 {
		n = 1
	}
	return uint64(n)
}

func estimateTokenCostBytes(n int) uint64 {
	if n == 0 {
		return 0
	}
	c := n / 4
	if c < 1 {
		c = 1
	}
	return uint64(c)
}

func newBridgeCapsFrame(anchorRef string, record interface{}, tokenEst uint64) *ncp.CapsFrame {
	ref := anchorRef
	te := tokenEst
	return &ncp.CapsFrame{
		AnchorRef: &ref,
		Count:     1,
		Data:      []any{record},
		TokenEst:  &te,
	}
}

// bridgeTimeoutContext derives a context with the frame's timeout applied.
func bridgeTimeoutContext(ctx context.Context, timeoutMs uint) (context.Context, context.CancelFunc) {
	if timeoutMs > 0 {
		return context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	}
	return context.WithCancel(ctx)
}
