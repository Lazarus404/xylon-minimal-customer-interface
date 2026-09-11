package secretgen_test

import (
	"encoding/base64"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/secretgen"
)

func TestGenerate(t *testing.T) {
	s, err := secretgen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Fatal("empty secret")
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 32 {
		t.Fatalf("got %d bytes, want >= 32", len(raw))
	}
}
