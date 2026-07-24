// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// NWP filter DSL → parameterized SQL translation (NPS-2 §5.2). Pure logic:
// no database driver is involved. Ported from the .NET reference
// (MemoryNode/Query/NwpFilterTranslator.cs) with identical SQL output and
// parameter naming (@p0, @p1, ...). Field names are validated against the
// schema to prevent SQL injection.

package nwp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DatabaseDialect selects SQL quoting and pagination syntax.
type DatabaseDialect int

const (
	// DialectSqlServer uses [bracket] quoting and OFFSET/FETCH pagination.
	DialectSqlServer DatabaseDialect = iota
	// DialectPostgreSql uses "double-quote" quoting and LIMIT/OFFSET pagination.
	DialectPostgreSql
)

// SqlMemoryNodeField describes a queryable field for the SQL translator
// (NPS-2 §4.1). It mirrors the .NET MemoryNodeField, adding the DB-column
// mapping that the wire-level MemoryNodeField does not carry.
type SqlMemoryNodeField struct {
	// Name is the field name exposed in NWP responses.
	Name string
	// Type is the NWP field type: "string", "number", "boolean", "datetime", "object".
	Type string
	// Description is the human-readable description surfaced in the AnchorFrame schema.
	Description string
	// Nullable reports whether the field may be null.
	Nullable bool
	// ColumnName is the underlying DB column, when different from Name. Empty → Name.
	ColumnName string
}

// ResolvedColumnName returns ColumnName, falling back to Name.
func (f SqlMemoryNodeField) ResolvedColumnName() string {
	if f.ColumnName != "" {
		return f.ColumnName
	}
	return f.Name
}

// SqlMemoryNodeSchema describes the DB table a Memory Node exposes, for the
// SQL translator / query builder (NPS-2 §4.1). Distinct from the wire-facing
// MemoryNodeSchema so the JSON schema shape stays unchanged.
type SqlMemoryNodeSchema struct {
	// TableName is the database table (or view) name.
	TableName string
	// PrimaryKey is the primary-key field name, used for cursor pagination.
	PrimaryKey string
	// Fields are all queryable fields; must contain at least the primary key.
	Fields []SqlMemoryNodeField
}

