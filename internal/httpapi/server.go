package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/admincfg"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/config"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/customers"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/secretgen"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/storeclient"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/webhooksign"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/webhookstore"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/xylonclient"
)

// Server is the ops HTTP API.
type Server struct {
	Cfg      config.Config
	Dir      *customers.Directory
	Store    *storeclient.Client
	Xylon    *xylonclient.Client
	Webhooks *webhookstore.Store
}

// Handler returns the root mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/customers", s.handleList)
	mux.HandleFunc("GET /v1/customers/{id}", s.handleGet)
	mux.HandleFunc("PATCH /v1/customers/{id}", s.handlePatch)
	mux.HandleFunc("POST /v1/customers/{id}/provision", s.handleProvision)
	mux.HandleFunc("POST /v1/customers/{id}/rotate", s.handleRotate)
	mux.HandleFunc("POST /v1/customers/{id}/revoke", s.handleRevoke)
	mux.HandleFunc("GET /v1/customers/{id}/sessions/{sid}/presence", s.handlePresence)
	mux.HandleFunc("GET /v1/customers/{id}/sessions/{sid}/history", s.handleHistory)
	mux.HandleFunc("POST /v1/customers/{id}/sessions/{sid}/data", s.handleData)
	mux.HandleFunc("POST /v1/customers/{id}/sessions/{sid}/moderate", s.handleModerate)
	mux.HandleFunc("POST /v1/customers/{id}/sessions/{sid}/record/start", s.handleRecordStart)
	mux.HandleFunc("POST /v1/customers/{id}/sessions/{sid}/record/stop", s.handleRecordStop)
	mux.HandleFunc("GET /v1/customers/{id}/sessions/{sid}/record", s.handleRecordGet)
	mux.HandleFunc("GET /v1/customers/{id}/sessions", s.handleSessionsList)
	mux.HandleFunc("GET /v1/customers/{id}/fleet/summary", s.handleFleetSummary)
	mux.HandleFunc("GET /v1/customers/{id}/fleet/nodes", s.handleFleetNodes)
	mux.HandleFunc("POST /v1/customers/{id}/fleet/accepting", s.handleFleetAccepting)
	mux.HandleFunc("POST /v1/webhooks/xylon", s.handleXylonWebhook)
	mux.HandleFunc("GET /v1/webhooks/xylon/latest", s.handleXylonWebhookLatest)
	return mux
}

