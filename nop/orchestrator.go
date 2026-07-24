// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Orchestrator is the contract for the core NOP orchestrator (NPS-5 §3, §5).
type Orchestrator interface {
	// Execute runs the full task lifecycle: validate → (preflight) → run DAG →
	// aggregate → (callback). Blocks until the task reaches a terminal state.
	Execute(ctx context.Context, task *OrchestratorTask) (*NopTaskResult, error)
	// Cancel requests cancellation of a running task.
	Cancel(ctx context.Context, taskID string) error
	// GetStatus returns the current task record, or nil if not found.
	GetStatus(ctx context.Context, taskID string) (*NopTaskRecord, error)
}

// NopOrchestrator dispatches a task's DAG to Worker Agents, handling retries,
// condition-based skipping, K-of-N, saga compensation and result aggregation.
type NopOrchestrator struct {
	worker WorkerClient
	store  TaskStore
	opts   *OrchestratorOptions
	http   *http.Client

	ctsMu   sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewOrchestrator creates a NopOrchestrator. opts may be nil for defaults.
func NewOrchestrator(worker WorkerClient, store TaskStore, opts *OrchestratorOptions) *NopOrchestrator {
	return &NopOrchestrator{
		worker:  worker,
		store:   store,
		opts:    opts.normalized(),
		http:    &http.Client{},
		cancels: make(map[string]context.CancelFunc),
	}
}

var _ Orchestrator = (*NopOrchestrator)(nil)

// SetHTTPClient overrides the HTTP client used for callback delivery. Useful for
// tests (e.g. an httptest TLS server's client) and custom transports.
func (o *NopOrchestrator) SetHTTPClient(c *http.Client) { o.http = c }

// nodeOutcome is the internal result of executing a single node.
type nodeOutcome struct {
	state        TaskState
	result       json.RawMessage
	errorCode    string
	errorMessage string
}

// Execute runs the task lifecycle (mirror of .NET ExecuteAsync).
func (o *NopOrchestrator) Execute(ctx context.Context, task *OrchestratorTask) (*NopTaskResult, error) {
	// 1a. Delegation chain depth.
	if task.DelegateDepth >= MaxDelegateChainDepth {
		return newFailureResult(task.TaskID, ErrDelegateChainTooDeep,
			fmt.Sprintf("Delegation chain depth %d exceeds the maximum of %d.", task.DelegateDepth, MaxDelegateChainDepth), nil), nil
	}

	// 1b. callback_url validation.
	if task.CallbackURL != "" {
		if urlErr := ValidateCallbackURL(task.CallbackURL); urlErr != "" {
			return newFailureResult(task.TaskID, ErrTaskDagInvalid, urlErr, nil), nil
		}
	}

	// 1c. DAG validation.
	validation := ValidateDag(task.Dag)
	if !validation.IsValid {
		return newFailureResult(task.TaskID, validation.ErrorCode, validation.ErrorMessage, nil), nil
	}

	// 2. Reject already-known tasks.
	if existing, _ := o.store.Get(ctx, task.TaskID); existing != nil {
		return newFailureResult(task.TaskID, ErrTaskAlreadyCompleted,
			fmt.Sprintf("Task '%s' already exists.", task.TaskID), nil), nil
	}

	// 3. Persist initial record.
	record := &NopTaskRecord{
		TaskID:    task.TaskID,
		Task:      task,
		State:     TaskStatePending,
		StartedAt: time.Now().UTC(),
		Subtasks:  make(map[string]*NopSubtaskRecord),
	}
	if err := o.store.Save(ctx, record); err != nil {
		return newFailureResult(task.TaskID, ErrTaskAlreadyCompleted, err.Error(), nil), nil
	}

	// 4. Timeout context = min(task.timeout_ms, MaxTimeoutMs).
	timeoutMs := task.effectiveTimeoutMs()
	if timeoutMs > MaxTimeoutMs {
		timeoutMs = MaxTimeoutMs
	}
	runCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()

	o.ctsMu.Lock()
	o.cancels[task.TaskID] = cancel
	o.ctsMu.Unlock()
	defer func() {
		o.ctsMu.Lock()
		delete(o.cancels, task.TaskID)
		o.ctsMu.Unlock()
	}()

	// 5. Optional preflight.
	if task.Preflight {
		_ = o.store.UpdateState(ctx, task.TaskID, TaskStatePreflight)
		if preflightFail := o.runPreflight(runCtx, task); preflightFail != "" {
			_ = o.store.UpdateState(ctx, task.TaskID, TaskStateFailed)
			return newFailureResult(task.TaskID, ErrResourceInsufficient, preflightFail, nil), nil
		}
	}

	_ = o.store.UpdateState(ctx, task.TaskID, TaskStateRunning)

	// 6. Execute DAG.
	startedAt := time.Now()
	result := o.runDag(runCtx, task, validation.TopologicalOrder)

	// Distinguish our own timeout/cancel from external (parent ctx) cancellation.
	// runCtx is cancelled by the timeout timer or an explicit Cancel() call while the
	// parent ctx is still live → surface a timeout failure (mirrors .NET
	// OperationCanceledException when !ct.IsCancellationRequested). We also treat a
	// non-completed result that took at least the timeout budget as a timeout, since
	// the node-level deadline (== task timeout by default) may race the timer.
	timedOut := runCtx.Err() != nil ||
		time.Since(startedAt) >= time.Duration(timeoutMs)*time.Millisecond
	if timedOut && ctx.Err() == nil && result.FinalState != TaskStateCompleted {
		_ = o.store.UpdateState(ctx, task.TaskID, TaskStateFailed)
		return newFailureResult(task.TaskID, ErrTaskTimeout,
			fmt.Sprintf("Task exceeded timeout of %dms.", timeoutMs), nil), nil
	}

	// 7. Finalize state.
	now := time.Now().UTC()
	record.CompletedAt = &now
	_ = o.store.UpdateState(ctx, task.TaskID, result.FinalState)

	// 8. Fire callback (fire-and-forget).
	if o.opts.EnableCallback && task.CallbackURL != "" {
		go o.fireCallback(task.CallbackURL, task.CallbackSecret, result)
	}

	return result, nil
}

// Cancel requests cancellation of a running task.
func (o *NopOrchestrator) Cancel(ctx context.Context, taskID string) error {
	o.ctsMu.Lock()
	if cancel, ok := o.cancels[taskID]; ok {
		cancel()
	}
	o.ctsMu.Unlock()
	return o.store.UpdateState(ctx, taskID, TaskStateCancelled)
}

// GetStatus returns the current task record.
func (o *NopOrchestrator) GetStatus(ctx context.Context, taskID string) (*NopTaskRecord, error) {
	return o.store.Get(ctx, taskID)
}

// ── DAG execution ─────────────────────────────────────────────────────────────

func (o *NopOrchestrator) runDag(ctx context.Context, task *OrchestratorTask, topoOrder []string) *NopTaskResult {
	allNodes := make(map[string]*DagNode, len(task.Dag.Nodes))
	for _, n := range task.Dag.Nodes {
		allNodes[n.ID] = n
	}

	nodeResults := make(map[string]json.RawMessage) // completed only
	nodeStates := make(map[string]TaskState)        // terminal state per node

	// End nodes = no outgoing edges.
	hasOutgoing := make(map[string]bool)
	for _, e := range task.Dag.Edges {
		hasOutgoing[e.From] = true
	}
	var endNodeIDs []string
	for _, n := range task.Dag.Nodes {
		if !hasOutgoing[n.ID] {
			endNodeIDs = append(endNodeIDs, n.ID)
		}
	}

	done := make(chan finishedMsg)
	inFlight := make(map[string]bool)

	for len(nodeStates) < len(allNodes) {
		if ctx.Err() != nil {
			return o.buildAbortResult(ctx, task, allNodes, topoOrder, nodeResults, nodeStates,
				ErrTaskCancelled, "Task cancelled.")
		}

		// Ready nodes: deps done, not yet started/finished.
		var readyNodes []*DagNode
		for _, n := range task.Dag.Nodes {
			if _, finished := nodeStates[n.ID]; finished {
				continue
			}
			if inFlight[n.ID] {
				continue
			}
			if areDepsDone(n, nodeStates) {
				readyNodes = append(readyNodes, n)
			}
		}

		// K-of-N: fail ready nodes that can never satisfy K.
		filtered := readyNodes[:0]
		for _, n := range readyNodes {
			if len(n.InputFrom) == 0 {
				filtered = append(filtered, n)
				continue
			}
			total := len(n.InputFrom)
			k := total
			if n.MinRequired > 0 {
				k = int(n.MinRequired)
			}
			success := countDeps(n.InputFrom, nodeStates, TaskStateCompleted, TaskStateSkipped)
			if success < k {
				nodeStates[n.ID] = TaskStateFailed
				_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
					TaskID: task.TaskID, NodeID: n.ID, SubtaskID: newUUID(),
					State: TaskStateFailed, ErrorCode: ErrSyncDependencyFailed,
					ErrorMessage: fmt.Sprintf("Only %d/%d required dependencies succeeded.", success, k),
				})
				continue
			}
			filtered = append(filtered, n)
		}
		readyNodes = filtered

		// Launch ready nodes up to MaxConcurrentNodes.
		for _, node := range readyNodes {
			if len(inFlight) >= o.opts.MaxConcurrentNodes {
				break
			}
			inFlight[node.ID] = true
			nodeCopy := node
			// Snapshot context for the node's input resolution.
			ctxSnapshot := cloneResults(nodeResults)
			go func() {
				outcome := o.executeNodeWithRetry(ctx, task, nodeCopy, ctxSnapshot)
				done <- finishedMsg{nodeID: nodeCopy.ID, outcome: outcome}
			}()
		}

		if len(inFlight) == 0 {
			break // stuck or finished
		}

		// Wait for the next completion.
		msg := <-done
		delete(inFlight, msg.nodeID)
		outcome := msg.outcome

		nodeStates[msg.nodeID] = outcome.state
		if outcome.state == TaskStateCompleted && outcome.result != nil {
			nodeResults[msg.nodeID] = outcome.result
		}

		// On failure, check whether an end node is now unrecoverable.
		if outcome.state == TaskStateFailed {
			mustAbort := false
			for _, e := range endNodeIDs {
				if canReachEndNode(e, msg.nodeID, task.Dag.Edges) &&
					!canEndNodeStillSucceed(e, allNodes, nodeStates) {
					mustAbort = true
					break
				}
			}
			if mustAbort {
				drainInFlight(inFlight, done)
				return o.buildAbortResult(ctx, task, allNodes, topoOrder, nodeResults, nodeStates,
					ErrSyncDependencyFailed, fmt.Sprintf("Node '%s' failed: %s", msg.nodeID, outcome.errorCode))
			}
		}
	}

	// All nodes done — check for end-node failures.
	var failedNodes []string
	for id, s := range nodeStates {
		if s == TaskStateFailed {
			failedNodes = append(failedNodes, id)
		}
	}
	endFailed := false
	for _, e := range endNodeIDs {
		if nodeStates[e] == TaskStateFailed {
			endFailed = true
			break
		}
	}
	if len(failedNodes) > 0 && endFailed {
		return o.buildAbortResult(ctx, task, allNodes, topoOrder, nodeResults, nodeStates,
			ErrSyncDependencyFailed, "End node(s) failed: "+joinStrings(failedNodes))
	}

	// Aggregate end-node results.
	aggregated := AggregateEndNodes(endNodeIDs, nodeResults, o.opts.DefaultAggregateStrategy)

	var successComp *SagaCompensationResult
	if CompensationRunsOnSuccess(task.effectiveCompensationPolicy()) {
		successComp = o.runSagaCompensation(ctx, task, allNodes, topoOrder, nodeResults, nodeStates)
	}

	return newSuccessResult(task.TaskID, aggregated, nodeResults, successComp)
}

