// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// In-memory Memory Node provider with in-memory filtering, for tests and small
// deployments. SQL-backed providers and the filter→SQL translator are DEFERRED;
// this keeps the IMemoryNodeProvider interface exercised end-to-end.

package nwp

import (
	"context"
	"reflect"
	"sort"
)

// InMemoryMemoryNodeProvider backs a Memory Node from an in-memory row slice.
// It supports equality/comparison filtering, ordering, offset and limit in
// process — no SQL. Rows are field-name → value maps.
type InMemoryMemoryNodeProvider struct {
	Rows []MemoryNodeRow
}

// NewInMemoryMemoryNodeProvider returns a provider over the given rows.
func NewInMemoryMemoryNodeProvider(rows []MemoryNodeRow) *InMemoryMemoryNodeProvider {
	return &InMemoryMemoryNodeProvider{Rows: rows}
}

// Query applies the frame's filter, order, offset and limit in memory.
func (p *InMemoryMemoryNodeProvider) Query(_ context.Context, frame *QueryFrame, opts MemoryNodeOptions) (*MemoryNodeQueryResult, error) {
	filtered := make([]MemoryNodeRow, 0, len(p.Rows))
	for _, row := range p.Rows {
		if matchFilter(row, frame.Filter) {
			filtered = append(filtered, row)
		}
	}

	applyOrder(filtered, frame.Order)

	offset := 0
	if frame.Offset != nil {
		offset = int(*frame.Offset)
	}
	if offset > len(filtered) {
		offset = len(filtered)
	}
	filtered = filtered[offset:]

	limit := int(opts.DefaultLimit)
	if frame.Limit != nil {
		limit = int(*frame.Limit)
	}
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	// Field projection.
	if len(frame.Fields) > 0 {
		projected := make([]MemoryNodeRow, len(filtered))
		for i, row := range filtered {
			pr := MemoryNodeRow{}
			for _, f := range frame.Fields {
				if v, ok := row[f]; ok {
					pr[f] = v
				}
			}
			projected[i] = pr
		}
		filtered = projected
	}

	return &MemoryNodeQueryResult{Rows: filtered}, nil
}

// matchFilter evaluates a filter against a row. Supported forms:
//   - nil                                → match all
//   - {field: value}                     → equality
//   - {field: {"eq"|"ne"|"gt"|"gte"|"lt"|"lte"|"in": value}} → operator
//   - {"and": [ ... ]} / {"or": [ ... ]} → boolean composition
func matchFilter(row MemoryNodeRow, filter any) bool {
	if filter == nil {
		return true
	}
	m, ok := filter.(map[string]any)
	if !ok {
		return true
	}
	for key, cond := range m {
		switch key {
		case "and":
			for _, sub := range toSlice(cond) {
				if !matchFilter(row, sub) {
					return false
				}
			}
		case "or":
			subs := toSlice(cond)
			if len(subs) == 0 {
				continue
			}
			anyMatch := false
			for _, sub := range subs {
				if matchFilter(row, sub) {
					anyMatch = true
					break
				}
			}
			if !anyMatch {
				return false
			}
		default:
			if !matchField(row[key], cond) {
				return false
			}
		}
	}
	return true
}

func matchField(actual, cond any) bool {
	ops, ok := cond.(map[string]any)
	if !ok {
		return valuesEqual(actual, cond)
	}
	for op, want := range ops {
		switch op {
		case "eq":
			if !valuesEqual(actual, want) {
				return false
			}
		case "ne":
			if valuesEqual(actual, want) {
				return false
			}
		case "gt":
			if !(compareValues(actual, want) > 0) {
				return false
			}
		case "gte":
			if !(compareValues(actual, want) >= 0) {
				return false
			}
		case "lt":
			if !(compareValues(actual, want) < 0) {
				return false
			}
		case "lte":
			if !(compareValues(actual, want) <= 0) {
				return false
			}
		case "in":
			found := false
			for _, v := range toSlice(want) {
				if valuesEqual(actual, v) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		default:
			// Unknown operator → treat the whole map as an equality value.
			return valuesEqual(actual, cond)
		}
	}
	return true
}

func applyOrder(rows []MemoryNodeRow, order any) {
	clauses := parseOrder(order)
	if len(clauses) == 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		for _, c := range clauses {
			cmp := compareValues(rows[i][c.field], rows[j][c.field])
			if cmp == 0 {
				continue
			}
			if c.desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
}

type orderClause struct {
	field string
	desc  bool
}

func parseOrder(order any) []orderClause {
	var out []orderClause
	add := func(m map[string]any) {
		field, _ := m["field"].(string)
		if field == "" {
			return
		}
		dir, _ := m["dir"].(string)
		out = append(out, orderClause{field: field, desc: dir == "desc"})
	}
	switch v := order.(type) {
	case map[string]any:
		add(v)
	case []any:
		for _, e := range v {
			if m, ok := e.(map[string]any); ok {
				add(m)
			}
		}
	}
	return out
}

func toSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func valuesEqual(a, b any) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			return af == bf
		}
	}
	return reflect.DeepEqual(a, b)
}

// compareValues returns -1, 0 or 1. Numbers compare numerically, strings
// lexicographically; mismatched/unsupported types compare as equal.
func compareValues(a, b any) int {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		switch {
		case as < bs:
			return -1
		case as > bs:
			return 1
		default:
			return 0
		}
	}
	return 0
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}
