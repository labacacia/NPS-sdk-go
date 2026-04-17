// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nip-ca-server/ca"
	"nip-ca-server/db"
)

// State holds shared application state.
type State struct {
	SK          ed25519.PrivateKey
	PubKeyStr   string
	DB          *db.CaDb
	CaNID       string
	BaseURL     string
	DisplayName string
	AgentDays   int
	NodeDays    int
	RenewalDays int
}

// Router builds and returns the HTTP mux.
func Router(s *State) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/agents/register", s.registerAgent)
	mux.HandleFunc("POST /v1/nodes/register", s.registerNode)
	mux.HandleFunc("POST /v1/agents/{nid}/renew", s.renew)
	mux.HandleFunc("POST /v1/agents/{nid}/revoke", s.revoke)
	mux.HandleFunc("GET /v1/agents/{nid}/verify", s.verify)
	mux.HandleFunc("GET /v1/ca/cert", s.caCert)
	mux.HandleFunc("GET /v1/crl", s.crl)
	mux.HandleFunc("GET /.well-known/nps-ca", s.wellKnown)
	mux.HandleFunc("GET /health", health)
	return mux
}

// ── Handlers ──────────────────────────────────────────────────────────────────

type registerReq struct {
	NID          *string        `json:"nid"`
	PubKey       string         `json:"pub_key"`
	Capabilities []string       `json:"capabilities"`
	Scope        map[string]any `json:"scope"`
	Metadata     map[string]any `json:"metadata"`
}

type revokeReq struct {
	Reason *string `json:"reason"`
}

func (s *State) registerAgent(w http.ResponseWriter, r *http.Request) {
	s.register(w, r, "agent")
}

func (s *State) registerNode(w http.ResponseWriter, r *http.Request) {
	s.register(w, r, "node")
}

func (s *State) register(w http.ResponseWriter, r *http.Request, entityType string) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "NIP-CA-BAD-REQUEST", "invalid JSON body")
		return
	}

	domain := caDomain(s.CaNID)
	nid := ""
	if req.NID != nil {
		nid = *req.NID
	} else {
		nid = ca.GenerateNID(domain, entityType)
	}

	existing, err := s.DB.GetActive(nid)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "NIP-CA-INTERNAL", err.Error())
		return
	}
	if existing != nil {
		jsonErr(w, http.StatusConflict, "NIP-CA-NID-ALREADY-EXISTS",
			nid+" already has an active certificate")
		return
	}

	days := s.AgentDays
	if entityType == "node" {
		days = s.NodeDays
	}

	serial, err := s.DB.NextSerial()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "NIP-CA-INTERNAL", err.Error())
		return
	}

	cert := ca.IssueCert(s.SK, s.CaNID, nid, req.PubKey,
		req.Capabilities, req.Scope, days, serial, req.Metadata)

	issuedAt, _ := cert["issued_at"].(string)
	expiresAt, _ := cert["expires_at"].(string)

	_, _ = s.DB.Insert(&db.InsertRec{
		NID: nid, EntityType: entityType, Serial: serial,
		PubKey: req.PubKey, Capabilities: req.Capabilities, Scope: req.Scope,
		IssuedBy: s.CaNID, IssuedAt: issuedAt, ExpiresAt: expiresAt, Metadata: req.Metadata,
	})

	jsonResp(w, http.StatusCreated, map[string]any{
		"nid": nid, "serial": serial,
		"issued_at": issuedAt, "expires_at": expiresAt,
		"ident_frame": cert,
	})
}