// buildAbortResult runs failure compensation (if configured) and constructs a failure result.
func (o *NopOrchestrator) buildAbortResult(
	ctx context.Context, task *OrchestratorTask, allNodes map[string]*DagNode, topoOrder []string,
	nodeResults map[string]json.RawMessage, nodeStates map[string]TaskState,
	defaultCode, message string) *NopTaskResult {

	var comp *SagaCompensationResult
	if CompensationRunsOnFailure(task.effectiveCompensationPolicy()) {
		comp = o.runSagaCompensation(ctx, task, allNodes, topoOrder, nodeResults, nodeStates)
	}
	errorCode := compensationFailureErrorCode(task, comp)
	if errorCode == "" {
		errorCode = defaultCode
	}
	return newFailureResult(task.TaskID, errorCode, message, comp)
}

// ── Node execution + retry ──────────────────────────────────────────────────

func (o *NopOrchestrator) executeNodeWithRetry(
	ctx context.Context, task *OrchestratorTask, node *DagNode, context map[string]json.RawMessage) nodeOutcome {

	subtaskID := newUUID()
	idempotencyKey := newUUID()
	maxRetries := int(task.MaxRetries)
	if node.RetryPolicy != nil {
		maxRetries = int(node.RetryPolicy.MaxRetries)
	}

	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		if ctx.Err() != nil {
			return nodeOutcome{state: TaskStateFailed, errorCode: ErrTaskCancelled}
		}

		// Evaluate condition once, before the first attempt.
		if attempt == 1 && node.Condition != "" {
			ok, err := EvaluateCondition(node.Condition, context)
			if err != nil {
				_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
					TaskID: task.TaskID, NodeID: node.ID, SubtaskID: subtaskID,
					State: TaskStateFailed, ErrorCode: ErrConditionEvalError,
					ErrorMessage: err.Error(), Attempt: attempt,
				})
				return nodeOutcome{state: TaskStateFailed, errorCode: ErrConditionEvalError, errorMessage: err.Error()}
			}
			if !ok {
				_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
					TaskID: task.TaskID, NodeID: node.ID, SubtaskID: subtaskID, State: TaskStateSkipped, Attempt: attempt,
				})
				return nodeOutcome{state: TaskStateSkipped}
			}
		}

		_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
			TaskID: task.TaskID, NodeID: node.ID, SubtaskID: subtaskID, State: TaskStateRunning, Attempt: attempt,
		})

		outcome := o.executeNodeOnce(ctx, task, node, subtaskID, idempotencyKey, context)

		if outcome.state == TaskStateCompleted {
			_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
				TaskID: task.TaskID, NodeID: node.ID, SubtaskID: subtaskID,
				State: TaskStateCompleted, Result: outcome.result, Attempt: attempt,
			})
			return outcome
		}

		// Failed — retryable?
		if !shouldRetry(node.RetryPolicy, outcome.errorCode, attempt, maxRetries) {
			_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
				TaskID: task.TaskID, NodeID: node.ID, SubtaskID: subtaskID,
				State: TaskStateFailed, ErrorCode: outcome.errorCode, ErrorMessage: outcome.errorMessage, Attempt: attempt,
			})
			return outcome
		}

		var delayMs uint64 = 1000
		if node.RetryPolicy != nil {
			delayMs = node.RetryPolicy.ComputeDelayMs(attempt - 1)
		}
		select {
		case <-ctx.Done():
			return nodeOutcome{state: TaskStateFailed, errorCode: ErrTaskCancelled}
		case <-time.After(time.Duration(delayMs) * time.Millisecond):
		}
	}

	// Exhausted retries.
	_ = o.store.UpdateSubtask(ctx, SubtaskUpdate{
		TaskID: task.TaskID, NodeID: node.ID, SubtaskID: subtaskID,
		State: TaskStateFailed, ErrorCode: ErrDelegateTimeout,
		ErrorMessage: fmt.Sprintf("Node '%s' exhausted %d retries.", node.ID, maxRetries),
	})
	return nodeOutcome{state: TaskStateFailed, errorCode: ErrDelegateTimeout}
}

