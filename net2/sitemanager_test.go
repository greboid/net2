package net2

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greboid/net2/config"
	"github.com/rs/zerolog"
)

func discardLogger() *zerolog.Logger {
	logger := zerolog.New(io.Discard)
	return &logger
}

// newManagerStub serves just enough of the Net2 API for site startup and a
// full update, mirroring the race test fixtures.
func newManagerStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/doors", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testDoorsJSON)) })
	mux.HandleFunc("/api/v1/accesslevels/areas", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testAreasJSON)) })
	mux.HandleFunc("/api/v1/accesslevels", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testLevelsJSON)) })
	mux.HandleFunc("/api/v1/departments", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testDepartmentsJSON)) })
	mux.HandleFunc("/api/v1/users/customfieldnames", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testCustomFieldsJSON)) })
	mux.HandleFunc("/api/v1/users/1/doorpermissionset", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testPermissionJSON)) })
	mux.HandleFunc("/api/v1/customquery/querydb", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "UsersEx"):
			_, _ = w.Write([]byte(testUsersJSON))
		case strings.Contains(query, "devices"):
			_, _ = w.Write([]byte(testDevicesJSON))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})
	return httptest.NewTestServer(t, mux)
}

// newManagerSite builds a site pointed at the given stub server; the
// in-memory test server routes all requests to the stub regardless of the
// base URL.
func newManagerSite(srv *httptest.Server, id int) *Site {
	client := srv.Client() // must be called before reading srv.URL
	return &Site{
		BaseURL:          srv.URL,
		httpClient:       client,
		SiteID:           id,
		Name:             "Site",
		logger:           discardLogger(),
		localIDFieldName: "Field1_100",
		config:           &config.SiteConfig{},
	}
}

// newDeadSite points at a server that answers 404 to everything, so every
// update step fails.
func newDeadSite(t *testing.T) *Site {
	t.Helper()
	srv := httptest.NewTestServer(t, http.NewServeMux())
	client := srv.Client()
	return &Site{
		BaseURL:    srv.URL,
		httpClient: client,
		SiteID:     99,
		Name:       "Dead Site",
		logger:     discardLogger(),
		config:     &config.SiteConfig{},
	}
}

func TestSiteManagerLifecycle(t *testing.T) {
	stub := newManagerStub(t)
	manager := &SiteManager{Logger: discardLogger()}

	if manager.GetSite(1) != nil {
		t.Error("expected no site before start")
	}
	if manager.GetSites() != nil {
		t.Error("expected nil site map before start")
	}
	if manager.Count() != 0 {
		t.Error("expected zero sites before start")
	}
	manager.Stop() // must be a no-op before start

	sites := []*Site{newManagerSite(stub, 1), newManagerSite(stub, 2)}
	if err := manager.Start(sites); err != nil {
		t.Fatalf("expected manager to start, got %v", err)
	}
	if manager.GetSite(1) == nil || manager.GetSite(2) == nil {
		t.Error("expected both sites to be published")
	}
	if manager.GetSite(3) != nil {
		t.Error("expected unknown site id to return nil")
	}
	if manager.Count() != 2 {
		t.Errorf("expected 2 sites, got %d", manager.Count())
	}
	if got := len(manager.GetSites()); got != 2 {
		t.Errorf("expected site map of 2, got %d", got)
	}

	manager.UpdateAll()
	for _, id := range []int{1, 2} {
		if got := len(manager.GetSite(id).GetUsers()); got != 2 {
			t.Errorf("site %d: expected 2 users after update, got %d", id, got)
		}
	}

	manager.Stop()
	if manager.GetSite(1) != nil {
		t.Error("expected no sites after stop")
	}
	if manager.GetSites() != nil {
		t.Error("expected nil site map after stop")
	}
	if manager.Count() != 0 {
		t.Errorf("expected zero sites after stop, got %d", manager.Count())
	}

	// Stopping an already-stopped site surfaces the scheduler error.
	sites[0].Stop()
}

func TestSiteManagerStartIsIdempotent(t *testing.T) {
	stub := newManagerStub(t)
	manager := &SiteManager{Logger: discardLogger()}
	sites := []*Site{newManagerSite(stub, 1)}
	if err := manager.Start(sites); err != nil {
		t.Fatalf("expected first start to succeed, got %v", err)
	}
	t.Cleanup(manager.Stop)
	if err := manager.Start(sites); err != nil {
		t.Errorf("expected second start to be a no-op, got %v", err)
	}
	if manager.Count() != 1 {
		t.Errorf("expected still 1 site, got %d", manager.Count())
	}
}

// TestSiteManagerStartFailsAndRollsBack ensures a site that cannot start is
// rolled back together with any sites that did start.
func TestSiteManagerStartFailsAndRollsBack(t *testing.T) {
	stub := newManagerStub(t)
	broken := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	manager := &SiteManager{Logger: discardLogger()}
	err := manager.Start([]*Site{
		newManagerSite(stub, 1),
		newManagerSite(broken, 2),
	})
	if err == nil {
		t.Fatal("expected start to fail for the broken site, got nil")
	}
	if manager.GetSite(1) != nil {
		t.Error("expected no sites to be published after failed start")
	}
	if manager.Count() != 0 {
		t.Errorf("expected zero sites after failed start, got %d", manager.Count())
	}
}
