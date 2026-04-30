// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package acme

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	cryptox509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client — ACME client implementing the agent-01 challenge per NPS-RFC-0002 §4.4.
//
// Flow: newNonce → newAccount → newOrder → fetch authz → sign challenge token →
// finalize with CSR → fetch leaf cert.
type Client struct {
	HTTPClient   *http.Client
	DirectoryURL string
	PrivateKey   ed25519.PrivateKey
	PublicKey    ed25519.PublicKey

	directory  *Directory
	accountUrl string
	lastNonce  string
}

// NewClient constructs a Client with sensible defaults.
func NewClient(directoryURL string, priv ed25519.PrivateKey) *Client {
	return &Client{
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		DirectoryURL: directoryURL,
		PrivateKey:   priv,
		PublicKey:    priv.Public().(ed25519.PublicKey),
	}
}

// IssueAgentCert drives the full agent-01 flow for nid. Returns issued PEM chain.
func (c *Client) IssueAgentCert(nid string) (string, error) {
	if err := c.ensureDirectory(); err != nil {
		return "", err
	}
	if c.accountUrl == "" {
		if err := c.newAccount(); err != nil {
			return "", err
		}
	}
	order, err := c.newOrder(nid)
	if err != nil {
		return "", err
	}
	authz, err := c.fetchAuthz(order.Authorizations[0])
	if err != nil {
		return "", err
	}
	if err := c.respondAgent01(authz); err != nil {
		return "", err
	}
	finalized, err := c.finalizeOrder(order, nid)
	if err != nil {
		return "", err
	}
	return c.downloadPem(finalized.Certificate)
}

// ── Stages ─────────────────────────────────────────────────────────────────

func (c *Client) ensureDirectory() error {
	if c.directory != nil {
		return nil
	}
	resp, err := c.HTTPClient.Get(c.DirectoryURL)
	if err != nil {
		return fmt.Errorf("get directory: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("get directory: HTTP %d", resp.StatusCode)
	}
	var dir Directory
	if err := json.NewDecoder(resp.Body).Decode(&dir); err != nil {
		return fmt.Errorf("decode directory: %w", err)
	}
	c.directory = &dir
	return c.refreshNonce()
}

func (c *Client) refreshNonce() error {
	req, err := http.NewRequest("HEAD", c.directory.NewNonce, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD newNonce: %w", err)
	}
	defer resp.Body.Close()
	c.lastNonce = resp.Header.Get("Replay-Nonce")
	if c.lastNonce == "" {
		return fmt.Errorf("server omitted Replay-Nonce")
	}
	return nil
}

func (c *Client) newAccount() error {
	jwk, err := JwkFromPublicKey(c.PublicKey)
	if err != nil {
		return err
	}
	env, err := Sign(
		ProtectedHeader{Alg: AlgEdDSA, Nonce: c.lastNonce, URL: c.directory.NewAccount, JWK: jwk},
		NewAccountPayload{TermsOfServiceAgreed: true},
		c.PrivateKey)
	if err != nil {
		return err
	}
	resp, err := c.post(c.directory.NewAccount, env)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := ensureSuccess(resp); err != nil {
		return err
	}
	c.accountUrl = resp.Header.Get("Location")
	if c.accountUrl == "" {
		return fmt.Errorf("server omitted account Location header")
	}
	c.captureNonce(resp)
	return nil
}

func (c *Client) newOrder(nid string) (*Order, error) {
	env, err := Sign(
		ProtectedHeader{Alg: AlgEdDSA, Nonce: c.lastNonce, URL: c.directory.NewOrder, Kid: c.accountUrl},
		NewOrderPayload{Identifiers: []Identifier{{Type: IdentifierTypeNID, Value: nid}}},
		c.PrivateKey)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(c.directory.NewOrder, env)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := ensureSuccess(resp); err != nil {
		return nil, err
	}
	c.captureNonce(resp)
	var order Order
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		return nil, fmt.Errorf("decode order: %w", err)
	}
	return &order, nil
}

func (c *Client) fetchAuthz(url string) (*Authorization, error) {
	// POST-as-GET (RFC 8555 §6.3).
	env, err := Sign(
		ProtectedHeader{Alg: AlgEdDSA, Nonce: c.lastNonce, URL: url, Kid: c.accountUrl},
		nil, c.PrivateKey)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(url, env)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := ensureSuccess(resp); err != nil {
		return nil, err
	}
	c.captureNonce(resp)
	var authz Authorization
	if err := json.NewDecoder(resp.Body).Decode(&authz); err != nil {
		return nil, fmt.Errorf("decode authz: %w", err)
	}
	return &authz, nil
}

func (c *Client) respondAgent01(authz *Authorization) error {
	var ch *Challenge
	for i := range authz.Challenges {
		if authz.Challenges[i].Type == ChallengeAgent01 {
			ch = &authz.Challenges[i]
			break
		}
	}
	if ch == nil {
		return fmt.Errorf("authz has no agent-01 challenge")
	}
	sig := ed25519.Sign(c.PrivateKey, []byte(ch.Token))
	env, err := Sign(
		ProtectedHeader{Alg: AlgEdDSA, Nonce: c.lastNonce, URL: ch.URL, Kid: c.accountUrl},
		ChallengeRespondPayload{AgentSignature: B64uEncode(sig)},
		c.PrivateKey)
	if err != nil {
		return err
	}
	resp, err := c.post(ch.URL, env)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := ensureSuccess(resp); err != nil {
		return err
	}
	c.captureNonce(resp)
	return nil
}

func (c *Client) finalizeOrder(order *Order, nid string) (*Order, error) {
	csrDer, err := c.buildCsr(nid)
	if err != nil {
		return nil, err
	}
	env, err := Sign(
		ProtectedHeader{Alg: AlgEdDSA, Nonce: c.lastNonce, URL: order.Finalize, Kid: c.accountUrl},
		FinalizePayload{CSR: B64uEncode(csrDer)},
		c.PrivateKey)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(order.Finalize, env)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := ensureSuccess(resp); err != nil {
		return nil, err
	}
	c.captureNonce(resp)
	var finalized Order
	if err := json.NewDecoder(resp.Body).Decode(&finalized); err != nil {
		return nil, fmt.Errorf("decode finalized order: %w", err)
	}
	return &finalized, nil
}

func (c *Client) downloadPem(certUrl string) (string, error) {
	env, err := Sign(
		ProtectedHeader{Alg: AlgEdDSA, Nonce: c.lastNonce, URL: certUrl, Kid: c.accountUrl},
		nil, c.PrivateKey)
	if err != nil {
		return "", err
	}
	resp, err := c.post(certUrl, env)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := ensureSuccess(resp); err != nil {
		return "", err
	}
	c.captureNonce(resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) buildCsr(nid string) ([]byte, error) {
	uri, err := url.Parse(nid)
	if err != nil {
		return nil, fmt.Errorf("parse nid as URI: %w", err)
	}
	tmpl := &cryptox509.CertificateRequest{
		Subject: pkix.Name{CommonName: nid},
		URIs:    []*url.URL{uri},
	}
	return cryptox509.CreateCertificateRequest(rand.Reader, tmpl, c.PrivateKey)
}

func (c *Client) post(url string, env *Envelope) (*http.Response, error) {
	body, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ContentTypeJoseJSON)
	return c.HTTPClient.Do(req)
}

func (c *Client) captureNonce(resp *http.Response) {
	if n := resp.Header.Get("Replay-Nonce"); n != "" {
		c.lastNonce = n
	}
}

func ensureSuccess(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("ACME %s HTTP %d", resp.Request.URL, resp.StatusCode)
}