// GetField returns the field descriptor for name (case-insensitive), or false.
func (s SqlMemoryNodeSchema) GetField(name string) (SqlMemoryNodeField, bool) {
	for _, f := range s.Fields {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return SqlMemoryNodeField{}, false
}

// HasField reports whether name is a declared field.
func (s SqlMemoryNodeSchema) HasField(name string) bool {
	_, ok := s.GetField(name)
	return ok
}

// NwpFilterError is raised when a NWP filter cannot be translated to SQL.
// NwpErrorCode carries the wire error code (default NWP-QUERY-FILTER-INVALID).
type NwpFilterError struct {
	Message      string
	NwpErrorCode string
}

func (e *NwpFilterError) Error() string { return e.Message }

func newFilterError(errorCode, format string, args ...any) *NwpFilterError {
	return &NwpFilterError{Message: fmt.Sprintf(format, args...), NwpErrorCode: errorCode}
}

// NwpFilterTranslator translates a NWP filter predicate (NPS-2 §5.2) to a
// parameterized SQL WHERE-clause fragment plus an ordered parameter list.
type NwpFilterTranslator struct {
	schema     SqlMemoryNodeSchema
	dialect    DatabaseDialect
	paramIndex int
	params     []NwpSqlParam
}

// NwpSqlParam is a single ordered SQL parameter. Name is the placeholder name
// without the leading '@'. Value holds a scalar, or a []any for $in/$nin.
type NwpSqlParam struct {
	Name  string
	Value any
}

// NewNwpFilterTranslator builds a translator for the given schema and dialect.
func NewNwpFilterTranslator(schema SqlMemoryNodeSchema, dialect DatabaseDialect) *NwpFilterTranslator {
	return &NwpFilterTranslator{schema: schema, dialect: dialect}
}

// Translate converts filter (a decoded JSON value: map[string]any / nil) into a
// WHERE-clause fragment, returning it together with the ordered parameters.
// A nil filter yields an empty clause and no parameters.
func (t *NwpFilterTranslator) Translate(filter any) (string, []NwpSqlParam, error) {
	t.paramIndex = 0
	t.params = nil
	if filter == nil {
		return "", nil, nil
	}
	obj, ok := filter.(map[string]any)
	if !ok {
		return "", nil, nil
	}
	sql, err := t.buildObject(obj)
	if err != nil {
		return "", nil, err
	}
	return sql, t.params, nil
}

func (t *NwpFilterTranslator) addParam(name string, value any) {
	t.params = append(t.params, NwpSqlParam{Name: name, Value: value})
}

// sortedKeys returns object keys in a deterministic order (stable SQL output).
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (t *NwpFilterTranslator) buildObject(obj map[string]any) (string, error) {
	var clauses []string
	for _, name := range sortedKeys(obj) {
		val := obj[name]
		if strings.HasPrefix(name, "$") {
			clause, err := t.buildLogical(name, val)
			if err != nil {
				return "", err
			}
			clauses = append(clauses, clause)
			continue
		}
		field, err := t.validateField(name)
		if err != nil {
			return "", err
		}
		clause, err := t.buildFieldCondition(field, val)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}

	switch len(clauses) {
	case 0:
		return "", nil
	case 1:
		return clauses[0], nil
	default:
		return "(" + strings.Join(clauses, " AND ") + ")", nil
	}
}

func (t *NwpFilterTranslator) buildLogical(op string, value any) (string, error) {
	// $not takes a single sub-object; $and/$or take an array.
	if op == "$not" {
		sub, ok := value.(map[string]any)
		if !ok {
			return "", newFilterError(ErrQueryFilterInvalid,
				"Logical operator '%s' requires an object value.", op)
		}
		inner, err := t.buildObject(sub)
		if err != nil {
			return "", err
		}
		if inner == "" {
			return "", nil
		}
		return "NOT (" + inner + ")", nil
	}

	var separator string
	switch op {
	case "$and":
		separator = " AND "
	case "$or":
		separator = " OR "
	default:
		return "", newFilterError(ErrQueryFilterInvalid, "Unknown logical operator '%s'.", op)
	}

	arr, ok := value.([]any)
	if !ok {
		return "", newFilterError(ErrQueryFilterInvalid,
			"Logical operator '%s' requires an array value.", op)
	}

	var parts []string
	for _, el := range arr {
		sub, ok := el.(map[string]any)
		if !ok {
			return "", newFilterError(ErrQueryFilterInvalid,
				"Logical operator '%s' array elements must be objects.", op)
		}
		s, err := t.buildObject(sub)
		if err != nil {
			return "", err
		}
		if s != "" {
			parts = append(parts, s)
		}
	}

	switch len(parts) {
	case 0:
		return "", nil
	case 1:
		return parts[0], nil
	default:
		return "(" + strings.Join(parts, separator) + ")", nil
	}
}

func (t *NwpFilterTranslator) buildFieldCondition(field SqlMemoryNodeField, condition any) (string, error) {
	cond, ok := condition.(map[string]any)
	if !ok {
		return "", newFilterError(ErrQueryFilterInvalid,
			"Field '%s' condition must be an object (e.g. {\"$eq\": value}).", field.Name)
	}

	col := t.quoteColumn(field.ResolvedColumnName())
	var parts []string
	for _, op := range sortedKeys(cond) {
		val := cond[op]
		var (
			clause string
			err    error
		)
		switch op {
		case "$in":
			clause, err = t.buildIn(col, val, false)
		case "$nin":
			clause, err = t.buildIn(col, val, true)
		case "$between":
			clause, err = t.buildBetween(col, val)
		default:
			clause, err = t.buildSimple(col, op, field.Name, val)
		}
		if err != nil {
			return "", err
		}
		parts = append(parts, clause)
	}

	if len(parts) == 1 {
		return parts[0], nil
	}
	return "(" + strings.Join(parts, " AND ") + ")", nil
}

func (t *NwpFilterTranslator) buildSimple(col, op, fieldName string, value any) (string, error) {
	paramName := fmt.Sprintf("p%d", t.paramIndex)
	t.paramIndex++
	v := extractValue(value)
	switch op {
	case "$eq":
		t.addParam(paramName, v)
		return fmt.Sprintf("%s = @%s", col, paramName), nil
	case "$ne":
		t.addParam(paramName, v)
		return fmt.Sprintf("%s <> @%s", col, paramName), nil
	case "$lt":
		t.addParam(paramName, v)
		return fmt.Sprintf("%s < @%s", col, paramName), nil
	case "$lte":
		t.addParam(paramName, v)
		return fmt.Sprintf("%s <= @%s", col, paramName), nil
	case "$gt":
		t.addParam(paramName, v)
		return fmt.Sprintf("%s > @%s", col, paramName), nil
	case "$gte":
		t.addParam(paramName, v)
		return fmt.Sprintf("%s >= @%s", col, paramName), nil
	case "$contains":
		t.addParam(paramName, fmt.Sprintf("%%%v%%", v))
		return fmt.Sprintf("%s LIKE @%s", col, paramName), nil
	default:
		return "", newFilterError(ErrQueryFilterInvalid,
			"Unknown filter operator '%s' on field '%s'.", op, fieldName)
	}
}

func (t *NwpFilterTranslator) buildIn(col string, arr any, negate bool) (string, error) {
	items, ok := arr.([]any)
	if !ok {
		return "", newFilterError(ErrQueryFilterInvalid, "$in/$nin requires an array value.")
	}
	values := make([]any, 0, len(items))
	for _, e := range items {
		values = append(values, extractValue(e))
	}
	if len(values) == 0 {
		// empty IN → always false; empty NIN → always true
		if negate {
			return "1=1", nil
		}
		return "1=0", nil
	}
	paramName := fmt.Sprintf("p%d", t.paramIndex)
	t.paramIndex++
	t.addParam(paramName, values)
	if negate {
		return fmt.Sprintf("%s NOT IN @%s", col, paramName), nil
	}
	return fmt.Sprintf("%s IN @%s", col, paramName), nil
}

func (t *NwpFilterTranslator) buildBetween(col string, arr any) (string, error) {
	items, ok := arr.([]any)
	if !ok || len(items) != 2 {
		return "", newFilterError(ErrQueryFilterInvalid,
			"$between requires an array of exactly two values [low, high].")
	}
	pLow := fmt.Sprintf("p%d", t.paramIndex)
	t.paramIndex++
	pHigh := fmt.Sprintf("p%d", t.paramIndex)
	t.paramIndex++
	t.addParam(pLow, extractValue(items[0]))
	t.addParam(pHigh, extractValue(items[1]))
	return fmt.Sprintf("%s BETWEEN @%s AND @%s", col, pLow, pHigh), nil
}

func (t *NwpFilterTranslator) validateField(name string) (SqlMemoryNodeField, error) {
	field, ok := t.schema.GetField(name)
	if !ok {
		return SqlMemoryNodeField{}, newFilterError(ErrQueryFieldUnknown, "Unknown field '%s'.", name)
	}
	return field, nil
}

func (t *NwpFilterTranslator) quoteColumn(col string) string {
	if t.dialect == DialectSqlServer {
		return "[" + col + "]"
	}
	return "\"" + col + "\""
}

// extractValue normalizes a decoded JSON value for use as a SQL parameter.
// JSON numbers are integral when they have no fractional part (mirrors the
// .NET TryGetInt64 → long else double behavior).
func extractValue(el any) any {
	switch v := el.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return el
	}
}
