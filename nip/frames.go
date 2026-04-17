// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import "github.com/labacacia/nps/impl/go/core"

func str(d core.FrameDict, k string) string {
	if v, ok := d[k].(string); ok { return v }
	return ""
}
func optStr(d core.FrameDict, k string) *string {
	if v, ok := d[k].(string); ok { return &v }
	return nil
}

// ── IdentFrame ────────────────────────────────────────────────────────────────

type IdentFrame struct {
	NID       string
	PubKey    string
	Meta      map[string]any
	Signature *string
}

func (f *IdentFrame) FrameType() core.FrameType { return core.FrameTypeIdent }

// UnsignedDict returns the dict without the signature field (for signing).
func (f *IdentFrame) UnsignedDict() core.FrameDict {
	d := core.FrameDict{"nid": f.NID, "pub_key": f.PubKey}
	if f.Meta != nil { d["meta"] = f.Meta }
	return d
}

func (f *IdentFrame) ToDict() core.FrameDict {
	d := f.UnsignedDict()
	if f.Signature != nil { d["signature"] = *f.Signature }
	return d
}

func IdentFrameFromDict(d core.FrameDict) *IdentFrame {
	var meta map[string]any
	if v, ok := d["meta"].(map[string]any); ok { meta = v }
	return &IdentFrame{
		NID:       str(d, "nid"),
		PubKey:    str(d, "pub_key"),
		Meta:      meta,
		Signature: optStr(d, "signature"),
	}
}

// ── TrustFrame ────────────────────────────────────────────────────────────────

type TrustFrame struct {
	IssuerNID  string
	SubjectNID string
	Scopes     []string
	ExpiresAt  *string
	Signature  *string
}

func (f *TrustFrame) FrameType() core.FrameType { return core.FrameTypeTrust }

func (f *TrustFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"issuer_nid": f.IssuerNID, "subject_nid": f.SubjectNID, "scopes": f.Scopes}
	if f.ExpiresAt != nil { d["expires_at"] = *f.ExpiresAt }
	if f.Signature != nil { d["signature"] = *f.Signature }
	return d
}

func TrustFrameFromDict(d core.FrameDict) *TrustFrame {
	var scopes []string
	switch v := d["scopes"].(type) {
	case []string:
		scopes = v
	case []any:
		for _, s := range v {
			if sv, ok := s.(string); ok { scopes = append(scopes, sv) }
		}
	}
	return &TrustFrame{
		IssuerNID:  str(d, "issuer_nid"),
		SubjectNID: str(d, "subject_nid"),
		Scopes:     scopes,
		ExpiresAt:  optStr(d, "expires_at"),
		Signature:  optStr(d, "signature"),
	}
}

// ── RevokeFrame ───────────────────────────────────────────────────────────────

type RevokeFrame struct {
	NID       string
	Reason    *string
	RevokedAt *string
}

func (f *RevokeFrame) FrameType() core.FrameType { return core.FrameTypeRevoke }

func (f *RevokeFrame) ToDict() core.FrameDict {
	d := core.FrameDict{"nid": f.NID}
	if f.Reason    != nil { d["reason"]     = *f.Reason }
	if f.RevokedAt != nil { d["revoked_at"] = *f.RevokedAt }
	return d
}

func RevokeFrameFromDict(d core.FrameDict) *RevokeFrame {
	return &RevokeFrame{
		NID:       str(d, "nid"),
		Reason:    optStr(d, "reason"),
		RevokedAt: optStr(d, "revoked_at"),
	}
}
