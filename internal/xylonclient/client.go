package xylonclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/controltoken"
)

// Client forwards Control REST calls to Xylon with a minted HMAC bearer.
type Client struct {
	BaseURL       string
	ParticipantID string
	TokenTTLSec   int
	HTTPClient    *http.Client
}

// New builds a client with a 5s timeout.
func New(baseURL, participantID string, tokenTTLSec int) *Client {
	if tokenTTLSec < 1 {
		tokenTTLSec = 60
	}
	if participantID == "" {
		participantID = "platform-admin"
	}
	return &Client{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		ParticipantID: participantID,
		TokenTTLSec:   tokenTTLSec,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// ProxyResult is the raw Xylon HTTP response to pass through.
type ProxyResult struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

// Presence GETs Control presence for tenant/session.
func (c *Client) Presence(secret, tenantID, sessionID string) (ProxyResult, error) {
	return c.do(http.MethodGet, secret, tenantID, sessionID, "presence", nil, nil, nil)
}

// History GETs Control history; label may be empty (Xylon default).
func (c *Client) History(secret, tenantID, sessionID, label string) (ProxyResult, error) {
	q := url.Values{}
	if label != "" {
		q.Set("label", label)
	}
	return c.do(http.MethodGet, secret, tenantID, sessionID, "history", q, nil, nil)
}

// Inject POSTs Control data body (JSON bytes).
func (c *Client) Inject(secret, tenantID, sessionID string, body []byte) (ProxyResult, error) {
	return c.do(http.MethodPost, secret, tenantID, sessionID, "data", nil, body, []string{"inject"})
}

// Moderate POSTs Control moderate body (JSON bytes).
func (c *Client) Moderate(secret, tenantID, sessionID string, body []byte) (ProxyResult, error) {
	return c.do(http.MethodPost, secret, tenantID, sessionID, "moderate", nil, body, []string{"moderate"})
}

func (c *Client) do(method, secret, tenantID, sessionID, action string, query url.Values, body []byte, caps []string) (ProxyResult, error) {
	if c.BaseURL == "" {
		return ProxyResult{}, fmt.Errorf("xylon_base_url empty")
	}
	token, err := controltoken.Mint(secret, controltoken.Params{
		TenantID:      tenantID,
		SessionID:     sessionID,
		ParticipantID: c.ParticipantID,
		Capabilities:  caps,
		TTLSec:        c.TokenTTLSec,
	})
	if err != nil {
		return ProxyResult{}, err
	}

	path := fmt.Sprintf("/v1/tenants/%s/sessions/%s/%s", url.PathEscape(tenantID), url.PathEscape(sessionID), action)
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		return ProxyResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return ProxyResult{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return ProxyResult{}, err
	}
	return ProxyResult{
		StatusCode:  res.StatusCode,
		ContentType: res.Header.Get("Content-Type"),
		Body:        raw,
	}, nil
}
