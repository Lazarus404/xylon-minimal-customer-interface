package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Config is the interface process configuration.
type Config struct {
	ListenAddr            string `json:"listen_addr"`
	StoreBaseURL          string `json:"store_base_url"`
	XylonBaseURL          string `json:"xylon_base_url"`
	ControlParticipantID  string `json:"control_participant_id"`
	ControlTokenTTLSec    int    `json:"control_token_ttl_sec"`
	AdminConfigPath       string `json:"admin_config_path"`
	AdminCustomerID       string `json:"admin_customer_id"`
	SeedOnStart           bool   `json:"seed_on_start"`
	DefaultGraceSec       int    `json:"default_grace_sec"`
	WebhookSecret         string `json:"webhook_secret"`
	AdminWebhookURL       string `json:"admin_webhook_url"`
}

// Load reads JSON config from path and applies defaults.
func Load(path string) (Config, error) {
	cfg := Config{
		ListenAddr:           "127.0.0.1:8093",
		StoreBaseURL:         "http://127.0.0.1:8091",
		XylonBaseURL:         "http://127.0.0.1:8080",
		ControlParticipantID: "platform-admin",
		ControlTokenTTLSec:   60,
		AdminCustomerID:      "acme",
		SeedOnStart:          true,
		DefaultGraceSec:      900,
	}
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8093"
	}
	if cfg.StoreBaseURL == "" {
		cfg.StoreBaseURL = "http://127.0.0.1:8091"
	}
	if cfg.XylonBaseURL == "" {
		cfg.XylonBaseURL = "http://127.0.0.1:8080"
	}
	if cfg.ControlParticipantID == "" {
		cfg.ControlParticipantID = "platform-admin"
	}
	if cfg.ControlTokenTTLSec < 1 {
		cfg.ControlTokenTTLSec = 60
	}
	if cfg.AdminCustomerID == "" {
		cfg.AdminCustomerID = "acme"
	}
	if cfg.DefaultGraceSec < 1 {
		cfg.DefaultGraceSec = 900
	}
	if err := requireLoopbackListen(cfg.ListenAddr); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// requireLoopbackListen rejects wildcard / non-loopback binds (ops API has no bearer).
func requireLoopbackListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen_addr: %w", err)
	}
	host = strings.TrimSpace(host)
	switch strings.ToLower(host) {
	case "", "0.0.0.0", "::", "[::]":
		return fmt.Errorf("listen_addr must be loopback (got %q); ops API is unauthenticated", addr)
	case "127.0.0.1", "localhost", "::1":
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("listen_addr must be loopback (got %q); ops API is unauthenticated", addr)
}
