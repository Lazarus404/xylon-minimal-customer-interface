package webhooksign_test

import (
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/webhooksign"
)

func TestSignRoundTrip(t *testing.T) {
	body := []byte(`{"type":"recording.completed","recording_id":"r1"}`)
	sig := webhooksign.Sign(body, "lab-webhook-secret")
	if !webhooksign.Valid(body, "lab-webhook-secret", sig) {
		t.Fatal("expected valid")
	}
	if webhooksign.Valid(body, "wrong", sig) {
		t.Fatal("expected invalid")
	}
}
