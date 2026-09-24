package httpapi_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/admincfg"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/config"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/customers"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/httpapi"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/storeclient"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/webhooksign"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/webhookstore"
	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/xylonclient"
)

func TestProvisionGetRevokeList(t *testing.T) {
	storeRows := map[string]bool{}
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/tenants/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
			storeRows[id] = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"current_present":true}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/tenants/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
			delete(storeRows, id)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/tenants/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
			if storeRows[id] {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"current_present":true}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"previous_present":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer store.Close()

	adminPath := filepath.Join(t.TempDir(), "admin.json")
	srv := &httpapi.Server{
		Cfg: config.Config{
			DefaultGraceSec: 900,
			AdminConfigPath: adminPath,
			AdminCustomerID: "acme",
		},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
	}
	srv.Dir.Upsert(customers.Customer{ID: "acme", Status: customers.StatusSuspended})
	srv.Dir.Upsert(customers.Customer{ID: "broke-co", Status: customers.StatusSuspended})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/v1/customers/acme/provision", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var prov map[string]any
	if err := json.NewDecoder(res.Body).Decode(&prov); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("provision status %d body=%v", res.StatusCode, prov)
	}
	secret, _ := prov["hmac_secret"].(string)
	if secret == "" {
		t.Fatal("expected hmac_secret once")
	}

	raw, err := os.ReadFile(adminPath)
	if err != nil {
		t.Fatal(err)
	}
	var af admincfg.File
	if err := json.Unmarshal(raw, &af); err != nil {
		t.Fatal(err)
	}
	if af.HMACSecret != secret || af.CustomerID != "acme" {
		t.Fatalf("admin cfg: %+v", af)
	}

	res, err = http.Get(ts.URL + "/v1/customers/acme")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var got map[string]any
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got["status"] != customers.StatusActive || got["store_present"] != true {
		t.Fatalf("get: %v", got)
	}
	if _, has := got["hmac_secret"]; has {
		t.Fatal("get must not include hmac_secret")
	}

	res, err = http.Get(ts.URL + "/v1/customers")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(body), "hmac_secret") {
		t.Fatalf("list leaked secret: %s", body)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/revoke", nil)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got["status"] != customers.StatusSuspended || got["store_present"] != false {
		t.Fatalf("revoke: %v", got)
	}
}

