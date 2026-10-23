package api

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greboid/net2/config"
	"github.com/greboid/net2/net2"
	"github.com/rs/zerolog"
)

// newTestRoutes builds a fully routed server over a single started site so
// requests exercise the validators and the context handoff to handlers.
func newTestRoutes(t *testing.T) http.Handler {
	t.Helper()
	logger := zerolog.New(io.Discard)
	// Stub Net2 API: site startup must resolve the local ID field against it.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/authorization/tokens", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"testtoken","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/api/v1/users/customfieldnames", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"StaffID","type":0,"maxLength":50}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	address := srv.Listener.Addr().(*net.TCPAddr)

	sites := net2.GetSites(&config.Config{
		ClientID: "test",
		Sites: []config.SiteConfig{
			{ID: 1, Name: "Test Site", IP: "127.0.0.1", Port: address.Port, Username: "u", Password: "p", LocalIDField: "StaffID"},
		},
	}, &logger)
	manager := &net2.SiteManager{Logger: &logger}
	if err := manager.Start(sites); err != nil {
		t.Fatalf("unable to start site manager: %v", err)
	}
	t.Cleanup(manager.Stop)

	site := manager.GetSite(1)
	site.Users = map[int]*net2.User{7: {ID: 7, FirstName: "Alice"}}
	site.Doors = map[uint64]*net2.Door{8: {ID: 8, Name: "Front"}, 42: {ID: 42, Name: "Back"}}

	server := &Server{Sites: manager}
	return server.GetRoutes()
}

func TestRoutesResolveContextValues(t *testing.T) {
	handler := newTestRoutes(t)
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"known user", "/api/v1/sites/1/users/7", http.StatusOK, `"id":7`},
		{"unknown user", "/api/v1/sites/1/users/8", http.StatusNotFound, "userID not found"},
		{"unknown site", "/api/v1/sites/2/users", http.StatusNotFound, "siteID not found"},
		{"known door", "/api/v1/sites/1/doors/42", http.StatusOK, `"id":42`},
		{"leading zero door id parses as decimal", "/api/v1/sites/1/doors/008", http.StatusOK, `"id":8`},
		{"unknown door", "/api/v1/sites/1/doors/43", http.StatusNotFound, "doorID not found"},
		{"site summary", "/api/v1/sites/1", http.StatusOK, `"Name":"Test Site"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tc.wantBody) {
				t.Errorf("expected body to contain %q, got %q", tc.wantBody, recorder.Body.String())
			}
		})
	}
}

func TestUpdateUserDataBindReportsMissingLastName(t *testing.T) {
	data := &UpdateUserData{FirstName: "Alice"}
	err := data.Bind(nil)
	if err == nil {
		t.Fatal("expected error for missing LastName, got nil")
	}
	if !strings.Contains(err.Error(), "LastName") {
		t.Errorf("expected the error to mention LastName, got %q", err.Error())
	}
}

func TestUpdateUserDataBindReportsMissingFirstName(t *testing.T) {
	data := &UpdateUserData{LastName: "Smith"}
	err := data.Bind(nil)
	if err == nil {
		t.Fatal("expected error for missing FirstName, got nil")
	}
	if !strings.Contains(err.Error(), "FirstName") {
		t.Errorf("expected the error to mention FirstName, got %q", err.Error())
	}
}