// Seed ensures demo customers and provisions acme when missing from the store.
func (s *Server) Seed() error {
	if _, ok := s.Dir.Get("acme"); !ok {
		s.Dir.Upsert(customers.Customer{
			ID:       "acme",
			Status:   customers.StatusActive,
			Features: customers.DefaultControlFeatures(),
		})
	}
	if _, ok := s.Dir.Get("broke-co"); !ok {
		s.Dir.Upsert(customers.Customer{
			ID:       "broke-co",
			Status:   customers.StatusSuspended,
			Features: map[string]any{},
			Notes:    "demo suspended; no store row",
		})
	}
	present, err := s.Store.Present("acme")
	if err != nil {
		return err
	}
	if !present {
		if _, err := s.provision("acme"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"ok": true}
	if err := s.Store.Healthz(); err != nil {
		out["store_ok"] = false
		out["store_error"] = err.Error()
	} else {
		out["store_ok"] = true
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	list := s.Dir.List()
	items := make([]map[string]any, 0, len(list))
	for _, c := range list {
		items = append(items, publicCustomer(c, nil))
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": items})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := s.Dir.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	present, err := s.Store.Present(id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "store_unreachable", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, publicCustomer(c, &present))
}

func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Dir.Get(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	var body struct {
		Status   *string        `json:"status"`
		Features map[string]any `json:"features"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
			return
		}
	}
	if body.Status != nil {
		switch *body.Status {
		case customers.StatusActive, customers.StatusSuspended:
		default:
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_status"})
			return
		}
	}
	c, ok := s.Dir.Patch(id, body.Status, body.Features)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}

	present, err := s.Store.Present(id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "store_unreachable", "detail": err.Error()})
		return
	}
	if present && body.Features != nil {
		if err := s.syncStoreFlags(id, c.Features); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, publicCustomer(c, &present))
}

func (s *Server) syncStoreFlags(id string, flags map[string]any) error {
	rec, err := s.Store.GetReveal(id)
	if err != nil {
		return err
	}
	rec.Flags = flags
	return s.Store.Put(id, rec)
}

func (s *Server) handleProvision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	secret, err := s.provision(id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	c, _ := s.Dir.Get(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"customer_id": c.ID,
		"status":      c.Status,
		"hmac_secret": secret,
	})
}

func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := s.Dir.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	grace := s.Cfg.DefaultGraceSec
	var body struct {
		GraceSec *int `json:"grace_sec"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.GraceSec != nil && *body.GraceSec > 0 {
			grace = *body.GraceSec
		}
	}
	secret, err := secretgen.Generate()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	present, err := s.Store.Present(id)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "store_unreachable", "detail": err.Error()})
		return
	}
	if present {
		if err := s.Store.PatchRotate(id, secret, grace); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
	} else {
		// No store row yet: install as current (same outcome as provision with a new secret).
		if c.Features == nil {
			c.Features = map[string]any{}
		}
		if err := s.Store.Put(id, storeclient.Record{Current: secret, Flags: c.Features}); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
	}

	s.Dir.SetStatus(id, customers.StatusActive)
	if err := s.maybeWriteAdmin(id, secret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"customer_id": id,
		"status":      customers.StatusActive,
		"hmac_secret": secret,
	})
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.Dir.Get(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if err := s.Store.Delete(id); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	s.Dir.SetStatus(id, customers.StatusSuspended)
	c, _ := s.Dir.Get(id)
	writeJSON(w, http.StatusOK, publicCustomer(c, boolPtr(false)))
}

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeaturePresence, func(secret, id, sid string, _ []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.Presence(secret, id, sid)
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeatureHistory, func(secret, id, sid string, _ []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.History(secret, id, sid, r.URL.Query().Get("label"))
	})
}

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeatureInject, func(secret, id, sid string, body []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.Inject(secret, id, sid, body)
	})
}

func (s *Server) handleModerate(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeatureModerate, func(secret, id, sid string, body []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.Moderate(secret, id, sid, body)
	})
}

func (s *Server) handleRecordStart(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeatureRecord, func(secret, id, sid string, body []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.RecordStart(secret, id, sid, body)
	})
}

func (s *Server) handleRecordStop(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeatureRecord, func(secret, id, sid string, body []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.RecordStop(secret, id, sid, body)
	})
}

func (s *Server) handleRecordGet(w http.ResponseWriter, r *http.Request) {
	s.proxyControl(w, r, customers.FeatureRecord, func(secret, id, sid string, _ []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.RecordGet(secret, id, sid)
	})
}

func (s *Server) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	s.proxyControlTenant(w, r, customers.FeatureSessions, func(secret, id string, _ []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.SessionsList(secret, id)
	})
}

func (s *Server) handleFleetSummary(w http.ResponseWriter, r *http.Request) {
	s.proxyControlTenant(w, r, customers.FeatureFleet, func(secret, id string, _ []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.FleetSummary(secret, id)
	})
}

func (s *Server) handleFleetNodes(w http.ResponseWriter, r *http.Request) {
	s.proxyControlTenant(w, r, customers.FeatureFleet, func(secret, id string, _ []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.FleetNodes(secret, id)
	})
}

func (s *Server) handleFleetAccepting(w http.ResponseWriter, r *http.Request) {
	s.proxyControlTenant(w, r, customers.FeatureFleet, func(secret, id string, body []byte) (xylonclient.ProxyResult, error) {
		return s.Xylon.FleetAccepting(secret, id, body)
	})
}

func (s *Server) handleXylonWebhook(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.WebhookSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "webhook_unconfigured"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_body"})
		return
	}
	sig := r.Header.Get(webhooksign.Header)
	if !webhooksign.Valid(body, s.Cfg.WebhookSecret, sig) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "bad_signature"})
		return
	}
	var ev map[string]any
	if err := json.Unmarshal(body, &ev); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	if s.Webhooks == nil {
		s.Webhooks = webhookstore.New(32)
	}
	s.Webhooks.Add(ev)
	if s.Cfg.AdminWebhookURL != "" {
		s.forwardAdminWebhook(body, sig)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) forwardAdminWebhook(body []byte, sig string) {
	req, err := http.NewRequest(http.MethodPost, s.Cfg.AdminWebhookURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("interface admin webhook forward build: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhooksign.Header, sig)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("interface admin webhook forward: %v", err)
		return
	}
	_ = res.Body.Close()
}

func (s *Server) handleXylonWebhookLatest(w http.ResponseWriter, r *http.Request) {
	if s.Webhooks == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "empty"})
		return
	}
	ev, ok := s.Webhooks.Latest()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "empty"})
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