func TestRotateExistingAndMissingStoreRow(t *testing.T) {
	storeRows := map[string]bool{}
	var lastMethod string
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
		switch r.Method {
		case http.MethodGet:
			if storeRows[id] {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case http.MethodPut:
			storeRows[id] = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case http.MethodPatch:
			if !storeRows[id] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"previous_present":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer store.Close()

	srv := &httpapi.Server{
		Cfg:   config.Config{DefaultGraceSec: 900, AdminCustomerID: "acme"},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
	}
	srv.Dir.Upsert(customers.Customer{ID: "acme", Status: customers.StatusActive})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Missing store row -> PUT (install)
	res, err := http.Post(ts.URL+"/v1/customers/acme/rotate", "application/json", strings.NewReader(`{"grace_sec":60}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	if res.StatusCode != http.StatusOK || body["hmac_secret"] == nil || body["hmac_secret"] == "" {
		t.Fatalf("rotate missing row: %d %v", res.StatusCode, body)
	}
	if lastMethod != http.MethodPut {
		t.Fatalf("expected PUT for missing row, got %s", lastMethod)
	}
	first := body["hmac_secret"].(string)

	// Existing row -> PATCH
	res, err = http.Post(ts.URL+"/v1/customers/acme/rotate", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_ = json.NewDecoder(res.Body).Decode(&body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("rotate existing: %d %v", res.StatusCode, body)
	}
	if lastMethod != http.MethodPatch {
		t.Fatalf("expected PATCH for existing row, got %s", lastMethod)
	}
	second, _ := body["hmac_secret"].(string)
	if second == "" || second == first {
		t.Fatalf("expected new secret, got %q (was %q)", second, first)
	}
}

func TestControlProxyGatesAndForward(t *testing.T) {
	var xylonHits int
	var lastXylonAuth, lastXylonPath string

	xylon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xylonHits++
		lastXylonAuth = r.Header.Get("Authorization")
		lastXylonPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"participants":{"p1":{"role":"host"}}}`))
	}))
	defer xylon.Close()

	storeSecrets := map[string]string{"acme": "tenant-secret"}
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
		switch r.Method {
		case http.MethodGet:
			sec, ok := storeSecrets[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.Query().Get("reveal") == "1" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{"tenant_id": id, "current": sec, "flags": map[string]any{}})
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"current_present":true}`))
		case http.MethodPut:
			var rec map[string]any
			_ = json.NewDecoder(r.Body).Decode(&rec)
			if cur, ok := rec["current"].(string); ok {
				storeSecrets[id] = cur
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer store.Close()

	srv := &httpapi.Server{
		Cfg: config.Config{
			DefaultGraceSec:      900,
			ControlParticipantID: "platform-admin",
			ControlTokenTTLSec:   60,
			XylonBaseURL:         xylon.URL,
		},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
		Xylon: xylonclient.New(xylon.URL, "platform-admin", 60),
	}
	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	srv.Dir.Upsert(customers.Customer{
		ID:       "broke-co",
		Status:   customers.StatusSuspended,
		Features: customers.DefaultControlFeatures(),
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Suspended -> 403, never hit Xylon
	res, err := http.Get(ts.URL + "/v1/customers/broke-co/sessions/room-1/presence")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "customer_suspended" {
		t.Fatalf("suspended: %d %v", res.StatusCode, body)
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after suspend: %d", xylonHits)
	}

	// Feature deny
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl:  true,
			customers.FeaturePresence: true,
			customers.FeatureHistory:  true,
			customers.FeatureInject:   true,
			customers.FeatureModerate: false,
		},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/sessions/room-1/moderate", strings.NewReader(`{"action":"kick","participant_id":"p2"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" {
		t.Fatalf("feature deny: %d %v", res.StatusCode, body)
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after feature deny: %d", xylonHits)
	}

	// Happy path presence
	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	res, err = http.Get(ts.URL + "/v1/customers/acme/sessions/room-1/presence")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("presence: %d %s", res.StatusCode, raw)
	}
	if xylonHits != 1 {
		t.Fatalf("xylon hits: %d", xylonHits)
	}
	if !strings.HasPrefix(lastXylonAuth, "Bearer ") {
		t.Fatalf("auth: %q", lastXylonAuth)
	}
	if lastXylonPath != "/v1/tenants/acme/sessions/room-1/presence" {
		t.Fatalf("path: %q", lastXylonPath)
	}
	if !strings.Contains(string(raw), `"p1"`) {
		t.Fatalf("passthrough body: %s", raw)
	}
}

