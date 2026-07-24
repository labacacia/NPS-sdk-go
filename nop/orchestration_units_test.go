// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labacacia/NPS-sdk-go/nop"
)

func ctx(entries map[string]string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(entries))
	for k, v := range entries {
		out[k] = json.RawMessage(v)
	}
	return out
}

func mustEval(t *testing.T, expr string, c map[string]json.RawMessage) bool {
	t.Helper()
	ok, err := nop.EvaluateCondition(expr, c)
	if err != nil {
		t.Fatalf("Evaluate(%q) error: %v", expr, err)
	}
	return ok
}

// ── Condition truth table ──────────────────────────────────────────────────────

func TestCond_Numeric(t *testing.T) {
	cases := []struct {
		expr string
		c    map[string]string
		want bool
	}{
		{"$.analyze.score > 0.7", map[string]string{"analyze": `{"score":0.92}`}, true},
		{"$.analyze.score > 0.7", map[string]string{"analyze": `{"score":0.5}`}, false},
		{"$.n.val >= 5", map[string]string{"n": `{"val":5}`}, true},
		{"$.n.count < 10", map[string]string{"n": `{"count":3}`}, true},
		{"$.n.count <= 10", map[string]string{"n": `{"count":10}`}, true},
	}
	for _, tc := range cases {
		if got := mustEval(t, tc.expr, ctx(tc.c)); got != tc.want {
			t.Errorf("%q => %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestCond_StringAndNull(t *testing.T) {
	if !mustEval(t, `$.n.status != "ok"`, ctx(map[string]string{"n": `{"status":"error"}`})) {
		t.Error("string != failed")
	}
	if !mustEval(t, `$.c.label == "positive"`, ctx(map[string]string{"c": `{"label":"positive"}`})) {
		t.Error("string == failed")
	}
	if mustEval(t, `$.c.label == "positive"`, ctx(map[string]string{"c": `{"label":"negative"}`})) {
		t.Error("string == false-negative")
	}
	// Missing field resolves to null; null == null true.
	if !mustEval(t, "$.n.missing == null", ctx(map[string]string{"n": `{}`})) {
		t.Error("null == null failed")
	}
	if !mustEval(t, "$.n.x != null", ctx(map[string]string{"n": `{"x":1}`})) {
		t.Error("value != null failed")
	}
}

func TestCond_BooleanLogic(t *testing.T) {
	c := ctx(map[string]string{"n": `{"score":0.9,"count":5}`})
	if !mustEval(t, "$.n.score > 0.7 && $.n.count > 0", c) {
		t.Error("&& both-true failed")
	}
	c2 := ctx(map[string]string{"n": `{"score":0.9,"count":0}`})
	if mustEval(t, "$.n.score > 0.7 && $.n.count > 0", c2) {
		t.Error("&& one-false should be false")
	}
	c3 := ctx(map[string]string{"n": `{"a":1,"b":0}`})
	if !mustEval(t, "$.n.a > 5 || $.n.b == 0", c3) {
		t.Error("|| one-true failed")
	}
	if !mustEval(t, "!$.n.ok", ctx(map[string]string{"n": `{"ok":false}`})) {
		t.Error("! negate failed")
	}
	c4 := ctx(map[string]string{"n": `{"a":0,"b":1,"c":1}`})
	if !mustEval(t, "($.n.a > 0 || $.n.b > 0) && $.n.c > 0", c4) {
		t.Error("grouping failed")
	}
}

func TestCond_Literals(t *testing.T) {
	empty := map[string]json.RawMessage{}
	if !mustEval(t, "true", empty) {
		t.Error("true literal")
	}
	if mustEval(t, "false", empty) {
		t.Error("false literal")
	}
	if !mustEval(t, "", empty) {
		t.Error("empty condition should be true")
	}
}

func TestCond_UnknownToken_Errors(t *testing.T) {
	_, err := nop.EvaluateCondition("$.n.x @@ 1", map[string]json.RawMessage{})
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
}

// ── Input mapper ────────────────────────────────────────────────────────────────

func TestMapper_Resolve(t *testing.T) {
	c := ctx(map[string]string{"n1": `{"a":{"b":42},"x":"hi"}`})
	got, err := nop.ResolvePath("$.n1.a.b", c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "42" {
		t.Errorf("resolve nested: %s", got)
	}
	// Missing → nil, no error.
	got, err = nop.ResolvePath("$.n1.missing", c)
	if err != nil || got != nil {
		t.Errorf("missing should be (nil,nil): %s %v", got, err)
	}
	// Unknown node → nil.
	got, _ = nop.ResolvePath("$.other.x", c)
	if got != nil {
		t.Errorf("unknown node should be nil: %s", got)
	}
}

func TestMapper_BadPrefix_Errors(t *testing.T) {
	_, err := nop.ResolvePath("n1.a", map[string]json.RawMessage{})
	if err == nil {
		t.Fatal("expected error for non-$. path")
	}
}

func TestMapper_DepthLimit_Errors(t *testing.T) {
	deep := "$." + strings.Repeat("a.", nop.MaxInputMappingDepth+2)
	deep = strings.TrimSuffix(deep, ".")
	_, err := nop.ResolvePath(deep, map[string]json.RawMessage{})
	if err == nil {
		t.Fatalf("expected depth error for %q", deep)
	}
}

func TestMapper_BuildParams(t *testing.T) {
	c := ctx(map[string]string{"n1": `{"id":"x1","n":7}`})
	mapping := map[string]json.RawMessage{
		"the_id": json.RawMessage(`"$.n1.id"`),
		"lit":    json.RawMessage(`123`),
	}
	out, err := nop.BuildParams(mapping, c)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["the_id"] != "x1" {
		t.Errorf("the_id resolved wrong: %v", m["the_id"])
	}
	if m["lit"] != float64(123) {
		t.Errorf("literal wrong: %v", m["lit"])
	}
}

// ── Aggregator ──────────────────────────────────────────────────────────────────

func TestAgg_Merge(t *testing.T) {
	res := []json.RawMessage{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`)}
	out := nop.Aggregate(nop.AggregateStrategyMerge, res, 0)
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["a"] != float64(1) || m["b"] != float64(2) {
		t.Errorf("merge wrong: %v", m)
	}
}

func TestAgg_First(t *testing.T) {
	res := []json.RawMessage{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`)}
	out := nop.Aggregate(nop.AggregateStrategyFirst, res, 0)
	if strings.TrimSpace(string(out)) != `{"a":1}` {
		t.Errorf("first wrong: %s", out)
	}
}

func TestAgg_All(t *testing.T) {
	res := []json.RawMessage{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`)}
	out := nop.Aggregate(nop.AggregateStrategyAll, res, 0)
	var arr []any
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatal(err)
	}
	if len(arr) != 2 {
		t.Errorf("all length: %d", len(arr))
	}
}

func TestAgg_FastestK(t *testing.T) {
	res := []json.RawMessage{json.RawMessage(`{"a":1}`), json.RawMessage(`{"b":2}`), json.RawMessage(`{"c":3}`)}
	out := nop.Aggregate(nop.AggregateStrategyFastestK, res, 2)
	var arr []any
	_ = json.Unmarshal(out, &arr)
	if len(arr) != 2 {
		t.Errorf("fastest_k length: %d", len(arr))
	}
}

func TestAgg_Empty(t *testing.T) {
	out := nop.Aggregate(nop.AggregateStrategyMerge, nil, 0)
	if strings.TrimSpace(string(out)) != "{}" {
		t.Errorf("empty should be {}: %s", out)
	}
}

// ── DAG validator ────────────────────────────────────────────────────────────────

func TestDag_Valid(t *testing.T) {
	dag := &nop.TaskDag{
		Nodes: []*nop.DagNode{
			{ID: "a", Action: "x", Agent: "a"},
			{ID: "b", Action: "x", Agent: "b"},
		},
		Edges: []*nop.DagEdge{{From: "a", To: "b"}},
	}
	r := nop.ValidateDag(dag)
	if !r.IsValid {
		t.Fatalf("expected valid: %s", r.ErrorMessage)
	}
	if len(r.TopologicalOrder) != 2 || r.TopologicalOrder[0] != "a" {
		t.Errorf("topo order: %v", r.TopologicalOrder)
	}
}

func TestDag_Cycle(t *testing.T) {
	dag := &nop.TaskDag{
		Nodes: []*nop.DagNode{
			{ID: "s", Action: "x", Agent: "s"}, {ID: "a", Action: "x", Agent: "a"},
			{ID: "b", Action: "x", Agent: "b"}, {ID: "e", Action: "x", Agent: "e"},
		},
		Edges: []*nop.DagEdge{
			{From: "s", To: "a"}, {From: "a", To: "b"}, {From: "b", To: "a"}, {From: "a", To: "e"},
		},
	}
	r := nop.ValidateDag(dag)
	if r.IsValid || r.ErrorCode != nop.ErrTaskDagCycle {
		t.Fatalf("expected cycle: %+v", r)
	}
}

func TestDag_Duplicate(t *testing.T) {
	dag := &nop.TaskDag{
		Nodes: []*nop.DagNode{{ID: "a", Action: "x", Agent: "a"}, {ID: "a", Action: "x", Agent: "a"}},
	}
	r := nop.ValidateDag(dag)
	if r.IsValid || r.ErrorCode != nop.ErrTaskDagInvalid {
		t.Fatalf("expected dup invalid: %+v", r)
	}
}

func TestDag_TooLarge(t *testing.T) {
	var nodes []*nop.DagNode
	for i := 0; i <= nop.MaxDagNodes; i++ {
		id := "n" + strings.Repeat("x", 1) + string(rune('a'+i%26)) + string(rune('0'+i/26))
		nodes = append(nodes, &nop.DagNode{ID: id, Action: "x", Agent: id})
	}
	r := nop.ValidateDag(&nop.TaskDag{Nodes: nodes})
	if r.IsValid || r.ErrorCode != nop.ErrTaskDagTooLarge {
		t.Fatalf("expected too-large: %+v", r)
	}
}

func TestDag_UnknownEdge(t *testing.T) {
	dag := &nop.TaskDag{
		Nodes: []*nop.DagNode{{ID: "a", Action: "x", Agent: "a"}},
		Edges: []*nop.DagEdge{{From: "a", To: "ghost"}},
	}
	r := nop.ValidateDag(dag)
	if r.IsValid || r.ErrorCode != nop.ErrTaskDagInvalid {
		t.Fatalf("expected unknown-edge invalid: %+v", r)
	}
}

func TestDag_Empty(t *testing.T) {
	r := nop.ValidateDag(&nop.TaskDag{})
	if r.IsValid || r.ErrorCode != nop.ErrTaskDagInvalid {
		t.Fatalf("expected empty invalid: %+v", r)
	}
}

// ── Callback URL validation ──────────────────────────────────────────────────────

func TestCallbackURL_Validation(t *testing.T) {
	if nop.ValidateCallbackURL("https://example.com/hook") != "" {
		t.Error("valid https should pass")
	}
	if nop.ValidateCallbackURL("http://example.com/hook") == "" {
		t.Error("http should be rejected")
	}
	if nop.ValidateCallbackURL("https://localhost/hook") == "" {
		t.Error("localhost should be rejected (SSRF)")
	}
	if nop.ValidateCallbackURL("https://127.0.0.1/hook") == "" {
		t.Error("loopback IP should be rejected")
	}
	if nop.ValidateCallbackURL("https://10.0.0.5/hook") == "" {
		t.Error("private IP should be rejected")
	}
	if nop.ValidateCallbackURL("") == "" {
		t.Error("empty should be rejected")
	}
}
