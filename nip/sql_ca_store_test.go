// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package nip

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// memRow holds the raw string column values in nip_certs declaration order.
type memRow struct {
	nid, entityType, serial, pubKey, capJSON, scopeJSON string
	issuedBy, issuedAt, expiresAt                       string
	revokedAt, revokeReason, metadataJSON               *string
	nidRole, parentNid, lineageJSON                     *string
}

// memCaExecutor is a tiny in-memory SqlCaExecutor that understands the exact
// statement set the SqlNipCaStore issues. It lets us round-trip records without
// a database driver (none is in the module cache).
type memCaExecutor struct {
	rows []*memRow
	seq  int64
}

type memScanner struct{ vals []any }

func (m *memScanner) Scan(dest ...any) error {
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = m.vals[i].(string)
		default:
			// sql.NullString
			assignNull(d, m.vals[i])
		}
	}
	return nil
}

func assignNull(dest, v any) {
	// dest is *sql.NullString
	ns := dest.(interface{ Scan(any) error })
	if v == nil {
		_ = ns.Scan(nil)
	} else {
		_ = ns.Scan(v.(string))
	}
}

func (e *memCaExecutor) Exec(query string, args ...any) (int64, error) {
	q := strings.TrimSpace(query)
	switch {
	case strings.HasPrefix(q, "CREATE"), strings.HasPrefix(q, "INSERT OR IGNORE"):
		return 0, nil
	case strings.HasPrefix(q, "INSERT INTO nip_certs"):
		r := &memRow{
			nid: args[0].(string), entityType: args[1].(string), serial: args[2].(string),
			pubKey: args[3].(string), capJSON: args[4].(string), scopeJSON: args[5].(string),
			issuedBy: args[6].(string), issuedAt: args[7].(string), expiresAt: args[8].(string),
			metadataJSON: toOptStr(args[9]), nidRole: toOptStr(args[10]),
			parentNid: toOptStr(args[11]), lineageJSON: toOptStr(args[12]),
		}
		e.rows = append(e.rows, r)
		return 1, nil
	case strings.HasPrefix(q, "UPDATE nip_serial"):
		e.seq++
		return 1, nil
	case strings.HasPrefix(q, "UPDATE nip_certs SET revoked_at"):
		revokedAt := args[0].(string)
		reason := args[1].(string)
		nid := args[2].(string)
		for _, r := range e.rows {
			if r.nid == nid && r.revokedAt == nil {
				ra, rs := revokedAt, reason
				r.revokedAt, r.revokeReason = &ra, &rs
				return 1, nil
			}
		}
		return 0, nil
	}
	return 0, nil
}

func (e *memCaExecutor) QueryScalarInt64(query string, args ...any) (int64, error) {
	return e.seq, nil
}

func (e *memCaExecutor) QueryRows(query string, args []any, scan func(RowScanner) error) error {
	q := strings.TrimSpace(query)
	var matched []*memRow
	switch {
	case strings.Contains(q, "WHERE nid = ?"):
		for _, r := range e.rows {
			if r.nid == args[0].(string) {
				matched = append(matched, r)
			}
		}
		sort.SliceStable(matched, func(i, j int) bool { return matched[i].issuedAt > matched[j].issuedAt })
		if len(matched) > 1 {
			matched = matched[:1]
		}
	case strings.Contains(q, "WHERE serial = ?"):
		for _, r := range e.rows {
			if r.serial == args[0].(string) {
				matched = append(matched, r)
			}
		}
	case strings.Contains(q, "WHERE parent_nid = ?"):
		for _, r := range e.rows {
			if r.parentNid != nil && *r.parentNid == args[0].(string) {
				matched = append(matched, r)
			}
		}
	case strings.Contains(q, "revoked_at IS NOT NULL"):
		for _, r := range e.rows {
			if r.revokedAt != nil {
				matched = append(matched, r)
			}
		}
	default: // List
		matched = append(matched, e.rows...)
	}
	for _, r := range matched {
		sc := &memScanner{vals: []any{
			r.nid, r.entityType, r.serial, r.pubKey, r.capJSON, r.scopeJSON,
			r.issuedBy, r.issuedAt, r.expiresAt,
			deref(r.revokedAt), deref(r.revokeReason), deref(r.metadataJSON),
			deref(r.nidRole), deref(r.parentNid), deref(r.lineageJSON),
		}}
		if err := scan(sc); err != nil {
			return err
		}
	}
	return nil
}

