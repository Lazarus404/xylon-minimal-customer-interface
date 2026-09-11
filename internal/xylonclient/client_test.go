package xylonclient_test

import (
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