func (o *NopOrchestrator) executeNodeOnce(
	ctx context.Context, task *OrchestratorTask, node *DagNode,
	subtaskID, idempotencyKey string, context map[string]json.RawMessage) nodeOutcome {

	params, err := BuildParams(node.InputMapping, context)
	if err != nil {
		code := ErrInputMappingError
		if me, ok := err.(*MappingError); ok && me.ErrorCode != "" {
			code = me.ErrorCode
		}
		return nodeOutcome{state: TaskStateFailed, errorCode: code, errorMessage: err.Error()}
	}

	nodeTimeoutMs := task.effectiveTimeoutMs()
	if node.TimeoutMs != nil {
		nodeTimeoutMs = *node.TimeoutMs
	}
	if nodeTimeoutMs > MaxTimeoutMs {
		nodeTimeoutMs = MaxTimeoutMs
	}
	nodeCtx, nodeCancel := context2WithTimeout(ctx, nodeTimeoutMs)
	defer nodeCancel()

	deadline := time.Now().UTC().Add(time.Duration(nodeTimeoutMs) * time.Millisecond).Format(time.RFC3339Nano)

	req := &DelegateRequest{
		ParentTaskID:   task.TaskID,
		SubtaskID:      subtaskID,
		NodeID:         node.ID,
		TargetAgentNID: node.Agent,
		Action:         node.Action,
		Params:         params,
		DelegatedScope: json.RawMessage("{}"),
		DeadlineAt:     deadline,
		IdempotencyKey: idempotencyKey,
		Priority:       task.effectivePriority(),
		Context:        task.Context,
		DelegateDepth:  task.DelegateDepth + 1,
	}

	frames, err := o.worker.Delegate(nodeCtx, req)
	if err != nil {
		return nodeOutcome{state: TaskStateFailed, errorCode: ErrDelegateRejected, errorMessage: err.Error()}
	}

	var (
		finalResult json.RawMessage
		errorCode   string
		errorMsg    string
		lastSeq     uint64
		gotFinal    bool
	)

	for {
		select {
		case <-nodeCtx.Done():
			if ctx.Err() == nil {
				return nodeOutcome{state: TaskStateFailed, errorCode: ErrDelegateTimeout,
					errorMessage: fmt.Sprintf("Node '%s' timed out after %dms.", node.ID, nodeTimeoutMs)}
			}
			return nodeOutcome{state: TaskStateFailed, errorCode: ErrTaskCancelled}
		case frame, ok := <-frames:
			if !ok {
				if !gotFinal {
					return nodeOutcome{state: TaskStateFailed, errorCode: ErrDelegateTimeout,
						errorMessage: "Stream ended without final frame."}
				}
				if errorCode != "" {
					return nodeOutcome{state: TaskStateFailed, errorCode: errorCode, errorMessage: errorMsg}
				}
				return nodeOutcome{state: TaskStateCompleted, result: finalResult}
			}

			// Sequence gap check.
			if frame.Seq != lastSeq && frame.Seq != 0 {
				if frame.Seq != lastSeq+1 {
					return nodeOutcome{state: TaskStateFailed, errorCode: ErrStreamSeqGap}
				}
			}
			lastSeq = frame.Seq

			// Sender NID validation.
			if o.opts.ValidateSenderNID && frame.SenderNID != node.Agent {
				return nodeOutcome{state: TaskStateFailed, errorCode: ErrStreamNidMismatch}
			}

			if frame.IsFinal {
				gotFinal = true
				if frame.Error != nil {
					errorCode = frame.Error.Code
					errorMsg = frame.Error.Message
				} else {
					finalResult = frame.Data
				}
				// Continue draining until channel close (or break early).
				if frame.Error != nil {
					return nodeOutcome{state: TaskStateFailed, errorCode: errorCode, errorMessage: errorMsg}
				}
				return nodeOutcome{state: TaskStateCompleted, result: finalResult}
			}
		}
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func areDepsDone(node *DagNode, states map[string]TaskState) bool {
	if len(node.InputFrom) == 0 {
		return true
	}
	total := len(node.InputFrom)
	k := total
	if node.MinRequired > 0 {
		k = int(node.MinRequired)
	}
	success := countDeps(node.InputFrom, states, TaskStateCompleted, TaskStateSkipped)
	failed := countDeps(node.InputFrom, states, TaskStateFailed)
	if success >= k {
		return true // K already satisfied
	}
	if total-failed < k {
		return true // impossible to satisfy K
	}
	return false // still waiting
}

func countDeps(deps []string, states map[string]TaskState, match ...TaskState) int {
	n := 0
	for _, d := range deps {
		s, ok := states[d]
		if !ok {
			continue
		}
		for _, m := range match {
			if s == m {
				n++
				break
			}
		}
	}
	return n
}

func canEndNodeStillSucceed(endNodeID string, allNodes map[string]*DagNode, nodeStates map[string]TaskState) bool {
	node := allNodes[endNodeID]
	if len(node.InputFrom) == 0 {
		return false // reachable, no deps → can't recover
	}
	total := len(node.InputFrom)
	k := total
	if node.MinRequired > 0 {
		k = int(node.MinRequired)
	}
	failed := countDeps(node.InputFrom, nodeStates, TaskStateFailed)
	optimistic := total - failed
	return optimistic >= k
}

func canReachEndNode(endNodeID, failedNodeID string, edges []*DagEdge) bool {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	visited := make(map[string]bool)
	queue := []string{failedNodeID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == endNodeID {
			return true
		}
		if visited[cur] {
			continue
		}
		visited[cur] = true
		queue = append(queue, adj[cur]...)
	}
	return false
}

func (o *NopOrchestrator) runPreflight(ctx context.Context, task *OrchestratorTask) string {
	// Deduplicate by agent NID (one probe per unique agent).
	type agentActions struct {
		agent  string
		action string
	}
	seen := make(map[string]bool)
	var probes []agentActions
	for _, n := range task.Dag.Nodes {
		if seen[n.Agent] {
			continue
		}
		seen[n.Agent] = true
		probes = append(probes, agentActions{agent: n.Agent, action: n.Action})
	}

	type presult struct {
		res *PreflightResult
		err error
	}
	results := make(chan presult, len(probes))
	for _, p := range probes {
		p := p
		go func() {
			r, err := o.worker.Preflight(ctx, p.agent, p.action)
			results <- presult{res: r, err: err}
		}()
	}

	var firstUnavailable string
	var probeErr string
	for range probes {
		pr := <-results
		if pr.err != nil {
			if probeErr == "" {
				probeErr = fmt.Sprintf("Preflight probe failed: %v", pr.err)
			}
			continue
		}
		if pr.res != nil && !pr.res.Available && firstUnavailable == "" {
			reason := pr.res.UnavailableReason
			if reason == "" {
				reason = "no reason given"
			}
			firstUnavailable = fmt.Sprintf("Agent '%s' is unavailable: %s", pr.res.AgentNID, reason)
		}
	}
	if probeErr != "" {
		return probeErr
	}
	return firstUnavailable
}

func shouldRetry(policy *RetryPolicy, errorCode string, attempt, maxRetries int) bool {
	if attempt > maxRetries {
		return false
	}
	if policy != nil && len(policy.RetryOn) > 0 && errorCode != "" {
		for _, c := range policy.RetryOn {
			if c == errorCode {
				return true
			}
		}
		return false
	}
	return true
}

// finishedMsg is sent on the done channel when a node goroutine completes.
type finishedMsg struct {
	nodeID  string
	outcome nodeOutcome
}

func drainInFlight(inFlight map[string]bool, done <-chan finishedMsg) {
	for len(inFlight) > 0 {
		msg := <-done
		delete(inFlight, msg.nodeID)
	}
}

// ── Saga compensation ─────────────────────────────────────────────────────────

func (o *NopOrchestrator) runSagaCompensation(
	ctx context.Context, task *OrchestratorTask, allNodes map[string]*DagNode, topoOrder []string,
	nodeResults map[string]json.RawMessage, nodeStates map[string]TaskState) *SagaCompensationResult {

	// Completed nodes in reverse topo order.
	var completed []string
	for i := len(topoOrder) - 1; i >= 0; i-- {
		id := topoOrder[i]
		if s, ok := nodeStates[id]; ok && s == TaskStateCompleted {
			if _, exists := allNodes[id]; exists {
				completed = append(completed, id)
			}
		}
	}

	if CompensationIsStrict(task.effectiveCompensationPolicy()) {
		var missing []string
		for _, id := range completed {
			if allNodes[id].CompensateAction == "" {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			return &SagaCompensationResult{Attempted: 0, Succeeded: 0, Failed: len(missing), FailedNodeIDs: missing}
		}
	}

	var toCompensate []string
	for _, id := range completed {
		if allNodes[id].CompensateAction != "" {
			toCompensate = append(toCompensate, id)
		}
	}
	if len(toCompensate) == 0 {
		return &SagaCompensationResult{FailedNodeIDs: []string{}}
	}

	_ = o.store.UpdateState(ctx, task.TaskID, TaskStateCompensating)

	succeeded := 0
	var failedIDs []string
	for _, nodeID := range toCompensate {
		node := allNodes[nodeID]
		compNode := &DagNode{
			ID:           node.ID,
			Agent:        node.Agent,
			Action:       node.CompensateAction,
			InputMapping: node.CompensateParamsMapping,
			TimeoutMs:    node.TimeoutMs,
		}
		outcome := o.executeNodeOnce(ctx, task, compNode, newUUID(), newUUID(), nodeResults)
		if outcome.state == TaskStateCompleted {
			succeeded++
		} else {
			failedIDs = append(failedIDs, nodeID)
		}
	}

	if failedIDs == nil {
		failedIDs = []string{}
	}
	return &SagaCompensationResult{
		Attempted:     len(toCompensate),
		Succeeded:     succeeded,
		Failed:        len(failedIDs),
		FailedNodeIDs: failedIDs,
	}
}

func compensationFailureErrorCode(task *OrchestratorTask, comp *SagaCompensationResult) string {
	if !CompensationIsStrict(task.effectiveCompensationPolicy()) || comp == nil || comp.Failed == 0 {
		return ""
	}
	if comp.Attempted == 0 {
		return ErrCompensationNotSupported
	}
	return ErrCompensationFailed
}

// ── Callback ──────────────────────────────────────────────────────────────────

// callbackPayload is the snake_case JSON body POSTed to the callback URL.
type callbackPayload struct {
	TaskID           string                     `json:"task_id"`
	FinalState       string                     `json:"final_state"`
	AggregatedResult json.RawMessage            `json:"aggregated_result,omitempty"`
	ErrorCode        string                     `json:"error_code,omitempty"`
	ErrorMessage     string                     `json:"error_message,omitempty"`
	NodeResults      map[string]json.RawMessage `json:"node_results,omitempty"`
	Compensation     *SagaCompensationResult    `json:"compensation,omitempty"`
}

func (o *NopOrchestrator) fireCallback(callbackURL, callbackSecret string, result *NopTaskResult) {
	payload, _ := json.Marshal(callbackPayload{
		TaskID:           result.TaskID,
		FinalState:       string(result.FinalState),
		AggregatedResult: result.AggregatedResult,
		ErrorCode:        result.ErrorCode,
		ErrorMessage:     result.ErrorMessage,
		NodeResults:      result.NodeResults,
		Compensation:     result.Compensation,
	})
	signature := buildCallbackSignature(callbackSecret, payload)

	client := o.http
	if client == nil {
		client = &http.Client{}
	}
	client.Timeout = time.Duration(o.opts.CallbackTimeoutMs) * time.Millisecond

	for attempt := 1; attempt <= CallbackMaxRetries; attempt++ {
		req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			if signature != "" {
				req.Header.Set("X-NPS-Signature", signature)
			}
			resp, err := client.Do(req)
			if err == nil {
				code := resp.StatusCode
				resp.Body.Close()
				if code >= 200 && code < 300 {
					return
				}
			}
		}
		if attempt < CallbackMaxRetries && o.opts.CallbackRetryBaseDelayMs > 0 {
			delay := float64(o.opts.CallbackRetryBaseDelayMs) * pow2(attempt-1)
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
	}
}

// buildCallbackSignature returns "sha256=<lowerhex>" when callbackSecret is a
// valid base64url-encoded 32-byte HMAC key; otherwise the empty string.
func buildCallbackSignature(callbackSecret string, payload []byte) string {
	if callbackSecret == "" {
		return ""
	}
	key, ok := decodeBase64URL(callbackSecret)
	if !ok || len(key) != 32 {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func decodeBase64URL(value string) ([]byte, bool) {
	// Accept both raw and padded base64url.
	if b, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return b, true
	}
	if b, err := base64.URLEncoding.DecodeString(value); err == nil {
		return b, true
	}
	// Fall back to standard base64 (with URL char normalisation).
	norm := value
	if b, err := base64.StdEncoding.DecodeString(norm); err == nil {
		return b, true
	}
	return nil, false
}

// ── small utilities ────────────────────────────────────────────────────────────

func cloneResults(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// context2WithTimeout wraps context.WithTimeout to keep imports tidy.
func context2WithTimeout(parent context.Context, ms uint64) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(ms)*time.Millisecond)
}
