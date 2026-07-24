// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
//
// SQL-backed NIP CA certificate store (NPS-3 §8). Ported from the .NET
// reference SqliteNipCaStore.cs. The SQL/DDL is driver-agnostic; the store is
// built over an injectable executor so it is fully testable without a database
// driver. A database/sql binding (SqlDBExecutor) is provided; the host
// registers a concrete driver (go-sqlite3, pgx, ...) and supplies the *sql.DB.

package nip

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SqlCaExecutor abstracts the DB access the SQL CA store needs. Placeholders in
// the queries use the "?" positional style; a Postgres binding may rewrite them.
type SqlCaExecutor interface {
	// Exec runs a non-row statement and returns rows affected.
	Exec(query string, args ...any) (int64, error)
	// QueryRows runs a row-returning query, invoking scan for each row.
	QueryRows(query string, args []any, scan func(RowScanner) error) error
	// QueryScalarInt64 runs a query returning a single int64.
	QueryScalarInt64(query string, args ...any) (int64, error)
}

// RowScanner scans the current row's columns into dest (like sql.Rows.Scan).
type RowScanner interface {
	Scan(dest ...any) error
}

// SqlNipCaStore is a NipCaStore backed by SQL through an injectable executor.
// The schema and statements mirror the .NET SqliteNipCaStore verbatim.
type SqlNipCaStore struct {
	exec SqlCaExecutor
}

// NewSqlNipCaStore builds a SQL CA store over the given executor and applies the
// schema migration.
func NewSqlNipCaStore(exec SqlCaExecutor) (*SqlNipCaStore, error) {
	s := &SqlNipCaStore{exec: exec}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// MigrationStatements returns the ordered DDL/DML statements that build the NIP
// CA schema. Exposed for SQL-generation tests and for hosts that run migrations
// out-of-band.
func MigrationStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS nip_certs (
                nid               TEXT NOT NULL,
                entity_type       TEXT NOT NULL,
                serial            TEXT NOT NULL UNIQUE,
                pub_key           TEXT NOT NULL,
                capabilities_json TEXT NOT NULL DEFAULT '[]',
                scope_json        TEXT NOT NULL DEFAULT '{}',
                issued_by         TEXT NOT NULL,
                issued_at         TEXT NOT NULL,
                expires_at        TEXT NOT NULL,
                revoked_at        TEXT,
                revoke_reason     TEXT,
                metadata_json     TEXT,
                nid_role          TEXT,
                parent_nid        TEXT,
                lineage_json      TEXT
            )`,
		"CREATE INDEX IF NOT EXISTS idx_nip_certs_nid        ON nip_certs (nid)",
		"CREATE INDEX IF NOT EXISTS idx_nip_certs_serial     ON nip_certs (serial)",
		"CREATE INDEX IF NOT EXISTS idx_nip_certs_parent_nid ON nip_certs (parent_nid)",
		`CREATE TABLE IF NOT EXISTS nip_serial (
                id   INTEGER PRIMARY KEY,
                seq  INTEGER NOT NULL DEFAULT 0
            )`,
		"INSERT OR IGNORE INTO nip_serial (id, seq) VALUES (1, 0)",
	}
}

func (s *SqlNipCaStore) migrate() error {
	for _, stmt := range MigrationStatements() {
		if _, err := s.exec.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// Save inserts a newly issued certificate record.
func (s *SqlNipCaStore) Save(record *NipCertRecord) error {
	const q = `INSERT INTO nip_certs
                (nid, entity_type, serial, pub_key, capabilities_json, scope_json,
                 issued_by, issued_at, expires_at, metadata_json,
                 nid_role, parent_nid, lineage_json)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	capJSON, _ := json.Marshal(record.Capabilities)
	_, err := s.exec.Exec(q,
		record.Nid,
		record.EntityType,
		record.Serial,
		record.PubKey,
		string(capJSON),
		record.ScopeJson,
		record.IssuedBy,
		isoTime(record.IssuedAt),
		isoTime(record.ExpiresAt),
		nullString(record.MetadataJson),
		nullString(record.NidRole),
		nullString(record.ParentNid),
		nullString(record.LineageJson),
	)
	return err
}

// GetByNid returns the latest certificate record for nid, or nil.
func (s *SqlNipCaStore) GetByNid(nid string) (*NipCertRecord, error) {
	const q = `SELECT * FROM nip_certs WHERE nid = ? ORDER BY issued_at DESC LIMIT 1`
	return s.queryFirst(q, nid)
}

// GetBySerial returns the certificate record for serial, or nil.
func (s *SqlNipCaStore) GetBySerial(serial string) (*NipCertRecord, error) {
	const q = `SELECT * FROM nip_certs WHERE serial = ? LIMIT 1`
	return s.queryFirst(q, serial)
}

