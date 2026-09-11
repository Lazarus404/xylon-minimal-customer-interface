package admincfg_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/admincfg"
)

func TestWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.json")
	if err := admincfg.Write(path, "acme", "secret-value"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f admincfg.File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.CustomerID != "acme" || f.HMACSecret != "secret-value" {
		t.Fatalf("file: %+v", f)
	}
}

func TestWriteEmptyPathNoop(t *testing.T) {
	if err := admincfg.Write("", "acme", "x"); err != nil {
		t.Fatal(err)
	}
}
