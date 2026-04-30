// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nip_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	cryptox509 "crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	npsnip "github.com/labacacia/NPS-sdk-go/nip"
	"github.com/labacacia/NPS-sdk-go/nip/acme"
)

// Go parallel of .NET / Java / Python AcmeAgent01Tests per NPS-RFC-0002 §4.4.
// End-to-end agent-01 round-trip plus tampered-signature negative path.

type acmeFixture struct {
	caNid    string
	agentNid string
	caRoot   *cryptox509.Certificate
	agentPub ed25519.PublicKey
	agentPriv ed25519.PrivateKey
	server   *acme.Server
}

func newAcmeFixture(t *testing.T) *acmeFixture {
	t.Helper()
	caNid    := "urn:nps:ca:acme-test"
	agentNid := "urn:nps:agent:acme-test:1"

	_, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatalf("ca keygen: %v", err) }

	caRoot := mustIssueRoot(t, caPriv, caNid, big.NewInt(1))

	agentPub, agentPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatalf("agent keygen: %v", err) }

	srv := acme.NewServer(acme.ServerOptions{
		CaNid: caNid, CaKey: caPriv, CaRootCert: caRoot,
		CertValidity: 30 * 24 * time.Hour,
	})
	if err := srv.Start(); err != nil { t.Fatalf("server start: %v", err) }
	t.Cleanup(func() { _ = srv.Close() })

	return &acmeFixture{
		caNid: caNid, agentNid: agentNid, caRoot: caRoot,
		agentPub: agentPub, agentPriv: agentPriv, server: srv,
	}
}

func TestIssueAgentCert_RoundTrip_VerifiesAgainstRoot(t *testing.T) {
	fx := newAcmeFixture(t)

	client := acme.NewClient(fx.server.DirectoryURL(), fx.agentPriv)
	pemStr, err := client.IssueAgentCert(fx.agentNid)
	if err != nil {
		t.Fatalf("IssueAgentCert: %v", err)
	}
	if !bytes.Contains([]byte(pemStr), []byte("BEGIN CERTIFICATE")) {
		t.Fatalf("PEM missing certificate marker")
	}

	// Parse PEM chain into base64url DER list and verify against the trusted root.
	chainB64 := parsePemChainBase64Url(t, pemStr)
	if len(chainB64) == 0 {
		t.Fatalf("PEM chain empty")
	}
	asserted := npsnip.AssuranceAnonymous
	ok, errCode, errMsg := x509Adapter(chainB64, fx.agentNid, &asserted, []*cryptox509.Certificate{fx.caRoot})
	if !ok {
		t.Fatalf("leaf must verify; got code=%s msg=%s", errCode, errMsg)
	}
}