// Revoke marks a certificate as revoked; returns false if not found / already revoked.
func (s *SqlNipCaStore) Revoke(nid, reason string, revokedAt time.Time) (bool, error) {
	const q = `UPDATE nip_certs SET revoked_at = ?, revoke_reason = ? WHERE nid = ? AND revoked_at IS NULL`
	n, err := s.exec.Exec(q, isoTime(revokedAt), reason, nid)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// NextSerial atomically reserves and returns the next hex serial (e.g. 0xA3F9C).
func (s *SqlNipCaStore) NextSerial() (string, error) {
	if _, err := s.exec.Exec("UPDATE nip_serial SET seq = seq + 1 WHERE id = 1"); err != nil {
		return "", err
	}
	next, err := s.exec.QueryScalarInt64("SELECT seq FROM nip_serial WHERE id = 1")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("0x%X", next), nil
}

// List returns all certificate records, newest first.
func (s *SqlNipCaStore) List() ([]*NipCertRecord, error) {
	return s.queryMany("SELECT * FROM nip_certs ORDER BY issued_at DESC")
}

// GetRevoked returns all revoked certificates (for CRL generation).
func (s *SqlNipCaStore) GetRevoked() ([]*NipCertRecord, error) {
	return s.queryMany("SELECT * FROM nip_certs WHERE revoked_at IS NOT NULL ORDER BY revoked_at DESC")
}

// GetByParentNid returns records whose parent_nid equals parentNid, newest first.
func (s *SqlNipCaStore) GetByParentNid(parentNid string) ([]*NipCertRecord, error) {
	return s.queryMany("SELECT * FROM nip_certs WHERE parent_nid = ? ORDER BY issued_at DESC", parentNid)
}

// ── row mapping ───────────────────────────────────────────────────────────────

// certRow is the raw column layout of nip_certs in declaration order, so a
// SELECT * scans positionally without depending on driver column-name access.
type certRow struct {
	Nid          string
	EntityType   string
	Serial       string
	PubKey       string
	CapJSON      string
	ScopeJSON    string
	IssuedBy     string
	IssuedAt     string
	ExpiresAt    string
	RevokedAt    sql.NullString
	RevokeReason sql.NullString
	MetadataJSON sql.NullString
	NidRole      sql.NullString
	ParentNid    sql.NullString
	LineageJSON  sql.NullString
}

func (s *SqlNipCaStore) scanTargets(r *certRow) []any {
	return []any{
		&r.Nid, &r.EntityType, &r.Serial, &r.PubKey, &r.CapJSON, &r.ScopeJSON,
		&r.IssuedBy, &r.IssuedAt, &r.ExpiresAt, &r.RevokedAt, &r.RevokeReason,
		&r.MetadataJSON, &r.NidRole, &r.ParentNid, &r.LineageJSON,
	}
}

func (s *SqlNipCaStore) mapRow(r *certRow) (*NipCertRecord, error) {
	var caps []string
	if r.CapJSON != "" {
		if err := json.Unmarshal([]byte(r.CapJSON), &caps); err != nil {
			return nil, err
		}
	}
	issued, err := time.Parse(time.RFC3339Nano, r.IssuedAt)
	if err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	if err != nil {
		return nil, err
	}
	rec := &NipCertRecord{
		Nid:          r.Nid,
		EntityType:   r.EntityType,
		Serial:       r.Serial,
		PubKey:       r.PubKey,
		Capabilities: caps,
		ScopeJson:    r.ScopeJSON,
		IssuedBy:     r.IssuedBy,
		IssuedAt:     issued.UTC(),
		ExpiresAt:    expires.UTC(),
		RevokeReason: nullToPtr(r.RevokeReason),
		MetadataJson: nullToPtr(r.MetadataJSON),
		NidRole:      nullToPtr(r.NidRole),
		ParentNid:    nullToPtr(r.ParentNid),
		LineageJson:  nullToPtr(r.LineageJSON),
	}
	if r.RevokedAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, r.RevokedAt.String); err == nil {
			tu := t.UTC()
			rec.RevokedAt = &tu
		}
	}
	return rec, nil
}

func (s *SqlNipCaStore) queryFirst(q string, args ...any) (*NipCertRecord, error) {
	var found *NipCertRecord
	err := s.exec.QueryRows(q, args, func(sc RowScanner) error {
		if found != nil {
			return nil
		}
		var r certRow
		if err := sc.Scan(s.scanTargets(&r)...); err != nil {
			return err
		}
		rec, err := s.mapRow(&r)
		if err != nil {
			return err
		}
		found = rec
		return nil
	})
	return found, err
}

func (s *SqlNipCaStore) queryMany(q string, args ...any) ([]*NipCertRecord, error) {
	var out []*NipCertRecord
	err := s.exec.QueryRows(q, args, func(sc RowScanner) error {
		var r certRow
		if err := sc.Scan(s.scanTargets(&r)...); err != nil {
			return err
		}
		rec, err := s.mapRow(&r)
		if err != nil {
			return err
		}
		out = append(out, rec)
		return nil
	})
	return out, err
}

func nullString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

// ── database/sql binding ──────────────────────────────────────────────────────

// SqlDBExecutor adapts a *database/sql.DB to SqlCaExecutor. The host registers a
// concrete driver and supplies the *sql.DB, keeping the SDK dependency-free.
type SqlDBExecutor struct {
	DB *sql.DB
}

// NewSqlDBExecutor wraps a *sql.DB.
func NewSqlDBExecutor(db *sql.DB) *SqlDBExecutor { return &SqlDBExecutor{DB: db} }

// Exec runs a non-row statement and returns rows affected.
func (e *SqlDBExecutor) Exec(query string, args ...any) (int64, error) {
	res, err := e.DB.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil // some drivers don't report; treat as unknown-but-ok
	}
	return n, nil
}

// QueryRows runs a row-returning query, invoking scan per row.
func (e *SqlDBExecutor) QueryRows(query string, args []any, scan func(RowScanner) error) error {
	rows, err := e.DB.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// QueryScalarInt64 runs a query returning a single int64.
func (e *SqlDBExecutor) QueryScalarInt64(query string, args ...any) (int64, error) {
	var n int64
	err := e.DB.QueryRow(query, args...).Scan(&n)
	return n, err
}
