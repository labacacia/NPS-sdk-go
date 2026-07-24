// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// SQL SELECT builder for Memory Node queries (NPS-2 §5). Ported from the .NET
// reference (MemoryNode/Query/SqlQueryBuilder.cs) with identical SQL output,
// parameter naming, and opaque Base64-URL cursor encoding.

package nwp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// SqlQueryOptions carries the query limits the builder needs. Mirrors the
// subset of MemoryNodeOptions used by the .NET SqlQueryBuilder.
type SqlQueryOptions struct {
	DefaultLimit uint64
	MaxLimit     uint64
}

// SqlOrderClause is one ORDER BY term: field name plus direction ("ASC"/"DESC").
type SqlOrderClause struct {
	Field string
	Dir   string
}

// SqlQueryBuilder builds parameterized SELECT queries from a QueryFrame,
// handling projection, filter, ordering, and cursor pagination. Dialect-specific
// quoting and LIMIT syntax is injected via DatabaseDialect.
type SqlQueryBuilder struct {
	schema  SqlMemoryNodeSchema
	dialect DatabaseDialect
	filter  *NwpFilterTranslator
}

// NewSqlQueryBuilder builds a query builder for the given schema and dialect.
func NewSqlQueryBuilder(schema SqlMemoryNodeSchema, dialect DatabaseDialect) *SqlQueryBuilder {
	return &SqlQueryBuilder{
		schema:  schema,
		dialect: dialect,
		filter:  NewNwpFilterTranslator(schema, dialect),
	}
}

// Build produces the full SELECT SQL and its ordered parameters.
//
// filter/order accept decoded-JSON values (map[string]any, []any) OR a typed
// []SqlOrderClause for order. limit/offset come from frameLimit and cursor.
func (b *SqlQueryBuilder) Build(
	fields []string,
	filter any,
	order []SqlOrderClause,
	frameLimit uint64,
	cursor string,
	options SqlQueryOptions,
) (string, []NwpSqlParam, error) {
	var sb strings.Builder

	limit := frameLimit
	if limit == 0 {
		limit = options.DefaultLimit
	}
	if options.MaxLimit > 0 && limit > options.MaxLimit {
		limit = options.MaxLimit
	}
	offset := DecodeCursor(cursor)

	// SELECT
	selectList, err := b.buildSelectList(fields)
	if err != nil {
		return "", nil, err
	}
	sb.WriteString("SELECT ")
	sb.WriteString(selectList)

	// FROM
	sb.WriteString(" FROM ")
	sb.WriteString(b.quoteTable(b.schema.TableName))

	// WHERE
	where, params, err := b.filter.Translate(filter)
	if err != nil {
		return "", nil, err
	}
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	// ORDER BY (required for stable pagination)
	if len(order) > 0 {
		orderBy, err := b.buildOrderBy(order)
		if err != nil {
			return "", nil, err
		}
		sb.WriteString(" ORDER BY ")
		sb.WriteString(orderBy)
	} else {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(b.quoteColumn(b.schema.PrimaryKey))
	}

	// PAGINATION — dialect-specific syntax
	if b.dialect == DialectSqlServer {
		sb.WriteString(" OFFSET @_offset ROWS FETCH NEXT @_limit ROWS ONLY")
	} else {
		sb.WriteString(" LIMIT @_limit OFFSET @_offset")
	}

	params = append(params,
		NwpSqlParam{Name: "_limit", Value: int(limit)},
		NwpSqlParam{Name: "_offset", Value: int(offset)},
	)

	return sb.String(), params, nil
}

// BuildCount produces a COUNT(*) query for the same filter (cursor validation).
func (b *SqlQueryBuilder) BuildCount(filter any) (string, []NwpSqlParam, error) {
	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*) FROM ")
	sb.WriteString(b.quoteTable(b.schema.TableName))

	where, params, err := b.filter.Translate(filter)
	if err != nil {
		return "", nil, err
	}
	if where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}
	return sb.String(), params, nil
}

// ── Cursor ──────────────────────────────────────────────────────────────────

// EncodeCursor encodes a row offset as an opaque Base64-URL cursor.
// Offsets <= 0 encode to the empty string (no cursor).
func EncodeCursor(nextOffset int64) string {
	if nextOffset <= 0 {
		return ""
	}
	raw := fmt.Sprintf(`{"o":%d}`, nextOffset)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor decodes a Base64-URL cursor back to a row offset.
// Returns 0 for an empty or invalid cursor.
func DecodeCursor(cursor string) int64 {
	if cursor == "" {
		return 0
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		// tolerate padded input
		data, err = base64.URLEncoding.DecodeString(cursor)
		if err != nil {
			return 0
		}
	}
	var doc struct {
		O json.Number `json:"o"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return 0
	}
	n, err := doc.O.Int64()
	if err != nil {
		return 0
	}
	return n
}

// ── Private ───────────────────────────────────────────────────────────────

func (b *SqlQueryBuilder) buildSelectList(fields []string) (string, error) {
	if len(fields) == 0 {
		// All declared schema fields (not SELECT *, to avoid schema drift).
		cols := make([]string, len(b.schema.Fields))
		for i, f := range b.schema.Fields {
			cols[i] = b.quoteColumn(f.ResolvedColumnName())
		}
		return strings.Join(cols, ", "), nil
	}

	for _, name := range fields {
		if !b.schema.HasField(name) {
			return "", newFilterError(ErrQueryFieldUnknown, "Unknown field '%s'.", name)
		}
	}

	parts := make([]string, 0, len(fields))
	for _, name := range fields {
		f, _ := b.schema.GetField(name)
		col := b.quoteColumn(f.ResolvedColumnName())
		// Alias back to the NWP name when the column name differs.
		if f.ColumnName != "" {
			parts = append(parts, col+" AS "+b.quoteColumn(f.Name))
		} else {
			parts = append(parts, col)
		}
	}
	return strings.Join(parts, ", "), nil
}

func (b *SqlQueryBuilder) buildOrderBy(order []SqlOrderClause) (string, error) {
	parts := make([]string, 0, len(order))
	for _, o := range order {
		field, ok := b.schema.GetField(o.Field)
		if !ok {
			return "", newFilterError(ErrQueryFieldUnknown, "Unknown order field '%s'.", o.Field)
		}
		dir := "ASC"
		if strings.EqualFold(o.Dir, "DESC") {
			dir = "DESC"
		}
		parts = append(parts, b.quoteColumn(field.ResolvedColumnName())+" "+dir)
	}
	return strings.Join(parts, ", "), nil
}

func (b *SqlQueryBuilder) quoteColumn(col string) string {
	if b.dialect == DialectSqlServer {
		return "[" + col + "]"
	}
	return "\"" + col + "\""
}

func (b *SqlQueryBuilder) quoteTable(table string) string {
	if b.dialect == DialectSqlServer {
		return "[" + table + "]"
	}
	return "\"" + table + "\""
}

// OrderClausesFromJSON converts a decoded-JSON order value (a single
// {field,dir} object or an array of them) into typed clauses. This lets the
// HTTP layer feed QueryFrame.Order directly into Build.
func OrderClausesFromJSON(order any) []SqlOrderClause {
	var out []SqlOrderClause
	add := func(m map[string]any) {
		field, _ := m["field"].(string)
		if field == "" {
			return
		}
		dir, _ := m["dir"].(string)
		out = append(out, SqlOrderClause{Field: field, Dir: dir})
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
	case []SqlOrderClause:
		return v
	}
	return out
}
