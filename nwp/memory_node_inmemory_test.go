// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nwp

import (
	"context"
	"testing"
)

func imRows() []MemoryNodeRow {
	return []MemoryNodeRow{
		{"id": "1", "name": "alice", "age": float64(30), "city": "sydney"},
		{"id": "2", "name": "bob", "age": float64(25), "city": "sydney"},
		{"id": "3", "name": "carol", "age": float64(40), "city": "perth"},
		{"id": "4", "name": "dave", "age": float64(35), "city": "perth"},
	}
}

func imQuery(t *testing.T, p *InMemoryMemoryNodeProvider, f *QueryFrame, opts MemoryNodeOptions) []MemoryNodeRow {
	t.Helper()
	res, err := p.Query(context.Background(), f, opts)
	if err != nil {
		t.Fatal(err)
	}
	return res.Rows
}

func TestInMemoryProviderEqualityFilter(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	rows := imQuery(t, p, &QueryFrame{Filter: map[string]any{"city": "perth"}}, MemoryNodeOptions{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 perth rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r["city"] != "perth" {
			t.Fatalf("bad row %+v", r)
		}
	}
}

func TestInMemoryProviderComparisonOperators(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	rows := imQuery(t, p, &QueryFrame{
		Filter: map[string]any{"age": map[string]any{"gte": float64(35)}},
	}, MemoryNodeOptions{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows age>=35, got %d (%+v)", len(rows), rows)
	}
}

func TestInMemoryProviderInOperator(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	rows := imQuery(t, p, &QueryFrame{
		Filter: map[string]any{"id": map[string]any{"in": []any{"1", "3"}}},
	}, MemoryNodeOptions{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for in, got %d", len(rows))
	}
}

func TestInMemoryProviderAndOr(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	// city == perth AND age < 40  => only dave (35).
	rows := imQuery(t, p, &QueryFrame{
		Filter: map[string]any{"and": []any{
			map[string]any{"city": "perth"},
			map[string]any{"age": map[string]any{"lt": float64(40)}},
		}},
	}, MemoryNodeOptions{})
	if len(rows) != 1 || rows[0]["name"] != "dave" {
		t.Fatalf("and filter %+v", rows)
	}

	// name == alice OR name == bob.
	rows = imQuery(t, p, &QueryFrame{
		Filter: map[string]any{"or": []any{
			map[string]any{"name": "alice"},
			map[string]any{"name": "bob"},
		}},
	}, MemoryNodeOptions{})
	if len(rows) != 2 {
		t.Fatalf("or filter %+v", rows)
	}
}

func TestInMemoryProviderOrderOffsetLimit(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	offset := uint64(1)
	limit := uint64(2)
	rows := imQuery(t, p, &QueryFrame{
		Order:  map[string]any{"field": "age", "dir": "desc"},
		Offset: &offset,
		Limit:  &limit,
	}, MemoryNodeOptions{DefaultLimit: 20})
	// ages desc: 40,35,30,25 -> offset 1, limit 2 -> 35,30
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["age"] != float64(35) || rows[1]["age"] != float64(30) {
		t.Fatalf("order/offset/limit wrong: %+v", rows)
	}
}

func TestInMemoryProviderFieldProjection(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	rows := imQuery(t, p, &QueryFrame{
		Filter: map[string]any{"id": "1"},
		Fields: []string{"id", "name"},
	}, MemoryNodeOptions{})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if len(r) != 2 || r["id"] != "1" || r["name"] != "alice" {
		t.Fatalf("projection wrong: %+v", r)
	}
	if _, has := r["age"]; has {
		t.Fatal("projected-out field present")
	}
}

func TestInMemoryProviderNilFilterMatchesAll(t *testing.T) {
	p := NewInMemoryMemoryNodeProvider(imRows())
	rows := imQuery(t, p, &QueryFrame{}, MemoryNodeOptions{})
	if len(rows) != 4 {
		t.Fatalf("nil filter should match all, got %d", len(rows))
	}
}
