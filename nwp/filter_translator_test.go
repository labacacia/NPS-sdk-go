// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"encoding/json"
	"strings"
	"testing"
)

var filterSchema = SqlMemoryNodeSchema{
	TableName:  "products",
	PrimaryKey: "id",
	Fields: []SqlMemoryNodeField{
		{Name: "id", Type: "number"},
		{Name: "name", Type: "string"},
		{Name: "price", Type: "number"},
		{Name: "active", Type: "boolean"},
		{Name: "category", Type: "string"},
	},
}

func parseFilter(t *testing.T, s string) any {
	t.Helper()
	var v any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func pg() *NwpFilterTranslator    { return NewNwpFilterTranslator(filterSchema, DialectPostgreSql) }
func mssql() *NwpFilterTranslator { return NewNwpFilterTranslator(filterSchema, DialectSqlServer) }

func paramValue(params []NwpSqlParam, name string) any {
	for _, p := range params {
		if p.Name == name {
			return p.Value
		}
	}
	return nil
}

func TestNullFilter_ReturnsEmpty(t *testing.T) {
	sql, params, err := pg().Translate(nil)
	if err != nil || sql != "" || len(params) != 0 {
		t.Fatalf("want empty, got sql=%q params=%v err=%v", sql, params, err)
	}
}

func TestEq_PostgreSql_QuotesCorrectly(t *testing.T) {
	sql, params, err := pg().Translate(parseFilter(t, `{"name":{"$eq":"widget"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if sql != `"name" = @p0` {
		t.Fatalf("sql=%q", sql)
	}
	if paramValue(params, "p0") != "widget" {
		t.Fatalf("p0=%v", paramValue(params, "p0"))
	}
}

func TestEq_SqlServer_UsesBrackets(t *testing.T) {
	sql, _, _ := mssql().Translate(parseFilter(t, `{"name":{"$eq":"widget"}}`))
	if sql != `[name] = @p0` {
		t.Fatalf("sql=%q", sql)
	}
}

func TestNe_ProducesNotEquals(t *testing.T) {
	sql, params, _ := pg().Translate(parseFilter(t, `{"price":{"$ne":0}}`))
	if !strings.Contains(sql, "<>") {
		t.Fatalf("sql=%q", sql)
	}
	if paramValue(params, "p0") != int64(0) {
		t.Fatalf("p0=%v (%T)", paramValue(params, "p0"), paramValue(params, "p0"))
	}
}

func TestComparisonOps(t *testing.T) {
	cases := map[string]string{"$lt": "<", "$lte": "<=", "$gt": ">", "$gte": ">="}
	for op, want := range cases {
		sql, _, _ := pg().Translate(parseFilter(t, `{"price":{"`+op+`":10}}`))
		if !strings.Contains(sql, want) {
			t.Fatalf("op %s: sql=%q want %q", op, sql, want)
		}
	}
}

func TestContains_WrapsWithPercent(t *testing.T) {
	sql, params, _ := pg().Translate(parseFilter(t, `{"name":{"$contains":"wid"}}`))
	if !strings.Contains(sql, "LIKE") {
		t.Fatalf("sql=%q", sql)
	}
	if paramValue(params, "p0") != "%wid%" {
		t.Fatalf("p0=%v", paramValue(params, "p0"))
	}
}

func TestIn_ProducesInClause(t *testing.T) {
	sql, params, _ := pg().Translate(parseFilter(t, `{"category":{"$in":["A","B"]}}`))
	if !strings.Contains(sql, "IN @p0") {
		t.Fatalf("sql=%q", sql)
	}
	vals, ok := paramValue(params, "p0").([]any)
	if !ok || len(vals) != 2 {
		t.Fatalf("p0=%v", paramValue(params, "p0"))
	}
}

func TestIn_EmptyArray_ReturnsFalse(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"category":{"$in":[]}}`))
	if sql != "1=0" {
		t.Fatalf("sql=%q", sql)
	}
}

func TestNin_EmptyArray_ReturnsTrue(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"category":{"$nin":[]}}`))
	if sql != "1=1" {
		t.Fatalf("sql=%q", sql)
	}
}

func TestNin_ProducesNotInClause(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"category":{"$nin":["X"]}}`))
	if !strings.Contains(sql, "NOT IN @p0") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBetween_ProducesBetweenClause(t *testing.T) {
	sql, params, _ := pg().Translate(parseFilter(t, `{"price":{"$between":[10,99]}}`))
	if !strings.Contains(sql, "BETWEEN @p0 AND @p1") {
		t.Fatalf("sql=%q", sql)
	}
	if paramValue(params, "p0") != int64(10) || paramValue(params, "p1") != int64(99) {
		t.Fatalf("p0=%v p1=%v", paramValue(params, "p0"), paramValue(params, "p1"))
	}
}

func TestBetween_WrongLength_Errors(t *testing.T) {
	_, _, err := pg().Translate(parseFilter(t, `{"price":{"$between":[10]}}`))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestAnd_JoinsWithAnd(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"$and":[{"name":{"$eq":"x"}},{"active":{"$eq":true}}]}`))
	if !strings.Contains(sql, " AND ") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestOr_JoinsWithOr(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"$or":[{"price":{"$lt":5}},{"price":{"$gt":100}}]}`))
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestNot_ProducesNotClause(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"$not":{"name":{"$eq":"x"}}}`))
	if !strings.HasPrefix(sql, "NOT (") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestMultiFieldObject_ImplicitAnd(t *testing.T) {
	sql, _, _ := pg().Translate(parseFilter(t, `{"name":{"$eq":"x"},"active":{"$eq":true}}`))
	if !strings.Contains(sql, "AND") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestUnknownField_Errors_FieldUnknown(t *testing.T) {
	_, _, err := pg().Translate(parseFilter(t, `{"ghost":{"$eq":1}}`))
	fe, ok := err.(*NwpFilterError)
	if !ok || fe.NwpErrorCode != ErrQueryFieldUnknown {
		t.Fatalf("err=%v", err)
	}
}

func TestUnknownOperator_Errors(t *testing.T) {
	_, _, err := pg().Translate(parseFilter(t, `{"name":{"$regex":".*"}}`))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestUnknownLogicalOp_Errors(t *testing.T) {
	_, _, err := pg().Translate(parseFilter(t, `{"$xyz":[{"name":{"$eq":"x"}}]}`))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestLogicalOp_NonArray_Errors(t *testing.T) {
	_, _, err := pg().Translate(parseFilter(t, `{"$and":{"name":{"$eq":"x"}}}`))
	if err == nil {
		t.Fatal("want error")
	}
}
