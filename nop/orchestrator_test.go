// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labacacia/NPS-sdk-go/nop"
)

// ── Mock worker client ────────────────────────────────────────────────────────

type nodeHandler func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame

type mockWorker struct {
	mu                        sync.Mutex
	handlers                  map[string]nodeHandler
	delays                    map[string]time.Duration
	preflightAvailable        bool
	preflightUnavailableAgent string
	preflightReason           string
}

func newMockWorker() *mockWorker {
	return &mockWorker{
		handlers:           map[string]nodeHandler{},
		delays:             map[string]time.Duration{},
		preflightAvailable: true,
	}
}

func (m *mockWorker) setupSuccess(nodeID, resultJSON string) {
	m.handlers[nodeID] = func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame {
		return []*nop.WorkerStreamFrame{{
			SubtaskID: req.SubtaskID, Seq: 0, IsFinal: true,
			SenderNID: nodeID, Data: json.RawMessage(resultJSON),
		}}
	}
}

func (m *mockWorker) setupSuccessDelay(nodeID, resultJSON string, d time.Duration) {
	m.setupSuccess(nodeID, resultJSON)
	m.delays[nodeID] = d
}

func (m *mockWorker) setupFailure(nodeID, code, msg string) {
	m.handlers[nodeID] = func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame {
		return []*nop.WorkerStreamFrame{{
			SubtaskID: req.SubtaskID, Seq: 0, IsFinal: true, SenderNID: nodeID,
			Error: &nop.StreamError{Code: code, Message: msg},
		}}
	}
}

func (m *mockWorker) setupHandler(nodeID string, h nodeHandler) { m.handlers[nodeID] = h }

func (m *mockWorker) Delegate(ctx context.Context, req *nop.DelegateRequest) (<-chan *nop.WorkerStreamFrame, error) {
	m.mu.Lock()
	h, ok := m.handlers[req.NodeID]
	delay := m.delays[req.NodeID]
	m.mu.Unlock()

	if !ok {
		h = func(r *nop.DelegateRequest) []*nop.WorkerStreamFrame {
			return []*nop.WorkerStreamFrame{{
				SubtaskID: r.SubtaskID, Seq: 0, IsFinal: true, SenderNID: r.NodeID,
				Data: json.RawMessage(`{"ok":true}`),
			}}
		}
	}

	ch := make(chan *nop.WorkerStreamFrame)
	go func() {
		defer close(ch)
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
		for _, f := range h(req) {
			select {
			case <-ctx.Done():
				return
			case ch <- f:
			}
		}
	}()
	return ch, nil
}

func (m *mockWorker) Preflight(ctx context.Context, agentNID, action string) (*nop.PreflightResult, error) {
	avail := m.preflightAvailable
	if m.preflightUnavailableAgent != "" && agentNID == m.preflightUnavailableAgent {
		avail = false
	}
	return &nop.PreflightResult{
		AgentNID:          agentNID,
		Available:         avail,
		UnavailableReason: m.preflightReason,
	}, nil
}

// ── Task builders ─────────────────────────────────────────────────────────────

func buildOrch(t *testing.T, configure func(*nop.OrchestratorOptions)) (*nop.NopOrchestrator, *mockWorker) {
	t.Helper()
	opts := nop.DefaultOrchestratorOptions()
	opts.ValidateSenderNID = false
	opts.EnableCallback = false
	opts.CallbackRetryBaseDelayMs = 0
	if configure != nil {
		configure(opts)
	}
	worker := newMockWorker()
	store := nop.NewInMemoryTaskStore()
	return nop.NewOrchestrator(worker, store, opts), worker
}

func single(id, condition string) *nop.OrchestratorTask {
	return &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{{ID: id, Action: "nwp://node/" + id, Agent: id, Condition: condition}},
			Edges: nil,
		},
	}
}

