// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeExecutor records the driver-ready SQL/args and returns canned rows.
type fakeExecutor struct {
	lastQuery string
	lastArgs  []any
	rows      []MemoryNodeRow
	scalar    int64
}

func (f *fakeExecutor) QueryRows(_ context.Context, query string, args []any) ([]MemoryNodeRow, error) {
	f.lastQuery = query
	f.lastArgs = args
	return f.rows, nil
}

func (f *fakeExecutor) QueryScalar(_ context.Context, query string, args []any) (int64, error) {
	f.lastQuery = query
	f.lastArgs = args
	return f.scalar, nil
}

func provSchema() SqlMemoryNodeSchema {
	return SqlMemoryNodeSchema{
		TableName:  "products",
		PrimaryKey: "id",
		Fields: []SqlMemoryNodeField{
			{Name: "id", Type: "number"},
			{Name: "name", Type: "string"},
			{Name: "price", Type: "number"},
		},
	}
}

func TestSqlProvider_BuildQuerySQL_Pg(t *testing.T) {
	p := NewPostgreSqlMemoryNodeProvider(provSchema(), &fakeExecutor{}, SqlQueryOptions{DefaultLimit: 20, MaxLimit: 100})
	frame := &QueryFrame{Filter: parseFilterMap(`{"price":{"$gt":10}}`)}
	sql, params, err := p.BuildQuerySQL(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `"price" > @p0`) || !strings.Contains(sql, "LIMIT @_limit OFFSET @_offset") {
		t.Fatalf("sql=%q", sql)
	}
	if param(params, "p0") != int64(10) {
		t.Fatalf("p0=%v", param(params, "p0"))
	}
}

func TestSqlProvider_Query_RebindsToPositional_Pg(t *testing.T) {
	exec := &fakeExecutor{rows: []MemoryNodeRow{{"id": int64(1)}}}
	p := NewPostgreSqlMemoryNodeProvider(provSchema(), exec, SqlQueryOptions{DefaultLimit: 20, MaxLimit: 100})
	frame := &QueryFrame{Filter: parseFilterMap(`{"price":{"$gt":10}}`)}
	res, err := p.Query(context.Background(), frame, MemoryNodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("rows=%d", len(res.Rows))
	}
	// $gt param becomes $1, _limit $2, _offset $3 in Postgres style.
	if !strings.Contains(exec.lastQuery, "> $1") {
		t.Fatalf("query=%q", exec.lastQuery)
	}
	if len(exec.lastArgs) != 3 || exec.lastArgs[0] != int64(10) {
		t.Fatalf("args=%v", exec.lastArgs)
	}
}

func TestSqlProvider_Query_RebindsToPositional_SqlServer(t *testing.T) {
	exec := &fakeExecutor{}
	p := NewSqlServerMemoryNodeProvider(provSchema(), exec, SqlQueryOptions{DefaultLimit: 20, MaxLimit: 100})
	frame := &QueryFrame{Filter: parseFilterMap(`{"name":{"$eq":"x"}}`)}
	if _, err := p.Query(context.Background(), frame, MemoryNodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exec.lastQuery, "= ?") {
		t.Fatalf("query=%q", exec.lastQuery)
	}
}

func TestSqlProvider_Query_InClause_ExpandsPlaceholders(t *testing.T) {
	exec := &fakeExecutor{}
	p := NewPostgreSqlMemoryNodeProvider(provSchema(), exec, SqlQueryOptions{DefaultLimit: 20, MaxLimit: 100})
	frame := &QueryFrame{Filter: parseFilterMap(`{"id":{"$in":[1,2,3]}}`)}
	if _, err := p.Query(context.Background(), frame, MemoryNodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exec.lastQuery, "IN ($1, $2, $3)") {
		t.Fatalf("query=%q", exec.lastQuery)
	}
	// 3 IN args + limit + offset
	if len(exec.lastArgs) != 5 {
		t.Fatalf("args=%v", exec.lastArgs)
	}
}

func TestSqlProvider_NextCursor_WhenFull(t *testing.T) {
	rows := make([]MemoryNodeRow, 20)
	for i := range rows {
		rows[i] = MemoryNodeRow{"id": int64(i)}
	}
	exec := &fakeExecutor{rows: rows}
	p := NewPostgreSqlMemoryNodeProvider(provSchema(), exec, SqlQueryOptions{DefaultLimit: 20, MaxLimit: 100})
	res, _ := p.Query(context.Background(), &QueryFrame{}, MemoryNodeOptions{})
	if res.NextCursor == "" {
		t.Fatal("want next cursor when page is full")
	}
	if DecodeCursor(res.NextCursor) != 20 {
		t.Fatalf("cursor offset=%d", DecodeCursor(res.NextCursor))
	}
}

func TestSqlProvider_Count(t *testing.T) {
	exec := &fakeExecutor{scalar: 42}
	p := NewPostgreSqlMemoryNodeProvider(provSchema(), exec, SqlQueryOptions{})
	n, err := p.Count(context.Background(), &QueryFrame{})
	if err != nil || n != 42 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if !strings.HasPrefix(exec.lastQuery, "SELECT COUNT(*)") {
		t.Fatalf("query=%q", exec.lastQuery)
	}
}

func TestRebindNamedParams_LongestNameFirst(t *testing.T) {
	// Ensure @p10 is not clobbered by @p1.
	sql := "a = @p1 AND b = @p10"
	params := []NwpSqlParam{{Name: "p1", Value: 1}, {Name: "p10", Value: 10}}
	out, args := RebindNamedParams(sql, params, DialectPostgreSql)
	if out != "a = $1 AND b = $2" {
		t.Fatalf("out=%q", out)
	}
	if args[0] != 1 || args[1] != 10 {
		t.Fatalf("args=%v", args)
	}
}

func parseFilterMap(s string) map[string]any {
	var v map[string]any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	_ = dec.Decode(&v)
	return v
}
