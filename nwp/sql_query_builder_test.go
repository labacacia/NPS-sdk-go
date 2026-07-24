// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nwp

import (
	"strings"
	"testing"
)

var builderSchema = SqlMemoryNodeSchema{
	TableName:  "products",
	PrimaryKey: "id",
	Fields: []SqlMemoryNodeField{
		{Name: "id", Type: "number"},
		{Name: "name", Type: "string"},
		{Name: "price", Type: "number"},
		{Name: "sku", Type: "string", ColumnName: "product_sku"},
	},
}

var builderOpts = SqlQueryOptions{DefaultLimit: 20, MaxLimit: 100}

func bpg() *SqlQueryBuilder  { return NewSqlQueryBuilder(builderSchema, DialectPostgreSql) }
func bsql() *SqlQueryBuilder { return NewSqlQueryBuilder(builderSchema, DialectSqlServer) }

func param(params []NwpSqlParam, name string) any {
	for _, p := range params {
		if p.Name == name {
			return p.Value
		}
	}
	return nil
}

func TestBuild_NoFields_SelectsAllSchemaColumns(t *testing.T) {
	sql, _, err := bpg().Build(nil, nil, nil, 0, "", builderOpts)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id"`, `"name"`, `"price"`, `"product_sku"`} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in %q", want, sql)
		}
	}
}

func TestBuild_ColumnAlias_AppliesAlias(t *testing.T) {
	sql, _, _ := bpg().Build([]string{"sku"}, nil, nil, 0, "", builderOpts)
	if !strings.Contains(sql, `"product_sku" AS "sku"`) {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuild_UnknownField_Errors(t *testing.T) {
	_, _, err := bpg().Build([]string{"ghost"}, nil, nil, 0, "", builderOpts)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestBuild_Pg_QuotesTableName(t *testing.T) {
	sql, _, _ := bpg().Build(nil, nil, nil, 0, "", builderOpts)
	if !strings.Contains(sql, `FROM "products"`) {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuild_SqlServer_BracketsTableName(t *testing.T) {
	sql, _, _ := bsql().Build(nil, nil, nil, 0, "", builderOpts)
	if !strings.Contains(sql, "FROM [products]") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuild_WithFilter_AppendsWhere(t *testing.T) {
	filter := parseFilter(t, `{"name":{"$eq":"widget"}}`)
	sql, params, _ := bpg().Build(nil, filter, nil, 0, "", builderOpts)
	if !strings.Contains(sql, "WHERE") {
		t.Fatalf("sql=%q", sql)
	}
	if param(params, "p0") != "widget" {
		t.Fatalf("p0=%v", param(params, "p0"))
	}
}

func TestBuild_NoFilter_NoWhere(t *testing.T) {
	sql, _, _ := bpg().Build(nil, nil, nil, 0, "", builderOpts)
	if strings.Contains(sql, "WHERE") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuild_NoOrder_DefaultsToPrimaryKey(t *testing.T) {
	sql, _, _ := bpg().Build(nil, nil, nil, 0, "", builderOpts)
	if !strings.Contains(sql, `ORDER BY "id"`) {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuild_ExplicitOrder_AppliesIt(t *testing.T) {
	order := []SqlOrderClause{{Field: "price", Dir: "DESC"}}
	sql, _, _ := bpg().Build(nil, nil, order, 0, "", builderOpts)
	if !strings.Contains(sql, `ORDER BY "price" DESC`) {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuild_UnknownOrderField_Errors(t *testing.T) {
	order := []SqlOrderClause{{Field: "ghost", Dir: "ASC"}}
	_, _, err := bpg().Build(nil, nil, order, 0, "", builderOpts)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestBuild_Pg_UsesLimitOffset(t *testing.T) {
	sql, params, _ := bpg().Build(nil, nil, nil, 10, "", builderOpts)
	if !strings.Contains(sql, "LIMIT @_limit OFFSET @_offset") {
		t.Fatalf("sql=%q", sql)
	}
	if param(params, "_limit") != 10 || param(params, "_offset") != 0 {
		t.Fatalf("limit=%v offset=%v", param(params, "_limit"), param(params, "_offset"))
	}
}

func TestBuild_SqlServer_UsesOffsetFetch(t *testing.T) {
	sql, params, _ := bsql().Build(nil, nil, nil, 5, "", builderOpts)
	if !strings.Contains(sql, "OFFSET @_offset ROWS FETCH NEXT @_limit ROWS ONLY") {
		t.Fatalf("sql=%q", sql)
	}
	if param(params, "_limit") != 5 {
		t.Fatalf("limit=%v", param(params, "_limit"))
	}
}

func TestBuild_LimitClamped_ToMaxLimit(t *testing.T) {
	_, params, _ := bpg().Build(nil, nil, nil, 999, "", builderOpts)
	if param(params, "_limit") != 100 {
		t.Fatalf("limit=%v", param(params, "_limit"))
	}
}

func TestBuild_ZeroLimit_UsesDefault(t *testing.T) {
	_, params, _ := bpg().Build(nil, nil, nil, 0, "", builderOpts)
	if param(params, "_limit") != 20 {
		t.Fatalf("limit=%v", param(params, "_limit"))
	}
}

func TestBuild_WithCursor_DecodesOffset(t *testing.T) {
	cursor := EncodeCursor(40)
	_, params, _ := bpg().Build(nil, nil, nil, 10, cursor, builderOpts)
	if param(params, "_offset") != 40 {
		t.Fatalf("offset=%v", param(params, "_offset"))
	}
}

func TestEncodeCursor_ZeroOrNegative_ReturnsEmpty(t *testing.T) {
	if EncodeCursor(0) != "" || EncodeCursor(-1) != "" {
		t.Fatal("want empty")
	}
}

func TestCursorRoundtrip(t *testing.T) {
	for _, off := range []int64{1, 20, 1000, (1 << 62)} {
		c := EncodeCursor(off)
		if c == "" {
			t.Fatalf("empty cursor for %d", off)
		}
		if DecodeCursor(c) != off {
			t.Fatalf("roundtrip %d → %d", off, DecodeCursor(c))
		}
	}
}

func TestDecodeCursor_NullOrEmpty_ReturnsZero(t *testing.T) {
	if DecodeCursor("") != 0 {
		t.Fatal("want 0")
	}
}

func TestDecodeCursor_Garbage_ReturnsZero(t *testing.T) {
	if DecodeCursor("not-a-cursor!@#$") != 0 {
		t.Fatal("want 0")
	}
}

func TestBuildCount_NoFilter_NoWhere(t *testing.T) {
	sql, _, _ := bpg().BuildCount(nil)
	if !strings.HasPrefix(sql, "SELECT COUNT(*) FROM") {
		t.Fatalf("sql=%q", sql)
	}
	if strings.Contains(sql, "WHERE") {
		t.Fatalf("sql=%q", sql)
	}
}

func TestBuildCount_WithFilter_AppendsWhere(t *testing.T) {
	sql, _, _ := bpg().BuildCount(parseFilter(t, `{"price":{"$gt":0}}`))
	if !strings.Contains(sql, "WHERE") {
		t.Fatalf("sql=%q", sql)
	}
}