func linear(ids ...string) *nop.OrchestratorTask {
	nodes := make([]*nop.DagNode, len(ids))
	for i, id := range ids {
		n := &nop.DagNode{ID: id, Action: "nwp://node/" + id, Agent: id}
		if i > 0 {
			n.InputFrom = []string{ids[i-1]}
		}
		nodes[i] = n
	}
	var edges []*nop.DagEdge
	for i := 1; i < len(ids); i++ {
		edges = append(edges, &nop.DagEdge{From: ids[i-1], To: ids[i]})
	}
	return &nop.OrchestratorTask{TaskID: nopUUID(), Dag: &nop.TaskDag{Nodes: nodes, Edges: edges}}
}

var uuidCounter int64

func nopUUID() string {
	return "task-" + hex.EncodeToString([]byte{byte(atomic.AddInt64(&uuidCounter, 1))}) + "-" +
		time.Now().Format("150405.000000000")
}

func exec(t *testing.T, o *nop.NopOrchestrator, task *nop.OrchestratorTask) *nop.NopTaskResult {
	t.Helper()
	res, err := o.Execute(context.Background(), task)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	return res
}

// ── Happy paths ───────────────────────────────────────────────────────────────

func TestOrch_SingleNode_Succeeds(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{"value": 42}`)
	res := exec(t, o, single("a", ""))
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("state: %s (%s: %s)", res.FinalState, res.ErrorCode, res.ErrorMessage)
	}
	if _, ok := res.NodeResults["a"]; !ok {
		t.Fatal("missing node result a")
	}
}

func TestOrch_LinearChain(t *testing.T) {
	o, w := buildOrch(t, nil)
	for _, id := range []string{"fetch", "analyze", "report"} {
		w.setupSuccess(id, `{"step":"`+id+`"}`)
	}
	res := exec(t, o, linear("fetch", "analyze", "report"))
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("state: %s", res.FinalState)
	}
	if len(res.NodeResults) != 3 {
		t.Fatalf("expected 3 node results, got %d", len(res.NodeResults))
	}
}

func TestOrch_DiamondDag(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("start", `{"x":1}`)
	w.setupSuccess("left", `{"l":10}`)
	w.setupSuccess("right", `{"r":20}`)
	w.setupSuccess("end", `{"done":true}`)
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "start", Action: "nwp://x", Agent: "start"},
				{ID: "left", Action: "nwp://x", Agent: "left", InputFrom: []string{"start"}},
				{ID: "right", Action: "nwp://x", Agent: "right", InputFrom: []string{"start"}},
				{ID: "end", Action: "nwp://x", Agent: "end", InputFrom: []string{"left", "right"}},
			},
			Edges: []*nop.DagEdge{
				{From: "start", To: "left"}, {From: "start", To: "right"},
				{From: "left", To: "end"}, {From: "right", To: "end"},
			},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("state: %s", res.FinalState)
	}
	if len(res.NodeResults) != 4 {
		t.Fatalf("expected 4 node results, got %d", len(res.NodeResults))
	}
}

func TestOrch_AggregatedResultMerges(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{"field_a":"hello"}`)
	w.setupSuccess("b", `{"field_b":"world"}`)
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "a", Action: "nwp://x", Agent: "a"},
				{ID: "b", Action: "nwp://x", Agent: "b"},
			},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("state: %s", res.FinalState)
	}
	var agg map[string]any
	if err := json.Unmarshal(res.AggregatedResult, &agg); err != nil {
		t.Fatalf("aggregated unmarshal: %v", err)
	}
	if _, ok := agg["field_a"]; !ok {
		t.Error("aggregated missing field_a")
	}
	if _, ok := agg["field_b"]; !ok {
		t.Error("aggregated missing field_b")
	}
}

// ── Condition skip ────────────────────────────────────────────────────────────

func TestOrch_ConditionFalse_Skipped(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("fetch", `{"count":0}`)
	w.setupSuccess("report", `{"done":true}`)
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "fetch", Action: "nwp://x", Agent: "fetch"},
				{ID: "report", Action: "nwp://x", Agent: "report", InputFrom: []string{"fetch"}, Condition: "$.fetch.count > 0"},
			},
			Edges: []*nop.DagEdge{{From: "fetch", To: "report"}},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("state: %s", res.FinalState)
	}
	if _, ok := res.NodeResults["fetch"]; !ok {
		t.Error("fetch should be present")
	}
	if _, ok := res.NodeResults["report"]; ok {
		t.Error("report should be skipped")
	}
}

