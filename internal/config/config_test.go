package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:8093" {
		t.Fatalf("listen: %q", cfg.ListenAddr)
	}
	if cfg.StoreBaseURL != "http://127.0.0.1:8091" {
		t.Fatalf("store: %q", cfg.StoreBaseURL)
	}
	if cfg.XylonBaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("xylon: %q", cfg.XylonBaseURL)
	}
	if cfg.ControlParticipantID != "platform-admin" {
		t.Fatalf("participant: %q", cfg.ControlParticipantID)
	}
	if cfg.ControlTokenTTLSec != 60 {
		t.Fatalf("ttl: %d", cfg.ControlTokenTTLSec)
	}
	if !cfg.SeedOnStart {
		t.Fatal("expected seed_on_start true")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{"listen_addr":"127.0.0.1:9000","seed_on_start":false,"default_grace_sec":60}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Fatalf("listen: %q", cfg.ListenAddr)
	}
	if cfg.SeedOnStart {
		t.Fatal("expected seed_on_start false")
	}
	if cfg.DefaultGraceSec != 60 {
		t.Fatalf("grace: %d", cfg.DefaultGraceSec)
	}
}

func TestLoadRejectsNonLoopback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{"listen_addr":"0.0.0.0:8093"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected error for 0.0.0.0")
	}
}