func (s *State) renew(w http.ResponseWriter, r *http.Request) {
	nid := r.PathValue("nid")
	rec, err := s.DB.GetActive(nid)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "NIP-CA-INTERNAL", err.Error())
		return
	}
	if rec == nil {
		jsonErr(w, http.StatusNotFound, "NIP-CA-NID-NOT-FOUND", nid+" not found")
		return
	}

	exp, err := time.Parse(time.RFC3339, rec.ExpiresAt)
	if err == nil {
		daysLeft := int(time.Until(exp).Hours() / 24)
		if daysLeft > s.RenewalDays {
			jsonErr(w, http.StatusBadRequest, "NIP-CA-RENEWAL-TOO-EARLY",
				"Renewal window opens in "+itoa(daysLeft-s.RenewalDays)+" days")
			return
		}
	}

	days := s.AgentDays
	if rec.EntityType == "node" {
		days = s.NodeDays
	}

	serial, err := s.DB.NextSerial()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "NIP-CA-INTERNAL", err.Error())
		return
	}

	cert := ca.IssueCert(s.SK, s.CaNID, nid, rec.PubKey,
		rec.Capabilities, rec.Scope, days, serial, rec.Metadata)

	issuedAt, _ := cert["issued_at"].(string)
	expiresAt, _ := cert["expires_at"].(string)

	_, _ = s.DB.Insert(&db.InsertRec{
		NID: nid, EntityType: rec.EntityType, Serial: serial,
		PubKey: rec.PubKey, Capabilities: rec.Capabilities, Scope: rec.Scope,
		IssuedBy: s.CaNID, IssuedAt: issuedAt, ExpiresAt: expiresAt, Metadata: rec.Metadata,
	})

	jsonResp(w, http.StatusOK, map[string]any{
		"nid": nid, "serial": serial,
		"issued_at": issuedAt, "expires_at": expiresAt,
		"ident_frame": cert,
	})
}

func (s *State) revoke(w http.ResponseWriter, r *http.Request) {
	nid := r.PathValue("nid")

	reason := "cessation_of_operation"
	var req revokeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Reason != nil {
		reason = *req.Reason
	}

	ok, err := s.DB.Revoke(nid, reason)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "NIP-CA-INTERNAL", err.Error())
		return
	}
	if !ok {
		jsonErr(w, http.StatusNotFound, "NIP-CA-NID-NOT-FOUND", nid+" not found")
		return
	}
	jsonResp(w, http.StatusOK, map[string]any{
		"nid": nid, "revoked_at": db.IsoNow(), "reason": reason,
	})
}

func (s *State) verify(w http.ResponseWriter, r *http.Request) {
	nid := r.PathValue("nid")
	rec, err := s.DB.GetActive(nid)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "NIP-CA-INTERNAL", err.Error())
		return
	}
	if rec == nil {
		jsonErr(w, http.StatusNotFound, "NIP-CA-NID-NOT-FOUND", nid+" not found")
		return
	}

	exp, _ := time.Parse(time.RFC3339, rec.ExpiresAt)
	valid := time.Now().UTC().Before(exp)

	resp := map[string]any{
		"valid": valid, "nid": nid, "entity_type": rec.EntityType,
		"pub_key": rec.PubKey, "capabilities": rec.Capabilities,
		"issued_by": rec.IssuedBy, "issued_at": rec.IssuedAt,
		"expires_at": rec.ExpiresAt, "serial": rec.Serial,
	}
	if !valid {
		resp["error_code"] = "NIP-CERT-EXPIRED"
	}
	jsonResp(w, http.StatusOK, resp)
}

func (s *State) caCert(w http.ResponseWriter, _ *http.Request) {
	jsonResp(w, http.StatusOK, map[string]any{
		"nid": s.CaNID, "display_name": s.DisplayName,
		"pub_key": s.PubKeyStr, "algorithm": "ed25519",
	})
}

func (s *State) crl(w http.ResponseWriter, _ *http.Request) {
	entries, _ := s.DB.CRL()
	if entries == nil {
		entries = []map[string]any{}
	}
	jsonResp(w, http.StatusOK, map[string]any{"revoked": entries})
}

func (s *State) wellKnown(w http.ResponseWriter, _ *http.Request) {
	base := strings.TrimRight(s.BaseURL, "/")
	jsonResp(w, http.StatusOK, map[string]any{
		"nps_ca": "0.1", "issuer": s.CaNID, "display_name": s.DisplayName,
		"public_key": s.PubKeyStr, "algorithms": []string{"ed25519"},
		"endpoints": map[string]any{
			"register": base + "/v1/agents/register",
			"verify":   base + "/v1/agents/{nid}/verify",
			"ocsp":     base + "/v1/agents/{nid}/verify",
			"crl":      base + "/v1/crl",
		},
		"capabilities":           []string{"agent", "node"},
		"max_cert_validity_days": max(s.AgentDays, s.NodeDays),
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	jsonResp(w, http.StatusOK, map[string]any{"status": "ok"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, errCode, msg string) {
	jsonResp(w, code, map[string]any{"error_code": errCode, "message": msg})
}

func caDomain(caNID string) string {
	parts := strings.Split(caNID, ":")
	if len(parts) >= 5 {
		return parts[len(parts)-2]
	}
	return "ca.local"
}

func itoa(n int) string { return strconv.Itoa(n) }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
