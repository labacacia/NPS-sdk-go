// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import "fmt"

// DagValidationResult is the result of DAG validation (mirror of .NET DagValidationResult).
type DagValidationResult struct {
	IsValid          bool
	ErrorCode        string
	ErrorMessage     string
	TopologicalOrder []string
}

func dagSuccess(order []string) *DagValidationResult {
	return &DagValidationResult{IsValid: true, TopologicalOrder: order}
}

func dagFailure(code, msg string) *DagValidationResult {
	return &DagValidationResult{IsValid: false, ErrorCode: code, ErrorMessage: msg}
}

// ValidateDag validates a TaskDag against NPS-5 §3.1.1 rules: acyclicity, node
// count limit, start/end node presence, edge consistency, and condition length.
// On success it returns a topological ordering (Kahn's algorithm).
func ValidateDag(dag *TaskDag) *DagValidationResult {
	if dag == nil || len(dag.Nodes) == 0 {
		return dagFailure(ErrTaskDagInvalid, "DAG must contain at least one node.")
	}

	if len(dag.Nodes) > MaxDagNodes {
		return dagFailure(ErrTaskDagTooLarge,
			fmt.Sprintf("DAG contains %d nodes, exceeding the maximum of %d.", len(dag.Nodes), MaxDagNodes))
	}

	nodeIDs := make(map[string]struct{}, len(dag.Nodes))
	for _, node := range dag.Nodes {
		if _, dup := nodeIDs[node.ID]; dup {
			return dagFailure(ErrTaskDagInvalid, fmt.Sprintf("Duplicate node ID: '%s'.", node.ID))
		}
		nodeIDs[node.ID] = struct{}{}
	}

	// Edges must reference existing nodes.
	adjacency := make(map[string][]string, len(nodeIDs))
	inDegree := make(map[string]int, len(nodeIDs))
	for id := range nodeIDs {
		adjacency[id] = nil
		inDegree[id] = 0
	}

	for _, edge := range dag.Edges {
		if _, ok := nodeIDs[edge.From]; !ok {
			return dagFailure(ErrTaskDagInvalid, fmt.Sprintf("Edge references unknown source node: '%s'.", edge.From))
		}
		if _, ok := nodeIDs[edge.To]; !ok {
			return dagFailure(ErrTaskDagInvalid, fmt.Sprintf("Edge references unknown target node: '%s'.", edge.To))
		}
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		inDegree[edge.To]++
	}

	// input_from references must be consistent with known nodes.
	for _, node := range dag.Nodes {
		for _, upstream := range node.InputFrom {
			if _, ok := nodeIDs[upstream]; !ok {
				return dagFailure(ErrTaskDagInvalid,
					fmt.Sprintf("Node '%s' references unknown upstream node '%s' in input_from.", node.ID, upstream))
			}
		}
	}

	// At least one start node (no incoming edges).
	hasStart := false
	for _, d := range inDegree {
		if d == 0 {
			hasStart = true
			break
		}
	}
	if !hasStart {
		return dagFailure(ErrTaskDagInvalid, "DAG must have at least one start node (no incoming edges).")
	}

	// At least one end node (no outgoing edges).
	hasEnd := false
	for _, list := range adjacency {
		if len(list) == 0 {
			hasEnd = true
			break
		}
	}
	if !hasEnd {
		return dagFailure(ErrTaskDagInvalid, "DAG must have at least one end node (no outgoing edges).")
	}

	// Condition expression length limit.
	for _, node := range dag.Nodes {
		if len(node.Condition) > MaxConditionLength {
			return dagFailure(ErrConditionEvalError,
				fmt.Sprintf("Node '%s' condition expression exceeds %d characters.", node.ID, MaxConditionLength))
		}
	}

	// Kahn's algorithm for topological sort + cycle detection.
	remaining := make(map[string]int, len(inDegree))
	for id, d := range inDegree {
		remaining[id] = d
	}
	var queue []string
	// Deterministic ordering: iterate nodes in declaration order for start seeds.
	for _, node := range dag.Nodes {
		if remaining[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}

	sorted := make([]string, 0, len(nodeIDs))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)
		for _, neighbor := range adjacency[current] {
			remaining[neighbor]--
			if remaining[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) != len(nodeIDs) {
		return dagFailure(ErrTaskDagCycle, "DAG contains a cycle.")
	}

	return dagSuccess(sorted)
}
