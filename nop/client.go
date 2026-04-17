// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const contentTypeJSON = "application/json"

// NopClient is an HTTP-mode NOP client.
type NopClient struct {
	baseURL string
	http    *http.Client
}

func NewNopClient(baseURL string) *NopClient {
	return NewNopClientFull(baseURL, nil)
}

func NewNopClientFull(baseURL string, httpClient *http.Client) *NopClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &NopClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    httpClient,
	}
}

// Submit submits a TaskFrame and returns the task ID.
func (c *NopClient) Submit(ctx context.Context, frame *TaskFrame) (string, error) {
	body, err := json.Marshal(frame.ToDict())
	if err != nil {
		return "", err
	}
	resp, err := c.post(ctx, c.baseURL+"/tasks", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp.StatusCode, "Submit"); err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return "", err
	}
	taskID, _ := m["task_id"].(string)
	if taskID == "" {
		taskID = frame.TaskID
	}
	return taskID, nil
}

// GetStatus retrieves the current status of a task.
func (c *NopClient) GetStatus(ctx context.Context, taskID string) (*NopTaskStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/tasks/"+taskID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp.StatusCode, "GetStatus"); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return NewNopTaskStatus(raw), nil
}

// Cancel requests cancellation of a task.
func (c *NopClient) Cancel(ctx context.Context, taskID string) error {
	body, _ := json.Marshal(map[string]any{"task_id": taskID, "action": "cancel"})
	resp, err := c.post(ctx, c.baseURL+"/tasks/"+taskID+"/cancel", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp.StatusCode, "Cancel")
}

// WaitOptions controls the polling behaviour of Wait.
type WaitOptions struct {
	PollInterval time.Duration // default 500ms
	MaxAttempts  int           // 0 = unlimited (rely on ctx)
}

// Wait polls GetStatus until the task reaches a terminal state or ctx is done.
func (c *NopClient) Wait(ctx context.Context, taskID string, opts *WaitOptions) (*NopTaskStatus, error) {
	interval := 500 * time.Millisecond
	maxAttempts := 0
	if opts != nil {
		if opts.PollInterval > 0 {
			interval = opts.PollInterval
		}
		maxAttempts = opts.MaxAttempts
	}

	attempt := 0
	for {
		status, err := c.GetStatus(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if status.IsTerminal() {
			return status, nil
		}
		attempt++
		if maxAttempts > 0 && attempt >= maxAttempts {
			return status, fmt.Errorf("NOP Wait: exceeded %d poll attempts for task %s", maxAttempts, taskID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (c *NopClient) post(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Accept", contentTypeJSON)
	return c.http.Do(req)
}

func checkStatus(status int, op string) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return fmt.Errorf("NOP %s failed: HTTP %d", op, status)
}

// ensure io is used (for future streaming extensions)
var _ = io.ReadAll
