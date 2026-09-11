package controltoken_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/controltoken"
)

func TestMintShapeAndMAC(t *testing.T) {
	tok, err := controltoken.Mint("test-secret", controltoken.Params{
		TenantID:      "acme",
		SessionID:     "room-1",
		ParticipantID: "platform-admin",
		Capabilities:  []string{"moderate"},
		TTLSec:        60,
		Now:           1_700_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatalf("parts: %q", tok)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	wantPayload := `1700000060:acme:room-1:platform-admin:["moderate"]`
	if string(payload) != wantPayload {
		t.Fatalf("payload %q want %q", payload, wantPayload)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), sig) {
		t.Fatal("mac mismatch")
	}
}

func TestMintEmptyCaps(t *testing.T) {
	tok, err := controltoken.Mint("s", controltoken.Params{
		TenantID: "t", SessionID: "s", ParticipantID: "p",
		TTLSec: 1, Now: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[0])
	if !strings.HasSuffix(string(payload), ":[]") {
		t.Fatalf("payload: %s", payload)
	}
}