func TestOrch_ConditionTrue_Executes(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("fetch", `{"count":5}`)
	w.setupSuccess("report", `{"done":true}`)
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "fetch", Action: "nwp://x", Agent: "fetch"},
				{ID: "report", Action: "nwp://x", Agent: "report", InputFrom: []string{"fetch"}, Condition: "$.fetch.count > 0"},
			},
			Edges: []*nop.DagEdge{{From: "fetch", To: "report"}},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("state: %s", res.FinalState)
	}
	if _, ok := res.NodeResults["report"]; !ok {
		t.Error("report should be present")
	}
}

// ── Failure handling ──────────────────────────────────────────────────────────

func TestOrch_NodeFailure_TaskFails(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupFailure("fetch", nop.ErrDelegateRejected, "capacity exceeded")
	res := exec(t, o, single("fetch", ""))
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed, got %s", res.FinalState)
	}
	if res.ErrorCode == "" {
		t.Error("expected error code")
	}
}

func TestOrch_NodeFailure_PropagatesToDependent(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupFailure("fetch", nop.ErrDelegateRejected, "")
	w.setupSuccess("analyze", `{"ok":true}`)
	res := exec(t, o, linear("fetch", "analyze"))
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed, got %s", res.FinalState)
	}
}

// ── K-of-N ────────────────────────────────────────────────────────────────────

func TestOrch_KofN_ProceedsWhenKMet(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{"v":1}`)
	w.setupFailure("b", "X-FAIL", "")
	w.setupSuccess("c", `{"v":3}`)
	w.setupSuccess("end", `{"done":true}`)
	// end depends on a,b,c with min_required=2 → 2 of 3 succeed → end runs.
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "a", Action: "nwp://x", Agent: "a"},
				{ID: "b", Action: "nwp://x", Agent: "b"},
				{ID: "c", Action: "nwp://x", Agent: "c"},
				{ID: "end", Action: "nwp://x", Agent: "end", InputFrom: []string{"a", "b", "c"}, MinRequired: 2},
			},
			Edges: []*nop.DagEdge{
				{From: "a", To: "end"}, {From: "b", To: "end"}, {From: "c", To: "end"},
			},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("expected completed (K met), got %s (%s)", res.FinalState, res.ErrorCode)
	}
	if _, ok := res.NodeResults["end"]; !ok {
		t.Error("end node should have run")
	}
}

func TestOrch_KofN_FailsWhenKUnmet(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{"v":1}`)
	w.setupFailure("b", "X-FAIL", "")
	w.setupFailure("c", "X-FAIL", "")
	w.setupSuccess("end", `{"done":true}`)
	// min_required=2 but only a succeeds → end can never satisfy K → task fails.
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "a", Action: "nwp://x", Agent: "a"},
				{ID: "b", Action: "nwp://x", Agent: "b"},
				{ID: "c", Action: "nwp://x", Agent: "c"},
				{ID: "end", Action: "nwp://x", Agent: "end", InputFrom: []string{"a", "b", "c"}, MinRequired: 2},
			},
			Edges: []*nop.DagEdge{
				{From: "a", To: "end"}, {From: "b", To: "end"}, {From: "c", To: "end"},
			},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed (K unmet), got %s", res.FinalState)
	}
}

// ── Retry ─────────────────────────────────────────────────────────────────────

