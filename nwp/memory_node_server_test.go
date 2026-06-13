// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubMemProvider struct{}

func (stubMemProvider) Query(_ context.Context, _ *QueryFrame, _ MemoryNodeOptions) (*MemoryNodeQueryResult, error) {
	return &MemoryNodeQueryResult{Rows: []MemoryNodeRow{{"id": "1"}}}, nil
}

func TestMemoryNodeServer_Manifest(t *testing.T) {
	srv := NewMemoryNodeServer(stubMemProvider{}, MemoryNodeOptions{
		NodeID:     "urn:nps:node:mem.example.com:m",
		PathPrefix: "/mem",
		Schema:     MemoryNodeSchema{Fields: []MemoryNodeField{{Name: "id", Type: "string"}}},
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mem/.nwm")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 from /.nwm, got %d", resp.StatusCode)
	}
}