func toOptStr(v any) *string {
	if v == nil {
		return nil
	}
	s := v.(string)
	return &s
}
func deref(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func newTestStore(t *testing.T) *SqlNipCaStore {
	t.Helper()
	s, err := NewSqlNipCaStore(&memCaExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSqlCaStore_SaveGetRoundtrip(t *testing.T) {
	s := newTestStore(t)
	issued := time.Now().UTC().Truncate(time.Second)
	rec := &NipCertRecord{
		Nid: "urn:nps:agent:a1", EntityType: "agent", Serial: "0x1",
		PubKey: "PK", Capabilities: []string{"read", "write"}, ScopeJson: "{}",
		IssuedBy: "ca", IssuedAt: issued, ExpiresAt: issued.Add(time.Hour),
	}
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByNid("urn:nps:agent:a1")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Serial != "0x1" || len(got.Capabilities) != 2 || got.Capabilities[0] != "read" {
		t.Fatalf("got=%+v", got)
	}
	if !got.IssuedAt.Equal(issued) {
		t.Fatalf("issuedAt=%v want %v", got.IssuedAt, issued)
	}

	bySerial, _ := s.GetBySerial("0x1")
	if bySerial == nil || bySerial.Nid != rec.Nid {
		t.Fatalf("bySerial=%+v", bySerial)
	}
}

func TestSqlCaStore_Revoke(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	_ = s.Save(&NipCertRecord{Nid: "n1", EntityType: "agent", Serial: "0x2", PubKey: "P",
		Capabilities: []string{}, ScopeJson: "{}", IssuedBy: "ca", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	ok, err := s.Revoke("n1", "compromise", now)
	if err != nil || !ok {
		t.Fatalf("revoke ok=%v err=%v", ok, err)
	}
	revoked, _ := s.GetRevoked()
	if len(revoked) != 1 || revoked[0].RevokeReason == nil || *revoked[0].RevokeReason != "compromise" {
		t.Fatalf("revoked=%+v", revoked)
	}
	// Second revoke of same NID → false.
	ok2, _ := s.Revoke("n1", "again", now)
	if ok2 {
		t.Fatal("double revoke should be false")
	}
}

func TestSqlCaStore_NextSerial_Hex(t *testing.T) {
	s := newTestStore(t)
	s1, _ := s.NextSerial()
	s2, _ := s.NextSerial()
	if s1 != "0x1" || s2 != "0x2" {
		t.Fatalf("serials %s %s", s1, s2)
	}
}

func TestSqlCaStore_GetByParentNid(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	parent := "urn:nps:group:g1"
	sess := &NipCertRecord{Nid: "sess1", EntityType: "agent", Serial: "0x3", PubKey: "P",
		Capabilities: []string{}, ScopeJson: "{}", IssuedBy: "ca", IssuedAt: now,
		ExpiresAt: now.Add(time.Hour), ParentNid: &parent}
	_ = s.Save(sess)
	kids, _ := s.GetByParentNid(parent)
	if len(kids) != 1 || kids[0].Nid != "sess1" {
		t.Fatalf("kids=%+v", kids)
	}
}

func TestMigrationStatements_Shape(t *testing.T) {
	stmts := MigrationStatements()
	if len(stmts) != 6 {
		t.Fatalf("stmts=%d", len(stmts))
	}
	if !strings.Contains(stmts[0], "CREATE TABLE IF NOT EXISTS nip_certs") {
		t.Fatalf("stmt0=%q", stmts[0])
	}
	if !strings.Contains(stmts[0], "parent_nid") || !strings.Contains(stmts[0], "lineage_json") {
		t.Fatalf("missing CR-0003 columns")
	}
}
