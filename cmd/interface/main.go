package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/config"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/customers"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/httpapi"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/storeclient"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/webhookstore"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/xylonclient"
)

func main() {
	configPath := flag.String("config", "", "path to JSON config (optional)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	srv := &httpapi.Server{
		Cfg:      cfg,
		Dir:      customers.New(),
		Store:    storeclient.New(cfg.StoreBaseURL),
		Xylon:    xylonclient.New(cfg.XylonBaseURL, cfg.ControlParticipantID, cfg.ControlTokenTTLSec),
		Webhooks: webhookstore.New(32),
	}

	if cfg.SeedOnStart {
		if err := srv.Seed(); err != nil {
			log.Printf("seed warning: %v (is the secret store up on %s?)", err, cfg.StoreBaseURL)
		}
	}

	log.Printf("listening on http://%s (store %s, xylon %s)", cfg.ListenAddr, cfg.StoreBaseURL, cfg.XylonBaseURL)
	if cfg.WebhookSecret != "" {
		log.Printf("webhook ingest enabled (POST /v1/webhooks/xylon)")
	}
	if cfg.AdminWebhookURL != "" {
		log.Printf("webhook fan-out to %s", cfg.AdminWebhookURL)
	}
	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
