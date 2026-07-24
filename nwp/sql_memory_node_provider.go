// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// SQL-backed Memory Node providers (NPS-2 §2.1), ported from the .NET reference
// PostgreSqlMemoryNodeProvider.cs / SqlServerMemoryNodeProvider.cs.
//
// The build layer (filter→SQL) is pure and identical to .NET. The execution
// layer is decoupled behind an injectable SqlExecutor so providers are testable
// without a live database. A concrete database/sql-backed executor is provided;
// binding to a specific driver (pgx, go-sqlite3, ...) is deferred to the host,
// which registers a driver and passes a *sql.DB.

package nwp

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// SqlExecutor abstracts query execution so SQL generation can be tested without
// a database. Implementations receive the driver-ready SQL (placeholders already
// rebound for the dialect) and the ordered argument list.
type SqlExecutor interface {
	// QueryRows runs a row-returning query and returns each row as a
	// column-name → value map.
	QueryRows(ctx context.Context, query string, args []any) ([]MemoryNodeRow, error)
	// QueryScalar runs a single-value query (e.g. COUNT(*)) and returns it.
	QueryScalar(ctx context.Context, query string, args []any) (int64, error)
}

// SqlMemoryNodeProvider is a Memory Node provider built on SqlQueryBuilder and
// an injectable SqlExecutor. Both PostgreSQL and SQL Server are covered by
// selecting the DatabaseDialect.
type SqlMemoryNodeProvider struct {
	schema   SqlMemoryNodeSchema
	dialect  DatabaseDialect
	executor SqlExecutor
	builder  *SqlQueryBuilder
	options  SqlQueryOptions
}

// NewSqlMemoryNodeProvider builds a provider for the given schema, dialect,
// executor and limits.
func NewSqlMemoryNodeProvider(
	schema SqlMemoryNodeSchema,
	dialect DatabaseDialect,
	executor SqlExecutor,
	options SqlQueryOptions,
) *SqlMemoryNodeProvider {
	if options.DefaultLimit == 0 {
		options.DefaultLimit = 20
	}
	if options.MaxLimit == 0 {
		options.MaxLimit = 1000
	}
	return &SqlMemoryNodeProvider{
		schema:   schema,
		dialect:  dialect,
		executor: executor,
		builder:  NewSqlQueryBuilder(schema, dialect),
		options:  options,
	}
}

// NewPostgreSqlMemoryNodeProvider builds a PostgreSQL provider.
func NewPostgreSqlMemoryNodeProvider(schema SqlMemoryNodeSchema, executor SqlExecutor, options SqlQueryOptions) *SqlMemoryNodeProvider {
	return NewSqlMemoryNodeProvider(schema, DialectPostgreSql, executor, options)
}

// NewSqlServerMemoryNodeProvider builds a SQL Server provider.
func NewSqlServerMemoryNodeProvider(schema SqlMemoryNodeSchema, executor SqlExecutor, options SqlQueryOptions) *SqlMemoryNodeProvider {
	return NewSqlMemoryNodeProvider(schema, DialectSqlServer, executor, options)
}

// BuildQuerySQL exposes the generated (named-parameter) SELECT SQL and ordered
// parameters for a query, without executing it. Primarily for SQL-generation
// tests and diagnostics.
func (p *SqlMemoryNodeProvider) BuildQuerySQL(frame *QueryFrame) (string, []NwpSqlParam, error) {
	return p.builder.Build(
		frame.Fields,
		frame.Filter,
		OrderClausesFromJSON(frame.Order),
		frameLimit(frame),
		frameCursor(frame),
		p.options,
	)
}

// BuildCountSQL exposes the generated COUNT(*) SQL and parameters.
func (p *SqlMemoryNodeProvider) BuildCountSQL(frame *QueryFrame) (string, []NwpSqlParam, error) {
	return p.builder.BuildCount(frame.Filter)
}

// Query executes the frame and returns a paginated result. Implements
// IMemoryNodeProvider.
func (p *SqlMemoryNodeProvider) Query(ctx context.Context, frame *QueryFrame, _ MemoryNodeOptions) (*MemoryNodeQueryResult, error) {
	namedSQL, params, err := p.BuildQuerySQL(frame)
	if err != nil {
		return nil, err
	}
	sqlText, args := RebindNamedParams(namedSQL, params, p.dialect)

	rows, err := p.executor.QueryRows(ctx, sqlText, args)
	if err != nil {
		return nil, err
	}

	limit := frameLimit(frame)
	if limit == 0 {
		limit = p.options.DefaultLimit
	}
	if p.options.MaxLimit > 0 && limit > p.options.MaxLimit {
		limit = p.options.MaxLimit
	}

	nextCursor := ""
	if uint64(len(rows)) == limit {
		nextCursor = EncodeCursor(DecodeCursor(frameCursor(frame)) + int64(limit))
	}

	return &MemoryNodeQueryResult{Rows: rows, NextCursor: nextCursor}, nil
}

// Count returns the total number of rows matching the frame's filter.
func (p *SqlMemoryNodeProvider) Count(ctx context.Context, frame *QueryFrame) (int64, error) {
	namedSQL, params, err := p.BuildCountSQL(frame)
	if err != nil {
		return 0, err
	}
	sqlText, args := RebindNamedParams(namedSQL, params, p.dialect)
	return p.executor.QueryScalar(ctx, sqlText, args)
}

