// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labacacia/NPS-sdk-go/core"
	"github.com/labacacia/NPS-sdk-go/ncp"
)

const (
	mimeNwpFrame   = "application/nwp-frame"
	mimeNwpCapsule = "application/nwp-capsule"
	mimeManifest   = "application/nwp-manifest+json"
)

// NwpClient is an HTTP-mode NWP client.
type NwpClient struct {
	baseURL string
	codec   *core.NpsFrameCodec
	tier    core.EncodingTier
	http    *http.Client
}

func NewNwpClient(baseURL string) *NwpClient {
	return NewNwpClientFull(baseURL, core.EncodingTierMsgPack, core.CreateFullRegistry(), nil)
}

func NewNwpClientFull(baseURL string, tier core.EncodingTier, reg *core.FrameRegistry, httpClient *http.Client) *NwpClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &NwpClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		codec:   core.NewNpsFrameCodec(reg),
		tier:    tier,
		http:    httpClient,
	}
}

// Query sends a QueryFrame and returns the CapsFrame response.
func (c *NwpClient) Query(ctx context.Context, frame *QueryFrame) (*ncp.CapsFrame, error) {
	wire, err := c.codec.Encode(frame.FrameType(), frame.ToDict(), c.tier, true)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, c.baseURL+"/query", wire)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkOK(resp.StatusCode, "/query"); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	ft, dict, err := c.codec.Decode(body)
	if err != nil {
		return nil, err
	}
	if ft != core.FrameTypeCaps {
		return nil, fmt.Errorf("expected CapsFrame, got 0x%02X", ft)
	}
	return ncp.CapsFrameFromDict(dict), nil
}

// Stream sends a QueryFrame and returns all StreamFrames.
func (c *NwpClient) Stream(ctx context.Context, frame *QueryFrame) ([]*ncp.StreamFrame, error) {
	wire, err := c.codec.Encode(frame.FrameType(), frame.ToDict(), c.tier, true)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, c.baseURL+"/stream", wire)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkOK(resp.StatusCode, "/stream"); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)

	var frames []*ncp.StreamFrame
	offset := 0
	for offset < len(body) {
		hdr, err := core.ParseFrameHeader(body[offset:])
		if err != nil {
			return nil, err
		}
		total := hdr.HeaderSize() + int(hdr.PayloadLength)
		ft, dict, err := c.codec.Decode(body[offset : offset+total])
		if err != nil {
			return nil, err
		}
		if ft != core.FrameTypeStream {
			return nil, fmt.Errorf("expected StreamFrame, got 0x%02X", ft)
		}
		sf := ncp.StreamFrameFromDict(dict)
		frames = append(frames, sf)
		if sf.IsLast {
			break
		}
		offset += total
	}
	return frames, nil
}

// Invoke sends an ActionFrame. Returns CapsFrame, AsyncActionResponse, or raw map.
type InvokeResult struct {
	Frame *core.FrameDict
	Async *AsyncActionResponse
	JSON  map[string]any
}

func (c *NwpClient) Invoke(ctx context.Context, frame *ActionFrame) (*InvokeResult, error) {
	wire, err := c.codec.Encode(frame.FrameType(), frame.ToDict(), c.tier, true)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, c.baseURL+"/invoke", wire)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkOK(resp.StatusCode, "/invoke"); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	ct := resp.Header.Get("Content-Type")

	if frame.Async {
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, err
		}
		return &InvokeResult{Async: AsyncActionResponseFromDict(m)}, nil
	}
	if strings.Contains(ct, mimeNwpFrame) {
		_, dict, err := c.codec.Decode(body)
		if err != nil {
			return nil, err
		}
		return &InvokeResult{Frame: &dict}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return &InvokeResult{JSON: m}, nil
}

// FetchManifest retrieves the node manifest from /.nwm.
func (c *NwpClient) FetchManifest(ctx context.Context) (map[string]any, error) {
	return c.getJSON(ctx, c.baseURL+"/.nwm")
}

// FetchSchema retrieves the node schema from /.schema.
func (c *NwpClient) FetchSchema(ctx context.Context) (any, error) {
	return c.getAny(ctx, c.baseURL+"/.schema")
}

// ListActions retrieves the available actions from /actions.
func (c *NwpClient) ListActions(ctx context.Context) (any, error) {
	return c.getAny(ctx, c.baseURL+"/actions")
}

// Subscribe sends a SubscribeFrame to /subscribe and returns the CapsFrame response.
func (c *NwpClient) Subscribe(ctx context.Context, frame *SubscribeFrame) (*ncp.CapsFrame, error) {
	wire, err := c.codec.Encode(frame.FrameType(), frame.ToDict(), c.tier, true)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, c.baseURL+"/subscribe", wire)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkOK(resp.StatusCode, "/subscribe"); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	ft, dict, err := c.codec.Decode(body)
	if err != nil {
		return nil, err
	}
	if ft != core.FrameTypeCaps {
		return nil, fmt.Errorf("expected CapsFrame, got 0x%02X", ft)
	}
	return ncp.CapsFrameFromDict(dict), nil
}

func (c *NwpClient) post(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mimeNwpFrame)
	req.Header.Set("Accept", mimeNwpFrame)
	return c.http.Do(req)
}

func (c *NwpClient) getJSON(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", mimeManifest)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkOK(resp.StatusCode, url); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *NwpClient) getAny(ctx context.Context, url string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkOK(resp.StatusCode, url); err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func checkOK(status int, path string) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return fmt.Errorf("NWP %s failed: HTTP %d", path, status)
}