func TestRespondAgent01_TamperedSignature_ServerReturnsChallengeFailed(t *testing.T) {
	fx := newAcmeFixture(t)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	// Step 1: directory + nonce.
	dir := getDirectory(t, httpClient, fx.server.DirectoryURL())
	nonce := getNonce(t, httpClient, dir.NewNonce)

	// newAccount.
	jwk, err := acme.JwkFromPublicKey(fx.agentPub)
	if err != nil { t.Fatalf("jwk: %v", err) }
	acctEnv, err := acme.Sign(
		acme.ProtectedHeader{Alg: acme.AlgEdDSA, Nonce: nonce, URL: dir.NewAccount, JWK: jwk},
		acme.NewAccountPayload{TermsOfServiceAgreed: true},
		fx.agentPriv)
	if err != nil { t.Fatalf("sign acct: %v", err) }
	acctResp, acctNonce := postJoseExpect(t, httpClient, dir.NewAccount, acctEnv, 201)
	accountUrl := acctResp.Header.Get("Location")
	nonce = acctNonce

	// newOrder.
	orderEnv, err := acme.Sign(
		acme.ProtectedHeader{Alg: acme.AlgEdDSA, Nonce: nonce, URL: dir.NewOrder, Kid: accountUrl},
		acme.NewOrderPayload{Identifiers: []acme.Identifier{{Type: acme.IdentifierTypeNID, Value: fx.agentNid}}},
		fx.agentPriv)
	if err != nil { t.Fatalf("sign order: %v", err) }
	orderHTTP, orderNonce := postJoseExpect(t, httpClient, dir.NewOrder, orderEnv, 201)
	var order acme.Order
	mustDecodeJSON(t, orderHTTP, &order)
	nonce = orderNonce

	// POST-as-GET on authz.
	authzEnv, err := acme.Sign(
		acme.ProtectedHeader{Alg: acme.AlgEdDSA, Nonce: nonce, URL: order.Authorizations[0], Kid: accountUrl},
		nil, fx.agentPriv)
	if err != nil { t.Fatalf("sign authz: %v", err) }
	authzHTTP, authzNonce := postJoseExpect(t, httpClient, order.Authorizations[0], authzEnv, 200)
	var authz acme.Authorization
	mustDecodeJSON(t, authzHTTP, &authz)
	nonce = authzNonce

	var ch *acme.Challenge
	for i := range authz.Challenges {
		if authz.Challenges[i].Type == acme.ChallengeAgent01 {
			ch = &authz.Challenges[i]
			break
		}
	}
	if ch == nil { t.Fatalf("authz missing agent-01 challenge") }

	// ★ Tampered: sign challenge token with a *different* keypair, but submit
	//   the JWS envelope under the registered account's key. The server verifies
	//   the JWS sig (passes with account key) and then verifies the agent
	//   signature against the same account key (fails).
	_, forgerPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatalf("forger keygen: %v", err) }
	forgedSig := ed25519.Sign(forgerPriv, []byte(ch.Token))

	chEnv, err := acme.Sign(
		acme.ProtectedHeader{Alg: acme.AlgEdDSA, Nonce: nonce, URL: ch.URL, Kid: accountUrl},
		acme.ChallengeRespondPayload{AgentSignature: acme.B64uEncode(forgedSig)},
		fx.agentPriv)
	if err != nil { t.Fatalf("sign challenge: %v", err) }

	body, _ := json.Marshal(chEnv)
	req, _ := http.NewRequest("POST", ch.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", acme.ContentTypeJoseJSON)
	resp, err := httpClient.Do(req)
	if err != nil { t.Fatalf("POST challenge: %v", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("want HTTP 400, got %d", resp.StatusCode)
	}
	var problem acme.ProblemDetail
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Type != npsnip.ErrAcmeChallengeFailed {
		t.Fatalf("want type %s, got %s (detail=%s)",
			npsnip.ErrAcmeChallengeFailed, problem.Type, problem.Detail)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func getDirectory(t *testing.T, c *http.Client, url string) *acme.Directory {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil { t.Fatalf("GET directory: %v", err) }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { t.Fatalf("directory HTTP %d", resp.StatusCode) }
	var dir acme.Directory
	if err := json.NewDecoder(resp.Body).Decode(&dir); err != nil {
		t.Fatalf("decode dir: %v", err)
	}
	return &dir
}

func getNonce(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	req, _ := http.NewRequest("HEAD", url, nil)
	resp, err := c.Do(req)
	if err != nil { t.Fatalf("HEAD newNonce: %v", err) }
	defer resp.Body.Close()
	n := resp.Header.Get("Replay-Nonce")
	if n == "" { t.Fatalf("server omitted Replay-Nonce") }
	return n
}

func postJoseExpect(t *testing.T, c *http.Client, url string, env *acme.Envelope, want int) (*http.Response, string) {
	t.Helper()
	body, _ := json.Marshal(env)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", acme.ContentTypeJoseJSON)
	resp, err := c.Do(req)
	if err != nil { t.Fatalf("POST %s: %v", url, err) }
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST %s want %d got %d: %s", url, want, resp.StatusCode, string(body))
	}
	nonce := resp.Header.Get("Replay-Nonce")
	return resp, nonce
}

func mustDecodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func parsePemChainBase64Url(t *testing.T, pemStr string) []string {
	t.Helper()
	var out []string
	rest := []byte(pemStr)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			out = append(out, b64uEncode(block.Bytes))
		}
	}
	return out
}

func b64uEncode(b []byte) string {
	return acme.B64uEncode(b)
}