// Stream implements StreamingProvider, paging through the full result set.
func (p *SqlMemoryNodeProvider) Stream(ctx context.Context, frame *QueryFrame, _ MemoryNodeOptions) (<-chan []MemoryNodeRow, <-chan error) {
	pages := make(chan []MemoryNodeRow)
	errs := make(chan error, 1)

	go func() {
		defer close(pages)
		defer close(errs)

		pageLimit := frameLimit(frame)
		if pageLimit == 0 {
			pageLimit = p.options.DefaultLimit
		}
		if p.options.MaxLimit > 0 && pageLimit > p.options.MaxLimit {
			pageLimit = p.options.MaxLimit
		}
		cursor := frameCursor(frame)

		for {
			if ctx.Err() != nil {
				return
			}
			pageFrame := *frame
			pl := pageLimit
			pageFrame.Limit = &pl
			pc := cursor
			pageFrame.Cursor = &pc

			namedSQL, params, err := p.BuildQuerySQL(&pageFrame)
			if err != nil {
				errs <- err
				return
			}
			sqlText, args := RebindNamedParams(namedSQL, params, p.dialect)
			rows, err := p.executor.QueryRows(ctx, sqlText, args)
			if err != nil {
				errs <- err
				return
			}
			if len(rows) == 0 {
				return
			}
			pages <- rows
			if uint64(len(rows)) != pageLimit {
				return
			}
			cursor = EncodeCursor(DecodeCursor(cursor) + int64(len(rows)))
		}
	}()

	return pages, errs
}

// ── Named-parameter rebinding ─────────────────────────────────────────────────

// RebindNamedParams rewrites @name placeholders produced by SqlQueryBuilder into
// the driver placeholder style for the dialect ($1.. for PostgreSQL, @pN for
// SQL Server via database/sql named args are not portable, so we also use
// positional @pN → ordered args). It expands []any parameters (from $in/$nin)
// into an IN (...) list. Returns the rewritten SQL and the ordered args slice.
func RebindNamedParams(namedSQL string, params []NwpSqlParam, dialect DatabaseDialect) (string, []any) {
	byName := make(map[string]any, len(params))
	for _, p := range params {
		byName[p.Name] = p.Value
	}

	// Replace longest names first so "@p10" is not matched by "@p1".
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	var args []any
	pos := 0
	sqlText := namedSQL

	placeholder := func() string {
		pos++
		if dialect == DialectPostgreSql {
			return fmt.Sprintf("$%d", pos)
		}
		return "?"
	}

	// Walk the placeholders in source order to keep positional binding correct.
	// We rebuild the string left to right.
	var out strings.Builder
	i := 0
	for i < len(sqlText) {
		if sqlText[i] == '@' {
			// find the token name
			j := i + 1
			for j < len(sqlText) && isNameChar(sqlText[j]) {
				j++
			}
			name := sqlText[i+1 : j]
			if val, ok := byName[name]; ok {
				if list, isList := val.([]any); isList {
					parts := make([]string, len(list))
					for k, v := range list {
						parts[k] = placeholder()
						args = append(args, v)
					}
					out.WriteString("(" + strings.Join(parts, ", ") + ")")
				} else {
					out.WriteString(placeholder())
					args = append(args, val)
				}
				i = j
				continue
			}
		}
		out.WriteByte(sqlText[i])
		i++
	}
	return out.String(), args
}

func isNameChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ── database/sql executor ─────────────────────────────────────────────────────

// DBSqlExecutor is a SqlExecutor over a *database/sql.DB. The host registers a
// driver (e.g. pgx, go-sqlite3) and passes the open *sql.DB. This keeps the SDK
// driver-agnostic and dependency-free.
type DBSqlExecutor struct {
	DB *sql.DB
}

// NewDBSqlExecutor wraps a *sql.DB as a SqlExecutor.
func NewDBSqlExecutor(db *sql.DB) *DBSqlExecutor { return &DBSqlExecutor{DB: db} }

// QueryRows runs a row-returning query and maps each row to a column→value map.
func (e *DBSqlExecutor) QueryRows(ctx context.Context, query string, args []any) ([]MemoryNodeRow, error) {
	rows, err := e.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []MemoryNodeRow
	for rows.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(MemoryNodeRow, len(cols))
		for i, c := range cols {
			row[c] = normalizeScanned(holders[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// QueryScalar runs a single-value query and returns it as int64.
func (e *DBSqlExecutor) QueryScalar(ctx context.Context, query string, args []any) (int64, error) {
	var n int64
	err := e.DB.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// normalizeScanned converts driver-returned []byte to string (mirrors the .NET
// row mapping that surfaces text columns as strings, DBNull → nil).
func normalizeScanned(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// ── QueryFrame accessors ──────────────────────────────────────────────────────

func frameLimit(f *QueryFrame) uint64 {
	if f.Limit != nil {
		return *f.Limit
	}
	return 0
}

func frameCursor(f *QueryFrame) string {
	if f.Cursor != nil {
		return *f.Cursor
	}
	return ""
}