func TestRecordProxyGatesAndForward(t *testing.T) {
	var xylonHits int
	var lastXylonAuth, lastXylonPath, lastXylonMethod, lastXylonBody string

	xylon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xylonHits++
		lastXylonAuth = r.Header.Get("Authorization")
		lastXylonPath = r.URL.Path
		lastXylonMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		lastXylonBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"recording_id":"rec-1","state":"recording"}`))
	}))
	defer xylon.Close()

	storeSecrets := map[string]string{"acme": "tenant-secret"}
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
		switch r.Method {
		case http.MethodGet:
			sec, ok := storeSecrets[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.Query().Get("reveal") == "1" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{"tenant_id": id, "current": sec, "flags": map[string]any{}})
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"current_present":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer store.Close()

	srv := &httpapi.Server{
		Cfg: config.Config{
			DefaultGraceSec:      900,
			ControlParticipantID: "platform-admin",
			ControlTokenTTLSec:   60,
			XylonBaseURL:         xylon.URL,
		},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
		Xylon: xylonclient.New(xylon.URL, "platform-admin", 60),
	}
	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	srv.Dir.Upsert(customers.Customer{
		ID:       "broke-co",
		Status:   customers.StatusSuspended,
		Features: customers.DefaultControlFeatures(),
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Suspended -> 403
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/broke-co/sessions/room-1/record/start", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "customer_suspended" {
		t.Fatalf("suspended: %d %v", res.StatusCode, body)
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after suspend: %d", xylonHits)
	}

	// Feature deny
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl:  true,
			customers.FeaturePresence: true,
			customers.FeatureHistory:  true,
			customers.FeatureInject:   true,
			customers.FeatureModerate: true,
			customers.FeatureRecord:   false,
		},
	})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/sessions/room-1/record/start", strings.NewReader(`{"mode":"audio"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" || body["feature"] != "record" {
		t.Fatalf("feature deny: %d %v", res.StatusCode, body)
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after feature deny: %d", xylonHits)
	}

	// Happy path start
	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	startBody := `{"mode":"audio"}`
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/sessions/room-1/record/start", strings.NewReader(startBody))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start: %d %s", res.StatusCode, raw)
	}
	if xylonHits != 1 || lastXylonMethod != http.MethodPost {
		t.Fatalf("xylon hits=%d method=%s", xylonHits, lastXylonMethod)
	}
	if lastXylonPath != "/v1/tenants/acme/sessions/room-1/record/start" {
		t.Fatalf("path: %q", lastXylonPath)
	}
	if lastXylonBody != startBody {
		t.Fatalf("body: %q", lastXylonBody)
	}
	if !strings.HasPrefix(lastXylonAuth, "Bearer ") {
		t.Fatalf("auth: %q", lastXylonAuth)
	}
	tok := strings.TrimPrefix(lastXylonAuth, "Bearer ")
	parts := strings.Split(tok, ".")
	payload, err := decodeRawURL(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `["record"]`) {
		t.Fatalf("caps: %q", payload)
	}
	if !strings.Contains(string(raw), `"rec-1"`) {
		t.Fatalf("passthrough: %s", raw)
	}

	// stop + get
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/sessions/room-1/record/stop", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if lastXylonPath != "/v1/tenants/acme/sessions/room-1/record/stop" {
		t.Fatalf("stop path: %q", lastXylonPath)
	}

	res, err = http.Get(ts.URL + "/v1/customers/acme/sessions/room-1/record")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if lastXylonMethod != http.MethodGet || lastXylonPath != "/v1/tenants/acme/sessions/room-1/record" {
		t.Fatalf("get method=%s path=%s", lastXylonMethod, lastXylonPath)
	}

	// control master deny (record true, control false)
	hitsBefore := xylonHits
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl: false,
			customers.FeatureRecord:  true,
		},
	})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/sessions/room-1/record/start", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" || body["feature"] != "control" {
		t.Fatalf("control master deny: %d %v", res.StatusCode, body)
	}
	if xylonHits != hitsBefore {
		t.Fatalf("xylon hit on control deny: %d -> %d", hitsBefore, xylonHits)
	}

	// feature deny on GET record
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl: true,
			customers.FeatureRecord:  false,
		},
	})
	res, err = http.Get(ts.URL + "/v1/customers/acme/sessions/room-1/record")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" || body["feature"] != "record" {
		t.Fatalf("get feature deny: %d %v", res.StatusCode, body)
	}
	if xylonHits != hitsBefore {
		t.Fatalf("xylon hit on get deny: %d -> %d", hitsBefore, xylonHits)
	}
}

func decodeRawURL(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func TestPatchCustomerFeatures(t *testing.T) {
	var putFlags map[string]any
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("reveal") == "1" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{"current": "sec", "flags": map[string]any{}})
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"current_present":true}`))
		case http.MethodPut:
			var rec map[string]any
			_ = json.NewDecoder(r.Body).Decode(&rec)
			putFlags, _ = rec["flags"].(map[string]any)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer store.Close()

	srv := &httpapi.Server{
		Cfg:   config.Config{DefaultGraceSec: 900},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
	}
	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/v1/customers/acme", strings.NewReader(`{"features":{"moderate":false}}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch: %d %v", res.StatusCode, body)
	}
	feats, _ := body["features"].(map[string]any)
	if feats["moderate"] != false {
		t.Fatalf("features: %v", feats)
	}
	if putFlags["moderate"] != false {
		t.Fatalf("store flags: %v", putFlags)
	}
}

