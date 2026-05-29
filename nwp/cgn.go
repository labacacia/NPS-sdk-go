// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"encoding/json"
	"fmt"
)

const bytesPerCgn = 4

// EstimateCgn returns CGN = ceil(UTF-8 bytes / 4) for a string.
// Returns 0 for empty input.
func EstimateCgn(s string) int {
	n := len([]byte(s))
	if n == 0 {
		return 0
	}
	return (n + bytesPerCgn - 1) / bytesPerCgn
}

// EstimateCgnBytes returns CGN for a raw byte slice.
func EstimateCgnBytes(b []byte) int {
	n := len(b)
	if n == 0 {
		return 0
	}
	return (n + bytesPerCgn - 1) / bytesPerCgn
}

// EstimateCgnJSON marshals v as compact JSON and returns its CGN estimate.
func EstimateCgnJSON(v any) (int, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	return EstimateCgnBytes(b), nil
}

// EstimateCgnRows returns the sum of CGN estimates for a slice of values.
func EstimateCgnRows(rows []any) (int, error) {
	total := 0
	for _, r := range rows {
		n, err := EstimateCgnJSON(r)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// TokenBudgetMeta is the wire-level CGN budget descriptor (NWP §5).
type TokenBudgetMeta struct {
	CgnLimit            int      `json:"cgn_limit"`
	Tokenizer           string   `json:"tokenizer,omitempty"`
	SupportedTokenizers []string `json:"supported_tokenizers,omitempty"`
	TokenBudgetHint     bool     `json:"token_budget_hint,omitempty"`
	Profile             string   `json:"profile,omitempty"`
}

// DefaultProfile returns "cgn.v1" when Profile is empty.
func (t TokenBudgetMeta) DefaultProfile() string {
	if t.Profile == "" {
		return "cgn.v1"
	}
	return t.Profile
}

// BudgetExceededError is returned when a response payload exceeds the CGN budget.
type BudgetExceededError struct {
	Requested int
	Limit     int
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("CGN budget exceeded: %d > %d", e.Requested, e.Limit)
}
