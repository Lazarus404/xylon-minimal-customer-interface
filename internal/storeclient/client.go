package storeclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Record matches xylon-simple-secret-store PUT body fields.
type Record struct {
	Current            string         `json:"current"`
	Previous           *string        `json:"previous"`
	PreviousExpiresAt  *int64         `json:"previous_expires_at"`
	Disabled           bool           `json:"disabled?"`
	Flags              map[string]any `json:"flags"`
}

// Client talks to xylon-simple-secret-store over HTTP (no auth).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New builds a client with a 2s timeout.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// Put upserts a tenant secret record.
func (c *Client) Put(tenantID string, rec Record) error {
	if rec.Flags == nil {
		rec.Flags = map[string]any{}
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, c.BaseURL+"/v1/tenants/"+tenantID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("store put: %s: %s", res.Status, bytes.TrimSpace(b))
	}
	return nil
}

// PatchRotate rotates current with grace_sec.
func (c *Client) PatchRotate(tenantID, current string, graceSec int) error {
	body, err := json.Marshal(map[string]any{
		"current":   current,
		"grace_sec": graceSec,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPatch, c.BaseURL+"/v1/tenants/"+tenantID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("store patch: %s: %s", res.Status, bytes.TrimSpace(b))
	}
	return nil
}

// Delete removes a tenant row.
func (c *Client) Delete(tenantID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/v1/tenants/"+tenantID, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode != http.StatusNoContent && (res.StatusCode < 200 || res.StatusCode >= 300) {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("store delete: %s: %s", res.Status, bytes.TrimSpace(b))
	}
	return nil
}

// Present reports whether a redacted GET finds the tenant.
func (c *Client) Present(tenantID string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/tenants/"+tenantID, nil)
	if err != nil {
		return false, err
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		b, _ := io.ReadAll(res.Body)
		return false, fmt.Errorf("store get: %s: %s", res.Status, bytes.TrimSpace(b))
	}
}

// GetReveal fetches the tenant record with raw secrets (?reveal=1).
func (c *Client) GetReveal(tenantID string) (Record, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/tenants/"+tenantID+"?reveal=1", nil)
	if err != nil {
		return Record{}, err
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return Record{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	switch res.StatusCode {
	case http.StatusOK:
		var rec Record
		if err := json.Unmarshal(body, &rec); err != nil {
			return Record{}, fmt.Errorf("store reveal decode: %w", err)
		}
		if rec.Current == "" {
			return Record{}, fmt.Errorf("store reveal: empty current")
		}
		if rec.Flags == nil {
			rec.Flags = map[string]any{}
		}
		return rec, nil
	case http.StatusNotFound:
		return Record{}, fmt.Errorf("store reveal: not_found")
	case http.StatusForbidden:
		return Record{}, fmt.Errorf("store reveal: reveal_disabled")
	default:
		return Record{}, fmt.Errorf("store reveal: %s: %s", res.Status, bytes.TrimSpace(body))
	}
}

// Healthz probes the store liveness endpoint.
func (c *Client) Healthz() error {
	res, err := c.HTTPClient.Get(c.BaseURL + "/healthz")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("store healthz: %s", res.Status)
	}
	return nil
}
