// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip

import "github.com/labacacia/NPS-sdk-go/core"

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

	// NPS-RFC-0003 — optional assurance level.
	AssuranceLevel *AssuranceLevel
	// NPS-RFC-0002 — optional v2 X.509 dual-trust extensions.
	// CertFormat is the wire form (CertFormatV1Proprietary | CertFormatV2X509).
	CertFormat *string
	// CertChain is base64url(DER), [leaf, intermediates..., root].
	CertChain []string
}

func (f *IdentFrame) FrameType() core.FrameType { return core.FrameTypeIdent }

// UnsignedDict returns the dict the v1 Ed25519 signature covers.
// Deliberately excludes cert_format / cert_chain so the v1 sig stays
// covering exactly the same payload as before NPS-RFC-0002.
func (f *IdentFrame) UnsignedDict() core.FrameDict {
	d := core.FrameDict{"nid": f.NID, "pub_key": f.PubKey}
	if f.Meta != nil { d["metadata"] = f.Meta }
	if f.AssuranceLevel != nil { d["assurance_level"] = f.AssuranceLevel.Wire }
	return d
}

func (f *IdentFrame) ToDict() core.FrameDict {
	d := f.UnsignedDict()
	if f.Signature  != nil { d["signature"]   = *f.Signature }
	if f.CertFormat != nil { d["cert_format"] = *f.CertFormat }
	if f.CertChain  != nil { d["cert_chain"]  = f.CertChain }
	return d
}

func IdentFrameFromDict(d core.FrameDict) *IdentFrame {
	var meta map[string]any
	if v, ok := d["metadata"].(map[string]any); ok { meta = v }
	var assurance *AssuranceLevel
	if v, ok := d["assurance_level"].(string); ok {
		if l, err := AssuranceFromWire(v); err == nil {
			assurance = &l
		}
	}
	var chain []string
	switch v := d["cert_chain"].(type) {
	case []string:
		chain = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok { chain = append(chain, s) }
		}
	}
	return &IdentFrame{
		NID:            str(d, "nid"),
		PubKey:         str(d, "pub_key"),
		Meta:           meta,
		Signature:      optStr(d, "signature"),
		AssuranceLevel: assurance,
		CertFormat:     optStr(d, "cert_format"),
		CertChain:      chain,
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
