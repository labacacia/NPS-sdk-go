// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"encoding/json"
	"fmt"
)

// Aggregate combines results using the given strategy (NPS-5 §3.3.2).
// minRequired is only used by the fastest_k strategy.
func Aggregate(strategy string, results []json.RawMessage, minRequired int) json.RawMessage {
	if len(results) == 0 {
		return json.RawMessage("{}")
	}

	switch strategy {
	case AggregateStrategyFirst:
		return results[0]
	case AggregateStrategyAll:
		return buildArray(results)
	case AggregateStrategyFastestK:
		k := minRequired
		if k <= 0 || k > len(results) {
			k = len(results)
		}
		return buildArray(results[:k])
	default: // "merge" and default
		return mergeResults(results)
	}
}

// mergeResults merges all JSON object results into one (last-write-wins on key
// conflicts). Non-object results are added under "_result_{i}" keys.
func mergeResults(results []json.RawMessage) json.RawMessage {
	merged := map[string]json.RawMessage{}
	for i, r := range results {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(r, &obj); err == nil {
			for k, v := range obj {
				merged[k] = v
			}
		} else {
			merged[fmt.Sprintf("_result_%d", i)] = r
		}
	}
	out, _ := json.Marshal(merged)
	return out
}

func buildArray(results []json.RawMessage) json.RawMessage {
	arr := make([]json.RawMessage, len(results))
	copy(arr, results)
	out, _ := json.Marshal(arr)
	return out
}

// AggregateEndNodes filters allResults to only the given end-node IDs (preserving
// their order), then aggregates using strategy.
func AggregateEndNodes(endNodeIDs []string, allResults map[string]json.RawMessage, strategy string) json.RawMessage {
	var endResults []json.RawMessage
	for _, id := range endNodeIDs {
		if r, ok := allResults[id]; ok {
			endResults = append(endResults, r)
		}
	}
	return Aggregate(strategy, endResults, 0)
}
