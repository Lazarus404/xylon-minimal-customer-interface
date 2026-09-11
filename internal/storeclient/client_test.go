package storeclient_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/storeclient"
)

func TestPutPatchDeletePresent(t *testing.T) {
	var lastMethod, lastPath string
	var lastBody map[string]any
	present := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod, lastPath = r.Method, r.URL.Path
		body, _ := io.ReadAll(r.Body)
		lastBody = nil
		if len(body) > 0 {
			_ = json.Unmarshal(body, &lastBody)
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/tenants/acme":
			present = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tenant_id":"acme","current_present":true}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/tenants/acme":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tenant_id":"acme","previous_present":true}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/tenants/acme":
			present = false
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tenants/acme":
			if present {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"tenant_id":"acme","current_present":true}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"not_found"}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := storeclient.New(srv.URL)

	ok, err := c.Present("acme")
	if err != nil || ok {
		t.Fatalf("present before put: ok=%v err=%v", ok, err)
	}

	if err := c.Put("acme", storeclient.Record{Current: "sec"}); err != nil {
		t.Fatal(err)
	}
	if lastMethod != http.MethodPut || lastBody["current"] != "sec" {
		t.Fatalf("put: method=%s body=%v", lastMethod, lastBody)
	}

	ok, err = c.Present("acme")
	if err != nil || !ok {
		t.Fatalf("present after put: ok=%v err=%v", ok, err)
	}

	if err := c.PatchRotate("acme", "sec2", 900); err != nil {
		t.Fatal(err)
	}
	if lastMethod != http.MethodPatch || lastPath != "/v1/tenants/acme" {
		t.Fatalf("patch: %s %s", lastMethod, lastPath)
	}
	if lastBody["grace_sec"].(float64) != 900 {
		t.Fatalf("grace: %v", lastBody["grace_sec"])
	}

	if err := c.Delete("acme"); err != nil {
		t.Fatal(err)
	}
	ok, err = c.Present("acme")
	if err != nil || ok {
		t.Fatalf("present after delete: ok=%v err=%v", ok, err)
	}
}

func TestGetReveal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tenants/acme" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("reveal") != "1" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tenant_id":"acme","current_present":true}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tenant_id":"acme","current":"secret-value","flags":{"control":true}}`))
	}))
	defer srv.Close()

	c := storeclient.New(srv.URL)
	rec, err := c.GetReveal("acme")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Current != "secret-value" {
		t.Fatalf("current: %q", rec.Current)
	}
	if rec.Flags["control"] != true {
		t.Fatalf("flags: %v", rec.Flags)
	}
}
