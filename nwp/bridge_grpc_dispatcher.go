// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labacacia/NPS-sdk-go/ncp"
)

// GRPCBridgeResponseAnchorRef is the anchor reference used for gRPC bridge
// response records.
const GRPCBridgeResponseAnchorRef = "nps://bridge/grpc-json-response/v1"

// GRPCBridgeDispatcher is the built-in Bridge dispatcher for unary gRPC calls
// using the JSON gRPC codec (application/grpc+json). The endpoint path
// identifies the service and method, e.g. https://host/Package.Service/Method.
type GRPCBridgeDispatcher struct {
	client *http.Client
}

// NewGRPCBridgeDispatcher creates a gRPC bridge dispatcher over an http.Client.
func NewGRPCBridgeDispatcher(client *http.Client) *GRPCBridgeDispatcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &GRPCBridgeDispatcher{client: client}
}

// Protocol returns "grpc".
func (d *GRPCBridgeDispatcher) Protocol() string { return BridgeProtocolGRPC }

// Dispatch performs the outbound gRPC-JSON unary call and maps the response.
func (d *GRPCBridgeDispatcher) Dispatch(ctx context.Context, frame *BridgeActionFrame, target *BridgeTarget) (*ncp.CapsFrame, error) {
	if frame == nil || target == nil {
		return nil, newBridgeDispatchError(BridgeErrTargetInvalid, "frame and target are required.")
	}

	uri, err := bridgeParseHTTPEndpoint(target)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := bridgeTimeoutContext(ctx, frame.TimeoutMs)
	defer cancel()

	payload := buildGRPCMessage(frame, target)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, uri.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, newBridgeDispatchErrorCause(BridgeErrUpstreamFailed, "gRPC bridge request failed.", err)
	}
	req.Header.Set("Content-Type", "application/grpc+json")
	req.Header.Set("te", "trailers")
	applyBridgeHeaders(req, target)

	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, newBridgeDispatchError(BridgeErrUpstreamFailed, "gRPC bridge request timed out.")
		}
		return nil, newBridgeDispatchErrorCause(BridgeErrUpstreamFailed, "gRPC bridge request failed.", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	record := buildGRPCResponseRecord(resp, bodyBytes)
	return newBridgeCapsFrame(GRPCBridgeResponseAnchorRef, record, estimateTokenCostBytes(len(bodyBytes))), nil
}

func buildGRPCMessage(frame *BridgeActionFrame, target *BridgeTarget) []byte {
	var payload json.RawMessage
	if p, ok := bridgeTargetJSON(target, "grpc_message"); ok {
		payload = p
	} else if p, ok := bridgeTargetJSON(target, "message"); ok {
		payload = p
	} else if p, ok := bridgeTargetJSON(target, "body"); ok {
		payload = p
	} else if obj, ok := frame.paramsObject(); ok {
		if p, has := obj["grpc_message"]; has {
			payload = p
		} else {
			payload = frame.Params
		}
	} else if len(frame.Params) > 0 {
		payload = frame.Params
	} else {
		payload = json.RawMessage("{}")
	}

	jsonBytes := []byte(payload)
	wire := make([]byte, len(jsonBytes)+5)
	wire[0] = 0
	binary.BigEndian.PutUint32(wire[1:5], uint32(len(jsonBytes)))
	copy(wire[5:], jsonBytes)
	return wire
}

func buildGRPCResponseRecord(resp *http.Response, body []byte) map[string]interface{} {
	grpcStatus := readGRPCHeader(resp, "grpc-status")
	success := resp.StatusCode >= 200 && resp.StatusCode < 300 && (grpcStatus == "0" || grpcStatus == "")

	messages := make([]interface{}, 0)
	for _, msg := range readGRPCMessages(body) {
		messages = append(messages, decodeGRPCMessage(msg))
	}

	return map[string]interface{}{
		"status_code":  resp.StatusCode,
		"success":      success,
		"content_type": nullableString(resp.Header.Get("Content-Type")),
		"grpc_status":  nullableString(grpcStatus),
		"grpc_message": nullableString(readGRPCHeader(resp, "grpc-message")),
		"headers":      flattenHeaders(resp.Header),
		"trailers":     flattenHeaders(resp.Trailer),
		"messages":     messages,
	}
}

func readGRPCHeader(resp *http.Response, name string) string {
	if v := resp.Trailer.Get(name); v != "" {
		return v
	}
	return resp.Header.Get(name)
}

func readGRPCMessages(body []byte) [][]byte {
	var out [][]byte
	offset := 0
	for len(body)-offset >= 5 {
		compressed := body[offset] != 0
		length := binary.BigEndian.Uint32(body[offset+1 : offset+5])
		offset += 5
		if compressed || uint64(length) > uint64(len(body)-offset) {
			break
		}
		out = append(out, body[offset:offset+int(length)])
		offset += int(length)
	}
	return out
}

func decodeGRPCMessage(message []byte) interface{} {
	if json.Valid(message) {
		return json.RawMessage(message)
	}
	// Fall back to base64, matching Utf8JsonWriter.WriteBase64StringValue.
	return base64.StdEncoding.EncodeToString(message)
}
