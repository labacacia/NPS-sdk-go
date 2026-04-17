// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "embed"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed sql/001_init.sql
var schema string

// CertRecord mirrors a row in nip_certificates.
type CertRecord struct {
	ID           int64
	NID          string
	EntityType   string
	Serial       string
	PubKey       string
	Capabilities []string
	Scope        map[string]any
	IssuedBy     string
	IssuedAt     string
	ExpiresAt    string
	RevokedAt    *string
	RevokeReason *string
	Metadata     map[string]any
}

// InsertRec is the input for Insert.
type InsertRec struct {
	NID          string
	EntityType   string
	Serial       string
	PubKey       string
	Capabilities []string
	Scope        map[string]any
	IssuedBy     string
	IssuedAt     string
	ExpiresAt    string
	Metadata     map[string]any
}

// CaDb wraps a SQLite connection with a mutex for safe concurrent use.
type CaDb struct {
	mu sync.Mutex
	db *sql.DB
}

// Open opens (or creates) the SQLite DB at path and runs the schema migration.
func Open(path string) (*CaDb, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1) // SQLite: serialize writes
	if _, err := conn.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		return nil, err
	}
	if _, err := conn.Exec(schema); err != nil {
		return nil, err
	}
	return &CaDb{db: conn}, nil
}

// IsoNow returns the current UTC time in RFC 3339 format.
func IsoNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// NextSerial returns the next hex serial number ("0xNNNNNN").
func (d *CaDb) NextSerial() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var n int64
	err := d.db.QueryRow(
		"SELECT COALESCE(MAX(CAST(REPLACE(serial,'0x','') AS INTEGER)),0)+1 FROM nip_certificates",
	).Scan(&n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("0x%06X", n), nil
}

// Insert adds a new certificate record.
func (d *CaDb) Insert(rec *InsertRec) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	caps, _ := json.Marshal(rec.Capabilities)
	scope, _ := json.Marshal(rec.Scope)
	var meta *string
	if rec.Metadata != nil {
		b, _ := json.Marshal(rec.Metadata)
		s := string(b)
		meta = &s
	}
	res, err := d.db.Exec(
		`INSERT INTO nip_certificates
		 (nid,entity_type,serial,pub_key,capabilities,scope_json,issued_by,issued_at,expires_at,metadata_json)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		rec.NID, rec.EntityType, rec.Serial, rec.PubKey,
		string(caps), string(scope),
		rec.IssuedBy, rec.IssuedAt, rec.ExpiresAt, meta,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetActive returns the most-recent non-revoked certificate for nid, or nil.
func (d *CaDb) GetActive(nid string) (*CertRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	row := d.db.QueryRow(
		`SELECT id,nid,entity_type,serial,pub_key,capabilities,scope_json,
		        issued_by,issued_at,expires_at,revoked_at,revoke_reason,metadata_json
		 FROM nip_certificates
		 WHERE nid=? AND revoked_at IS NULL
		 ORDER BY issued_at DESC LIMIT 1`,
		nid,
	)
	return scanRecord(row)
}

// Revoke marks the active certificate for nid as revoked. Returns false if not found.
func (d *CaDb) Revoke(nid, reason string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	res, err := d.db.Exec(
		`UPDATE nip_certificates SET revoked_at=?,revoke_reason=?
		 WHERE nid=? AND revoked_at IS NULL`,
		IsoNow(), reason, nid,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CRL returns all revoked certificate entries.
func (d *CaDb) CRL() ([]map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := d.db.Query(
		`SELECT serial,nid,revoked_at,revoke_reason
		 FROM nip_certificates WHERE revoked_at IS NOT NULL
		 ORDER BY revoked_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var serial, nid, revokedAt string
		var reason *string
		if err := rows.Scan(&serial, &nid, &revokedAt, &reason); err != nil {
			continue
		}
		entry := map[string]any{
			"serial":        serial,
			"nid":           nid,
			"revoked_at":    revokedAt,
			"revoke_reason": reason,
		}
		result = append(result, entry)
	}
	return result, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func scanRecord(row *sql.Row) (*CertRecord, error) {
	var rec CertRecord
	var capsJSON, scopeJSON string
	var metaJSON *string
	err := row.Scan(
		&rec.ID, &rec.NID, &rec.EntityType, &rec.Serial, &rec.PubKey,
		&capsJSON, &scopeJSON,
		&rec.IssuedBy, &rec.IssuedAt, &rec.ExpiresAt,
		&rec.RevokedAt, &rec.RevokeReason, &metaJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(capsJSON), &rec.Capabilities)
	_ = json.Unmarshal([]byte(scopeJSON), &rec.Scope)
	if metaJSON != nil {
		_ = json.Unmarshal([]byte(*metaJSON), &rec.Metadata)
	}
	return &rec, nil
}