func TestXylonWebhookIngest(t *testing.T) {
	srv := &httpapi.Server{
		Cfg:      config.Config{WebhookSecret: "lab-webhook-secret"},
		Dir:      customers.New(),
		Webhooks: webhookstore.New(8),
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	raw := []byte(`{"type":"recording.completed","recording_id":"r1","tenant_id":"acme"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/webhooks/xylon", strings.NewReader(string(raw)))
	req.Header.Set(webhooksign.Header, "sha256=deadbeef")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad sig status %d", res.StatusCode)
	}

	sig := webhooksign.Sign(raw, "lab-webhook-secret")
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/webhooks/xylon", strings.NewReader(string(raw)))
	req.Header.Set(webhooksign.Header, sig)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("good sig status %d", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/v1/webhooks/xylon/latest")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	if res.StatusCode != http.StatusOK || body["recording_id"] != "r1" {
		t.Fatalf("latest: %d %v", res.StatusCode, body)
	}
}

func TestSessionsListProxyGatesAndForward(t *testing.T) {
	var xylonHits int
	var lastXylonAuth, lastXylonPath string

	xylon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xylonHits++
		lastXylonAuth = r.Header.Get("Authorization")
		lastXylonPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sessions":[{"session_id":"room-1","participant_count":1}]}`))
	}))
	defer xylon.Close()

	storeSecrets := map[string]string{"acme": "tenant-secret"}
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
		if r.Method == http.MethodGet && r.URL.Query().Get("reveal") == "1" {
			sec, ok := storeSecrets[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"tenant_id": id, "current": sec, "flags": map[string]any{}})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"current_present":true}`))
	}))
	defer store.Close()

	srv := &httpapi.Server{
		Cfg: config.Config{
			ControlParticipantID: "platform-admin",
			ControlTokenTTLSec:   60,
			XylonBaseURL:         xylon.URL,
		},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
		Xylon: xylonclient.New(xylon.URL, "platform-admin", 60),
	}
	srv.Dir.Upsert(customers.Customer{
		ID:       "broke-co",
		Status:   customers.StatusSuspended,
		Features: customers.DefaultControlFeatures(),
	})
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl:  true,
			customers.FeatureSessions: false,
		},
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/v1/customers/broke-co/sessions")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "customer_suspended" {
		t.Fatalf("suspended: %d %v", res.StatusCode, body)
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after suspend: %d", xylonHits)
	}

	res, err = http.Get(ts.URL + "/v1/customers/acme/sessions")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" {
		t.Fatalf("feature deny: %d %v", res.StatusCode, body)
	}
	if body["feature"] != customers.FeatureSessions {
		t.Fatalf("denied feature: %v", body["feature"])
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after feature deny: %d", xylonHits)
	}

	// Master control off -> deny with feature=control
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl:  false,
			customers.FeatureSessions: true,
		},
	})
	res, err = http.Get(ts.URL + "/v1/customers/acme/sessions")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" {
		t.Fatalf("control deny: %d %v", res.StatusCode, body)
	}
	if body["feature"] != customers.FeatureControl {
		t.Fatalf("denied feature: %v", body["feature"])
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after control deny: %d", xylonHits)
	}

	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	res, err = http.Get(ts.URL + "/v1/customers/acme/sessions")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", res.StatusCode, raw)
	}
	if xylonHits != 1 {
		t.Fatalf("xylon hits: %d", xylonHits)
	}
	if lastXylonPath != "/v1/tenants/acme/sessions" {
		t.Fatalf("path: %q", lastXylonPath)
	}
	if !strings.HasPrefix(lastXylonAuth, "Bearer ") {
		t.Fatalf("auth: %q", lastXylonAuth)
	}
	if !strings.Contains(string(raw), `"room-1"`) {
		t.Fatalf("passthrough: %s", raw)
	}
}

func TestFleetProxyGatesAndForward(t *testing.T) {
	var xylonHits int
	var lastXylonPath, lastMethod, lastBody string

	xylon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xylonHits++
		lastXylonPath = r.URL.Path
		lastMethod = r.Method
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			lastBody = string(b)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"node":"a@host","accepting":false}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "/fleet/summary") {
			_, _ = w.Write([]byte(`{"summary":{"node_count":1,"participant_count":2,"regions":["local"]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"nodes":[{"node":"a@host","participant_count":2,"reachable":true}]}`))
	}))
	defer xylon.Close()

	storeSecrets := map[string]string{"acme": "tenant-secret"}
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/tenants/")
		if r.Method == http.MethodGet && r.URL.Query().Get("reveal") == "1" {
			sec, ok := storeSecrets[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"tenant_id": id, "current": sec, "flags": map[string]any{}})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"current_present":true}`))
	}))
	defer store.Close()

	srv := &httpapi.Server{
		Cfg: config.Config{
			ControlParticipantID: "platform-admin",
			ControlTokenTTLSec:   60,
			XylonBaseURL:         xylon.URL,
		},
		Dir:   customers.New(),
		Store: storeclient.New(store.URL),
		Xylon: xylonclient.New(xylon.URL, "platform-admin", 60),
	}
	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl: true,
			customers.FeatureFleet:   false,
		},
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/v1/customers/acme/fleet/nodes")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["error"] != "feature_denied" {
		t.Fatalf("feature deny: %d %v", res.StatusCode, body)
	}
	if body["feature"] != customers.FeatureFleet {
		t.Fatalf("denied feature: %v", body["feature"])
	}
	if xylonHits != 0 {
		t.Fatalf("xylon hits after feature deny: %d", xylonHits)
	}

	srv.Dir.Upsert(customers.Customer{
		ID:     "acme",
		Status: customers.StatusActive,
		Features: map[string]any{
			customers.FeatureControl: false,
			customers.FeatureFleet:   true,
		},
	})
	res, err = http.Get(ts.URL + "/v1/customers/acme/fleet/summary")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden || body["feature"] != customers.FeatureControl {
		t.Fatalf("control deny: %d %v", res.StatusCode, body)
	}

	srv.Dir.Upsert(customers.Customer{
		ID:       "acme",
		Status:   customers.StatusActive,
		Features: customers.DefaultControlFeatures(),
	})
	res, err = http.Get(ts.URL + "/v1/customers/acme/fleet/nodes")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("nodes: %d %s", res.StatusCode, raw)
	}
	if lastXylonPath != "/v1/tenants/acme/fleet/nodes" {
		t.Fatalf("nodes path: %q", lastXylonPath)
	}
	if !strings.Contains(string(raw), `"a@host"`) {
		t.Fatalf("passthrough: %s", raw)
	}

	res, err = http.Get(ts.URL + "/v1/customers/acme/fleet/summary")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("summary: %d %s", res.StatusCode, raw)
	}
	if lastXylonPath != "/v1/tenants/acme/fleet/summary" {
		t.Fatalf("summary path: %q", lastXylonPath)
	}
	if !strings.Contains(string(raw), `"node_count"`) {
		t.Fatalf("summary body: %s", raw)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/customers/acme/fleet/accepting", strings.NewReader(`{"node":"a@host","accepting":false}`))
	req.Header.Set("Content-Type", "application/json")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("accepting: %d %s", res.StatusCode, raw)
	}
	if lastMethod != http.MethodPost || lastXylonPath != "/v1/tenants/acme/fleet/accepting" {
		t.Fatalf("accepting forward method=%s path=%q", lastMethod, lastXylonPath)
	}
	if lastBody != `{"node":"a@host","accepting":false}` {
		t.Fatalf("accepting body: %q", lastBody)
	}
}

