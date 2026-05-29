// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import "testing"

func TestReputationPolicyZeroValue(t *testing.T) {
	var p ReputationPolicy
	// Zero value: Enabled is false, slices are nil, strings are empty
	if p.Enabled {
		t.Error("zero-value ReputationPolicy should have Enabled=false")
	}
	if p.LogSources != nil {
		t.Error("zero-value LogSources should be nil")
	}
	if p.CacheTtlSeconds != 0 {
		t.Error("zero-value CacheTtlSeconds should be 0")
	}
	if p.BanTtlSeconds != 0 {
		t.Error("zero-value BanTtlSeconds should be 0")
	}
}

func TestReputationPolicyFields(t *testing.T) {
	p := ReputationPolicy{
		Enabled:           true,
		LogSources:        []string{"https://log.example.com"},
		MinAssuranceLevel: "anonymous",
		CacheTtlSeconds:   300,
		BanTtlSeconds:     3600,
		OnLogUnavailable:  "allow",
	}
	if !p.Enabled {
		t.Error("Enabled should be true")
	}
	if len(p.LogSources) != 1 {
		t.Errorf("LogSources length: want 1, got %d", len(p.LogSources))
	}
	if p.MinAssuranceLevel != "anonymous" {
		t.Errorf("MinAssuranceLevel: want 'anonymous', got %q", p.MinAssuranceLevel)
	}
	if p.CacheTtlSeconds != 300 {
		t.Errorf("CacheTtlSeconds: want 300, got %d", p.CacheTtlSeconds)
	}
}

func TestRepOutcomeConstants(t *testing.T) {
	// Verify the four outcomes are distinct and ordered
	outcomes := []ReputationOutcome{RepAccept, RepThrottle, RepReject, RepBan}
	seen := make(map[ReputationOutcome]bool)
	for _, o := range outcomes {
		if seen[o] {
			t.Errorf("duplicate outcome value: %d", o)
		}
		seen[o] = true
	}
	if len(seen) != 4 {
		t.Errorf("expected 4 distinct outcomes, got %d", len(seen))
	}
	// Check ordering: Accept < Throttle < Reject < Ban
	if !(RepAccept < RepThrottle && RepThrottle < RepReject && RepReject < RepBan) {
		t.Error("outcome ordering not as expected")
	}
	if RepAccept != 0 {
		t.Errorf("RepAccept should be 0, got %d", RepAccept)
	}
}

func TestReputationDecisionFields(t *testing.T) {
	rule := ReputationRule{Incident: "*", Severity: ">=major", Count: 2}
	d := ReputationDecision{
		Outcome:     RepBan,
		MatchedRule: &rule,
		ErrorCode:   "NWP-AUTH-REPUTATION-BLOCKED",
	}
	if d.Outcome != RepBan {
		t.Errorf("Outcome: want RepBan, got %d", d.Outcome)
	}
	if d.MatchedRule == nil {
		t.Error("MatchedRule should not be nil")
	}
	if d.MatchedRule.Incident != "*" {
		t.Errorf("MatchedRule.Incident: want *, got %q", d.MatchedRule.Incident)
	}
	if d.ErrorCode != "NWP-AUTH-REPUTATION-BLOCKED" {
		t.Errorf("ErrorCode: want NWP-AUTH-REPUTATION-BLOCKED, got %q", d.ErrorCode)
	}
}

func TestReputationDecisionAccept(t *testing.T) {
	d := ReputationDecision{Outcome: RepAccept}
	if d.Outcome != RepAccept {
		t.Errorf("Outcome: want RepAccept, got %d", d.Outcome)
	}
	if d.MatchedRule != nil {
		t.Error("MatchedRule should be nil for Accept")
	}
	if d.ErrorCode != "" {
		t.Errorf("ErrorCode should be empty, got %q", d.ErrorCode)
	}
}

func TestReputationRuleFields(t *testing.T) {
	days := uint(30)
	r := ReputationRule{
		Incident:   "spam",
		Severity:   ">=minor",
		WithinDays: &days,
		Count:      3,
	}
	if r.Incident != "spam" {
		t.Errorf("Incident: want 'spam', got %q", r.Incident)
	}
	if r.Severity != ">=minor" {
		t.Errorf("Severity: want '>=minor', got %q", r.Severity)
	}
	if r.WithinDays == nil || *r.WithinDays != 30 {
		t.Error("WithinDays: want pointer to 30")
	}
	if r.Count != 3 {
		t.Errorf("Count: want 3, got %d", r.Count)
	}
}
