package controltoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Params are the claims written into a Control HMAC token.
type Params struct {
	TenantID      string
	SessionID     string
	ParticipantID string
	Capabilities  []string
	TTLSec        int
	// Now overrides wall clock (unix seconds) for tests; zero = time.Now.
	Now int64
}

// Mint builds a Xylon.Control.Auth.HMAC-compatible bearer.
func Mint(secret string, p Params) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("empty secret")
	}
	if p.TenantID == "" || p.SessionID == "" || p.ParticipantID == "" {
		return "", fmt.Errorf("tenant, session, and participant required")
	}
	ttl := p.TTLSec
	if ttl < 1 {
		ttl = 60
	}
	now := p.Now
	if now == 0 {
		now = time.Now().Unix()
	}
	expiry := now + int64(ttl)

	caps := p.Capabilities
	if caps == nil {
		caps = []string{}
	}
	capsJSON, err := json.Marshal(caps)
	if err != nil {
		return "", fmt.Errorf("caps: %w", err)
	}

	payload := strconv.FormatInt(expiry, 10) + ":" + p.TenantID + ":" + p.SessionID + ":" +
		p.ParticipantID + ":" + string(capsJSON)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(sig), nil
}