type controlCall func(secret, customerID, sessionID string, body []byte) (xylonclient.ProxyResult, error)

type tenantControlCall func(secret, customerID string, body []byte) (xylonclient.ProxyResult, error)

func (s *Server) proxyControlTenant(w http.ResponseWriter, r *http.Request, feature string, call tenantControlCall) {
	if s.Xylon == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "xylon_unconfigured"})
		return
	}
	id := r.PathValue("id")
	c, ok := s.Dir.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if c.Status != customers.StatusActive {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "customer_suspended"})
		return
	}
	if !customers.ControlAllowed(c.Features, feature) {
		denied := feature
		if !customers.FeatureEnabled(c.Features, customers.FeatureControl) {
			denied = customers.FeatureControl
		}
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "feature_denied", "feature": denied})
		return
	}

	rec, err := s.Store.GetReveal(id)
	if err != nil {
		if strings.Contains(err.Error(), "not_found") {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "store_secret_missing"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "store_unreachable", "detail": err.Error()})
		return
	}

	var body []byte
	if r.Method == http.MethodPost {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_body"})
			return
		}
	}

	res, err := call(rec.Current, id, body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "xylon_unreachable", "detail": err.Error()})
		return
	}
	if res.ContentType != "" {
		w.Header().Set("Content-Type", res.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}

func (s *Server) proxyControl(w http.ResponseWriter, r *http.Request, feature string, call controlCall) {
	if s.Xylon == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "xylon_unconfigured"})
		return
	}
	id := r.PathValue("id")
	sid := r.PathValue("sid")
	c, ok := s.Dir.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if c.Status != customers.StatusActive {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "customer_suspended"})
		return
	}
	if !customers.ControlAllowed(c.Features, feature) {
		denied := feature
		if !customers.FeatureEnabled(c.Features, customers.FeatureControl) {
			denied = customers.FeatureControl
		}
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "feature_denied", "feature": denied})
		return
	}

	rec, err := s.Store.GetReveal(id)
	if err != nil {
		if strings.Contains(err.Error(), "not_found") {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "store_secret_missing"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "store_unreachable", "detail": err.Error()})
		return
	}

	var body []byte
	if r.Method == http.MethodPost {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read_body"})
			return
		}
	}

	res, err := call(rec.Current, id, sid, body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "xylon_unreachable", "detail": err.Error()})
		return
	}
	if res.ContentType != "" {
		w.Header().Set("Content-Type", res.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(res.Body)
}

func (s *Server) provision(id string) (string, error) {
	c, ok := s.Dir.Get(id)
	if !ok {
		c = customers.Customer{ID: id, Features: map[string]any{}}
	}
	c.Status = customers.StatusActive
	if c.Features == nil {
		c.Features = map[string]any{}
	}
	s.Dir.Upsert(c)

	secret, err := secretgen.Generate()
	if err != nil {
		return "", err
	}
	rec := storeclient.Record{
		Current:  secret,
		Previous: nil,
		Flags:    c.Features,
	}
	if err := s.Store.Put(id, rec); err != nil {
		return "", err
	}
	if err := s.maybeWriteAdmin(id, secret); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Server) maybeWriteAdmin(id, secret string) error {
	if s.Cfg.AdminConfigPath == "" {
		return nil
	}
	if id != s.Cfg.AdminCustomerID {
		return nil
	}
	return admincfg.Write(s.Cfg.AdminConfigPath, id, secret)
}

func publicCustomer(c customers.Customer, storePresent *bool) map[string]any {
	out := map[string]any{
		"customer_id": c.ID,
		"status":      c.Status,
		"features":    c.Features,
	}
	if c.Notes != "" {
		out["notes"] = c.Notes
	}
	if storePresent != nil {
		out["store_present"] = *storePresent
	}
	return out
}

func boolPtr(v bool) *bool { return &v }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