func TestOrch_Retry_SucceedsOnSecondAttempt(t *testing.T) {
	o, w := buildOrch(t, nil)
	var calls int32
	w.setupHandler("op", func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame {
		c := atomic.AddInt32(&calls, 1)
		if c == 1 {
			return []*nop.WorkerStreamFrame{{SubtaskID: req.SubtaskID, IsFinal: true, SenderNID: "op",
				Error: &nop.StreamError{Code: "ERR", Retryable: true}}}
		}
		return []*nop.WorkerStreamFrame{{SubtaskID: req.SubtaskID, IsFinal: true, SenderNID: "op",
			Data: json.RawMessage(`{"ok":true}`)}}
	})
	task := &nop.OrchestratorTask{
		TaskID:     nopUUID(),
		MaxRetries: 2,
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{{ID: "op", Action: "nwp://x", Agent: "op",
				RetryPolicy: &nop.RetryPolicy{MaxRetries: 2, InitialDelayMs: 1}}},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("expected completed, got %s (%s)", res.FinalState, res.ErrorCode)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestOrch_RetryOn_AllowlistRespected(t *testing.T) {
	o, w := buildOrch(t, nil)
	var calls int32
	w.setupHandler("op", func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame {
		atomic.AddInt32(&calls, 1)
		return []*nop.WorkerStreamFrame{{SubtaskID: req.SubtaskID, IsFinal: true, SenderNID: "op",
			Error: &nop.StreamError{Code: "NON-RETRYABLE"}}}
	})
	// RetryOn only lists a different code → no retry.
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{{ID: "op", Action: "nwp://x", Agent: "op",
				RetryPolicy: &nop.RetryPolicy{MaxRetries: 3, InitialDelayMs: 1, RetryOn: []string{"OTHER-CODE"}}}},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed, got %s", res.FinalState)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
}

// ── Saga compensation ─────────────────────────────────────────────────────────

func TestOrch_Saga_BestEffort_CompensatesOnFailure(t *testing.T) {
	o, w := buildOrch(t, nil)
	var refundCalls int32
	var refundParams atomic.Value
	w.setupHandler("charge", func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame {
		if req.Action == "nwp://payments/refund" {
			atomic.AddInt32(&refundCalls, 1)
			refundParams.Store(string(req.Params))
			return []*nop.WorkerStreamFrame{{SubtaskID: req.SubtaskID, IsFinal: true, SenderNID: "charge",
				Data: json.RawMessage(`{"refunded":true}`)}}
		}
		return []*nop.WorkerStreamFrame{{SubtaskID: req.SubtaskID, IsFinal: true, SenderNID: "charge",
			Data: json.RawMessage(`{"charge_id":"ch_1","amount":25}`)}}
	})
	w.setupFailure("ship", "SHIP-FAILED", "")

	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{
					ID: "charge", Action: "nwp://payments/charge", Agent: "charge",
					CompensateAction: "nwp://payments/refund",
					CompensateParamsMapping: map[string]json.RawMessage{
						"charge_id": json.RawMessage(`"$.charge.charge_id"`),
					},
				},
				{ID: "ship", Action: "nwp://shipping/ship", Agent: "ship", InputFrom: []string{"charge"}},
			},
			Edges: []*nop.DagEdge{{From: "charge", To: "ship"}},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed, got %s", res.FinalState)
	}
	if atomic.LoadInt32(&refundCalls) != 1 {
		t.Fatalf("expected 1 refund call, got %d", refundCalls)
	}
	if res.Compensation == nil || res.Compensation.Attempted != 1 || res.Compensation.Succeeded != 1 {
		t.Fatalf("compensation summary wrong: %+v", res.Compensation)
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(refundParams.Load().(string)), &p)
	if p["charge_id"] != "ch_1" {
		t.Errorf("refund charge_id mismatch: %v", p["charge_id"])
	}
}

func TestOrch_Saga_Strict_MissingCompensate_NotSupported(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("charge", `{"charge_id":"ch_1"}`)
	w.setupFailure("ship", "SHIP-FAILED", "")
	task := &nop.OrchestratorTask{
		TaskID:             nopUUID(),
		CompensationPolicy: nop.CompensationPolicyStrict,
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "charge", Action: "nwp://payments/charge", Agent: "charge"},
				{ID: "ship", Action: "nwp://shipping/ship", Agent: "ship", InputFrom: []string{"charge"}},
			},
			Edges: []*nop.DagEdge{{From: "charge", To: "ship"}},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed, got %s", res.FinalState)
	}
	if res.ErrorCode != nop.ErrCompensationNotSupported {
		t.Fatalf("expected %s, got %s", nop.ErrCompensationNotSupported, res.ErrorCode)
	}
	if res.Compensation == nil || res.Compensation.Attempted != 0 || res.Compensation.Failed != 1 {
		t.Fatalf("compensation summary wrong: %+v", res.Compensation)
	}
	if len(res.Compensation.FailedNodeIDs) != 1 || res.Compensation.FailedNodeIDs[0] != "charge" {
		t.Errorf("failed node ids: %v", res.Compensation.FailedNodeIDs)
	}
}

