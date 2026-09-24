package xylonclient_test

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Lazarus404/xylon-minimal-customer-interface/internal/xylonclient"
)

func TestPresenceForwardsBearer(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"participants":{}}`))
	}))
	defer srv.Close()

	c := xylonclient.New(srv.URL, "platform-admin", 60)
	res, err := c.Presence("lab-secret", "acme", "room-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") || len(gotAuth) < 20 {
		t.Fatalf("auth: %q", gotAuth)
	}
	if gotPath != "/v1/tenants/acme/sessions/room-1/presence" {
		t.Fatalf("path: %q", gotPath)
	}
	if string(res.Body) != `{"participants":{}}` {
		t.Fatalf("body: %s", res.Body)
	}
}

func TestModeratePostsBody(t *testing.T) {
	var gotBody, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := xylonclient.New(srv.URL, "platform-admin", 30)
	body := []byte(`{"action":"kick","participant_id":"p2"}`)
	res, err := c.Moderate("sec", "acme", "room-1", body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || gotPath != "/v1/tenants/acme/sessions/room-1/moderate" {
		t.Fatalf("status=%d path=%s", res.StatusCode, gotPath)
	}
	if gotBody != string(body) {
		t.Fatalf("body: %q", gotBody)
	}
}

func TestRecordStartStopGet(t *testing.T) {
	var gotPath, gotAuth, gotBody, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"recording_id":"rec-1"}`))
	}))
	defer srv.Close()

	c := xylonclient.New(srv.URL, "platform-admin", 60)
	body := []byte(`{"mode":"audio"}`)
	res, err := c.RecordStart("sec", "acme", "room-1", body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || gotMethod != http.MethodPost {
		t.Fatalf("start status=%d method=%s", res.StatusCode, gotMethod)
	}
	if gotPath != "/v1/tenants/acme/sessions/room-1/record/start" {
		t.Fatalf("start path: %q", gotPath)
	}
	if gotBody != string(body) {
		t.Fatalf("start body: %q", gotBody)
	}
	payload := bearerPayload(t, gotAuth)
	if !strings.Contains(payload, `["record"]`) {
		t.Fatalf("caps payload: %q", payload)
	}

	res, err = c.RecordStop("sec", "acme", "room-1", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/tenants/acme/sessions/room-1/record/stop" {
		t.Fatalf("stop path: %q", gotPath)
	}
	payload = bearerPayload(t, gotAuth)
	if !strings.Contains(payload, `["record"]`) {
		t.Fatalf("stop caps: %q", payload)
	}

	res, err = c.RecordGet("sec", "acme", "room-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v1/tenants/acme/sessions/room-1/record" {
		t.Fatalf("get method=%s path=%s", gotMethod, gotPath)
	}
	if string(res.Body) != `{"recording_id":"rec-1"}` {
		t.Fatalf("get body: %s", res.Body)
	}
}

func bearerPayload(t *testing.T, auth string) string {
	t.Helper()
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Fatalf("auth: %q", auth)
	}
	tok := strings.TrimPrefix(auth, "Bearer ")
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		t.Fatalf("token parts: %q", tok)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestHistoryQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[]}`))
	}))
	defer srv.Close()

	c := xylonclient.New(srv.URL, "admin", 60)
	_, err := c.History("sec", "t", "s", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "label=chat" {
		t.Fatalf("query: %q", gotQuery)
	}
}

func TestSessionsList(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	}))
	defer srv.Close()

	c := xylonclient.New(srv.URL, "platform-admin", 60)
	res, err := c.SessionsList("sec", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if gotPath != "/v1/tenants/acme/sessions" {
		t.Fatalf("path: %q", gotPath)
	}
	payload := bearerPayload(t, gotAuth)
	if !strings.Contains(payload, `["sessions"]`) {
		t.Fatalf("caps payload: %q", payload)
	}
	if string(res.Body) != `{"sessions":[]}` {
		t.Fatalf("body: %s", res.Body)
	}
}

func TestFleetSummaryAndNodes(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "/fleet/summary") {
			_, _ = w.Write([]byte(`{"summary":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer srv.Close()

	c := xylonclient.New(srv.URL, "platform-admin", 60)
	res, err := c.FleetNodes("sec", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/tenants/acme/fleet/nodes" {
		t.Fatalf("nodes path: %q", gotPath)
	}
	payload := bearerPayload(t, gotAuth)
	if !strings.Contains(payload, `["fleet"]`) {
		t.Fatalf("caps: %q", payload)
	}
	if string(res.Body) != `{"nodes":[]}` {
		t.Fatalf("body: %s", res.Body)
	}

	res, err = c.FleetSummary("sec", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/tenants/acme/fleet/summary" {
		t.Fatalf("summary path: %q", gotPath)
	}
	if string(res.Body) != `{"summary":{}}` {
		t.Fatalf("summary body: %s", res.Body)
	}

	gotMethod := ""
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"node":"a@host","accepting":false}`))
		_ = b
	}))
	defer srv2.Close()
	c2 := xylonclient.New(srv2.URL, "platform-admin", 60)
	res, err = c2.FleetAccepting("sec", "acme", []byte(`{"node":"a@host","accepting":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/tenants/acme/fleet/accepting" {
		t.Fatalf("accepting method=%s path=%q", gotMethod, gotPath)
	}
	payload = bearerPayload(t, gotAuth)
	if !strings.Contains(payload, `["fleet"]`) {
		t.Fatalf("accepting caps: %q", payload)
	}
	if !strings.Contains(string(res.Body), `"accepting":false`) {
		t.Fatalf("accepting body: %s", res.Body)
	}
}
