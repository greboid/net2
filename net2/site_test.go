package net2

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/greboid/net2/config"
	"github.com/rs/zerolog"
)

// TestDoRequestReplaysBodyAfter401 verifies that a request retried after an
// authentication failure carries the same body as the original attempt.
func TestDoRequestReplaysBodyAfter401(t *testing.T) {
	var attempts atomic.Int32
	var bodies []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/authorization/tokens", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"testtoken","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/api/v1/commands/door/open", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	logger := zerolog.New(io.Discard)
	site := &Site{
		BaseURL:    srv.URL,
		httpClient: srv.Client(),
		logger:     &logger,
		clientID:   "testclient",
		Doors:      map[uint64]*Door{42: {ID: 42}},
		config:     &config.SiteConfig{Username: "user", Password: "pass"},
	}
	if err := site.OpenDoor(42); err != nil {
		t.Fatalf("expected door open to succeed after retry, got %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 request attempts, got %d", len(bodies))
	}
	if bodies[0] == "" {
		t.Fatal("first attempt had an empty body")
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("body was not replayed on retry: first %q, second %q", bodies[0], bodies[1])
	}
}

// TestSiteManagerRejectsDuplicateSiteIDs ensures two sites sharing an id
// fail fast instead of silently shadowing each other.
func TestSiteManagerRejectsDuplicateSiteIDs(t *testing.T) {
	logger := zerolog.New(io.Discard)
	manager := &SiteManager{Logger: &logger}
	err := manager.Start([]*Site{
		{SiteID: 1, Name: "One"},
		{SiteID: 1, Name: "OneAgain"},
	})
	if err == nil {
		t.Fatal("expected duplicate site id error, got nil")
	}
	if manager.GetSite(1) != nil {
		t.Error("no site should be published after a failed start")
	}
}

// TestStopWithoutStart ensures Stop is safe when Start never got as far as
// creating the scheduler.
func TestStopWithoutStart(t *testing.T) {
	logger := zerolog.New(io.Discard)
	site := &Site{Name: "NeverStarted", logger: &logger}
	site.Stop() // must not panic
}

// TestMarshalJSONShape checks the API summary shape for sites.
func TestMarshalJSONShape(t *testing.T) {
	logger := zerolog.New(io.Discard)
	site := &Site{SiteID: 3, Name: "HQ", logger: &logger}
	data, err := site.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"ID":3,"Name":"HQ","lastPolled":"0001-01-01T00:00:00Z"}`
	if string(data) != expected {
		t.Errorf("unexpected JSON: got %s, want %s", data, expected)
	}
}

func newLocalIDFieldSite(t *testing.T, handler func(w http.ResponseWriter, _ *http.Request)) *Site {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/customfieldnames", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	logger := zerolog.New(io.Discard)
	return &Site{
		BaseURL:          srv.URL,
		httpClient:       srv.Client(),
		Name:             "LocalID",
		logger:           &logger,
		localIDFieldName: "StaffID",
	}
}

// TestStartFailsWhenCustomFieldsUnavailable ensures startup fails fast when
// the custom field names cannot be fetched; an unresolved local ID field
// would make every user query invalid SQL.
func TestStartFailsWhenCustomFieldsUnavailable(t *testing.T) {
	site := newLocalIDFieldSite(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := site.Start(); err == nil {
		t.Fatal("expected Start to fail when custom fields cannot be fetched, got nil")
	}
	if site.LocalIDField != "" {
		t.Errorf("expected LocalIDField to stay empty, got %q", site.LocalIDField)
	}
}

// TestStartFailsWhenLocalIDFieldMissing ensures a configured field that the
// Net2 server does not offer is a startup error, not silent empty queries.
func TestStartFailsWhenLocalIDFieldMissing(t *testing.T) {
	site := newLocalIDFieldSite(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"OtherField","type":0,"maxLength":50}]`))
	})
	if err := site.Start(); err == nil {
		t.Fatal("expected Start to fail when the configured field is absent, got nil")
	}
}

// TestStartResolvesLocalIDField checks the successful path publishes the
// resolved query-field identifier.
func TestStartResolvesLocalIDField(t *testing.T) {
	site := newLocalIDFieldSite(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"StaffID","type":0,"maxLength":50}]`))
	})
	if err := site.Start(); err != nil {
		t.Fatalf("expected Start to succeed, got %v", err)
	}
	t.Cleanup(site.Stop)
	if site.LocalIDField != "Field1_100" {
		t.Errorf("expected LocalIDField Field1_100, got %q", site.LocalIDField)
	}
}