// ── DAG validation via Execute ─────────────────────────────────────────────────

func TestOrch_InvalidDag_Cycle(t *testing.T) {
	o, _ := buildOrch(t, nil)
	task := &nop.OrchestratorTask{
		TaskID: nopUUID(),
		Dag: &nop.TaskDag{
			Nodes: []*nop.DagNode{
				{ID: "s", Action: "nwp://x", Agent: "s"},
				{ID: "a", Action: "nwp://x", Agent: "a"},
				{ID: "b", Action: "nwp://x", Agent: "b"},
				{ID: "e", Action: "nwp://x", Agent: "e"},
			},
			Edges: []*nop.DagEdge{
				{From: "s", To: "a"}, {From: "a", To: "b"}, {From: "b", To: "a"}, {From: "a", To: "e"},
			},
		},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed || res.ErrorCode != nop.ErrTaskDagCycle {
		t.Fatalf("expected cycle failure, got %s/%s", res.FinalState, res.ErrorCode)
	}
}

func TestOrch_DuplicateTaskID(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{}`)
	task := single("a", "")
	_ = exec(t, o, task)
	res := exec(t, o, task) // same TaskID
	if res.FinalState != nop.TaskStateFailed || res.ErrorCode != nop.ErrTaskAlreadyCompleted {
		t.Fatalf("expected already-completed failure, got %s/%s", res.FinalState, res.ErrorCode)
	}
}

func TestOrch_DelegateChainTooDeep(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{}`)
	task := single("a", "")
	task.DelegateDepth = nop.MaxDelegateChainDepth
	res := exec(t, o, task)
	if res.ErrorCode != nop.ErrDelegateChainTooDeep {
		t.Fatalf("expected chain-too-deep, got %s", res.ErrorCode)
	}
}

func TestOrch_InvalidCallbackURL(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{}`)
	task := single("a", "")
	task.CallbackURL = "http://localhost/hook" // not https + private
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed || res.ErrorCode != nop.ErrTaskDagInvalid {
		t.Fatalf("expected callback-url rejection, got %s/%s", res.FinalState, res.ErrorCode)
	}
}

// ── Status query ──────────────────────────────────────────────────────────────

func TestOrch_GetStatus(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{}`)
	task := single("a", "")
	_ = exec(t, o, task)
	rec, _ := o.GetStatus(context.Background(), task.TaskID)
	if rec == nil || rec.TaskID != task.TaskID || rec.State != nop.TaskStateCompleted {
		t.Fatalf("unexpected record: %+v", rec)
	}
	unknown, _ := o.GetStatus(context.Background(), "no-such-id")
	if unknown != nil {
		t.Error("expected nil for unknown task")
	}
}

// ── Timeout ───────────────────────────────────────────────────────────────────

func TestOrch_Timeout(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccessDelay("slow", `{}`, 5*time.Second)
	task := &nop.OrchestratorTask{
		TaskID:    nopUUID(),
		TimeoutMs: 50,
		Dag:       &nop.TaskDag{Nodes: []*nop.DagNode{{ID: "slow", Action: "nwp://x", Agent: "slow"}}},
	}
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed || res.ErrorCode != nop.ErrTaskTimeout {
		t.Fatalf("expected timeout, got %s/%s", res.FinalState, res.ErrorCode)
	}
}

// ── Preflight ─────────────────────────────────────────────────────────────────