func TestXylonWebhookForwardsToAdmin(t *testing.T) {
	var forwarded []byte
	var forwardedSig string
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded, _ = io.ReadAll(r.Body)
		forwardedSig = r.Header.Get(webhooksign.Header)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer admin.Close()

	srv := &httpapi.Server{
		Cfg: config.Config{
			WebhookSecret:   "lab-webhook-secret",
			AdminWebhookURL: admin.URL,
		},
		Dir:      customers.New(),
		Webhooks: webhookstore.New(8),
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	raw := []byte(`{"type":"session.created","session_id":"room-1","tenant_id":"acme"}`)
	sig := webhooksign.Sign(raw, "lab-webhook-secret")
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/webhooks/xylon", strings.NewReader(string(raw)))
	req.Header.Set(webhooksign.Header, sig)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if string(forwarded) != string(raw) {
		t.Fatalf("forward body: %s", forwarded)
	}
	if forwardedSig != sig {
		t.Fatalf("forward sig: %q want %q", forwardedSig, sig)
	}
	if !webhooksign.Valid(forwarded, "lab-webhook-secret", forwardedSig) {
		t.Fatal("forwarded signature invalid under shared secret")
	}
	ev, ok := srv.Webhooks.Latest()
	if !ok || ev["type"] != "session.created" {
		t.Fatalf("ring: %v", ev)
	}
}