func TestOrch_Preflight_UnavailableFails(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{}`)
	w.preflightUnavailableAgent = "a"
	w.preflightReason = "busy"
	task := single("a", "")
	task.Preflight = true
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateFailed || res.ErrorCode != nop.ErrResourceInsufficient {
		t.Fatalf("expected resource-insufficient, got %s/%s", res.FinalState, res.ErrorCode)
	}
}

func TestOrch_Preflight_AvailablePasses(t *testing.T) {
	o, w := buildOrch(t, nil)
	w.setupSuccess("a", `{"v":1}`)
	task := single("a", "")
	task.Preflight = true
	res := exec(t, o, task)
	if res.FinalState != nop.TaskStateCompleted {
		t.Fatalf("expected completed, got %s", res.FinalState)
	}
}

// ── Sender NID validation ──────────────────────────────────────────────────────

func TestOrch_SenderNidMismatch(t *testing.T) {
	o, w := buildOrch(t, func(opt *nop.OrchestratorOptions) { opt.ValidateSenderNID = true })
	w.setupHandler("a", func(req *nop.DelegateRequest) []*nop.WorkerStreamFrame {
		return []*nop.WorkerStreamFrame{{SubtaskID: req.SubtaskID, IsFinal: true,
			SenderNID: "wrong-nid", Data: json.RawMessage(`{}`)}}
	})
	res := exec(t, o, single("a", ""))
	if res.FinalState != nop.TaskStateFailed {
		t.Fatalf("expected failed, got %s", res.FinalState)
	}
}

// ── Callback signature + delivery via httptest ─────────────────────────────────

func TestOrch_Callback_HMACSignature(t *testing.T) {
	// 32-byte key, base64url encoded.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	secret := base64.RawURLEncoding.EncodeToString(key)

	var gotSig string
	var gotBody []byte
	done := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(wr http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-NPS-Signature")
		gotBody, _ = readAll(r)
		wr.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	opts := nop.DefaultOrchestratorOptions()
	opts.ValidateSenderNID = false
	opts.EnableCallback = true
	opts.CallbackRetryBaseDelayMs = 0
	worker := newMockWorker()
	worker.setupSuccess("a", `{"v":1}`)
	o := nop.NewOrchestrator(worker, nop.NewInMemoryTaskStore(), opts)
	// Use the test server's TLS client so https to the httptest server works.
	o.SetHTTPClient(srv.Client())

	// Independent signature check (deterministic, no network dependency).
	payload := []byte(`{"task_id":"t"}`)
	want := hmacHex(key, payload)
	if got := nop.BuildCallbackSignatureForTest(secret, payload); got != "sha256="+want {
		t.Fatalf("signature mismatch: got %s want sha256=%s", got, want)
	}

	// Delivery test: private-host SSRF guard blocks 127.0.0.1 in Execute, so exercise the
	// callback path directly.
	nop.FireCallbackForTest(o, srv.URL, secret, &nop.NopTaskResult{TaskID: "t", FinalState: nop.TaskStateCompleted})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback not delivered")
	}
	if gotSig == "" {
		t.Error("missing X-NPS-Signature header")
	}
	// Recompute signature over the actually-delivered body.
	if gotSig != "sha256="+hmacHex(key, gotBody) {
		t.Errorf("delivered signature mismatch: %s", gotSig)
	}
}

func TestOrch_Callback_NoSecret_NoSignature(t *testing.T) {
	sig := nop.BuildCallbackSignatureForTest("", []byte(`{}`))
	if sig != "" {
		t.Errorf("expected empty signature for empty secret, got %s", sig)
	}
	// Non-32-byte key → no signature.
	shortSecret := base64.RawURLEncoding.EncodeToString([]byte("short"))
	if s := nop.BuildCallbackSignatureForTest(shortSecret, []byte(`{}`)); s != "" {
		t.Errorf("expected empty signature for short key, got %s", s)
	}
}

func hmacHex(key, payload []byte) string {
	h := hmacNew(key)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func hmacNew(key []byte) hash.Hash { return hmac.New(sha256.New, key) }

func readAll(r *http.Request) ([]byte, error) { return io.ReadAll(r.Body) }
