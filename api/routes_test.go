package api

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greboid/net2/config"
	"github.com/greboid/net2/net2"
	"github.com/rs/zerolog"
)

const (
	apiDoorsJSON       = `[{"id":8,"name":"Front"},{"id":42,"name":"Back"},{"id":999,"name":"Broken"}]`
	apiLevelsJSON      = `[{"id":1,"name":"Standard"},{"id":2,"name":"Manager"}]`
	apiAreasJSON       = `[{"areaID":5,"name":"Lobby"}]`
	apiDepartmentsJSON = `[{"Id":3,"Name":"Staff - Front Desk"},{"Id":4,"Name":"Visitors"},{"Id":5,"Name":"Cleaners - Night"},{"Id":6,"Name":"Cancelled Users"}]`
	apiDevicesJSON     = `[{"Address":8,"StatusFlag":2},{"Address":42,"StatusFlag":0}]`
	apiPermissionJSON  = `{"accessLevels":[1],"individualPermissions":[]}`
)

// apiUsersJSON builds the custom-query user rows with timestamps relative to
// the current day so the "today" filters have data to return.
func apiUsersJSON() string {
	lastAccess := time.Now().Format("2006-01-02T15:04:05")
	expiry := time.Now().AddDate(30, 0, 0).Format("2006-01-02T15:04:05")
	user := func(id int, firstName, surname, departmentID, departmentName, localID string) string {
		return fmt.Sprintf(`{"UserGUID":"g%d","userID":%d,"FirstName":"%s","Surname":"%s","ActivateDate":"%s","ExpiryDate":"%s","PIN":"%d","lastKnownLocation":"Desk","lastAccessTime":"%s","DepartmentID":%s,"DepartmentName":"%s","AccessLevelName":"Standard","LocalID":"%s"}`,
			id, id, firstName, surname, lastAccess, expiry, 1000+id, lastAccess, departmentID, departmentName, localID)
	}
	return "[" + strings.Join([]string{
		user(7, "Alice", "Smith", "3", "Staff - Front Desk", "5001"),
		user(8, "Bob", "Jones", "4", "Visitors", "5002"),
		user(12, "Carol", "White", "3", "Staff - Front Desk", "5003"),
		user(13, "Eve", "Gone", "6", "Cancelled Users", "5005"),
		user(998, "Dan", "Broken", "4", "Visitors", "5004"),
	}, ",") + "]"
}

// apiDoorCommandHandler simulates a Net2 door command endpoint: door 999
// always fails, and when failDoor42 is set door 42 fails too so the error
// branches of the door handlers can be exercised.
func apiDoorCommandHandler(failDoor42 bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "999") || (failDoor42 && strings.Contains(string(body), "42")) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// newTestRoutes builds a fully routed server over a started site backed by
// a stub Net2 API, so requests exercise the validators, the context
// handoff, and the real site methods behind every handler.
func newTestRoutes(t *testing.T) http.Handler {
	t.Helper()
	logger := zerolog.New(io.Discard)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/authorization/tokens", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"testtoken","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("/api/v1/users/customfieldnames", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"StaffID","type":0,"maxLength":50}]`))
	})
	mux.HandleFunc("/api/v1/doors", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(apiDoorsJSON)) })
	mux.HandleFunc("/api/v1/accesslevels", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(apiLevelsJSON)) })
	mux.HandleFunc("/api/v1/accesslevels/areas", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(apiAreasJSON)) })
	mux.HandleFunc("/api/v1/departments", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(apiDepartmentsJSON)) })
	mux.HandleFunc("/api/v1/users/7/doorpermissionset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "9999") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(apiPermissionJSON))
	})
	mux.HandleFunc("/api/v1/customquery/querydb", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		switch {
		case strings.Contains(query, "UsersEx"):
			_, _ = w.Write([]byte(apiUsersJSON()))
		case strings.Contains(query, "devices"):
			_, _ = w.Write([]byte(apiDevicesJSON))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})
	mux.HandleFunc("/api/v1/commands/door/open", apiDoorCommandHandler(false))
	mux.HandleFunc("/api/v1/commands/door/close", apiDoorCommandHandler(true))
	mux.HandleFunc("/api/v1/commands/door/control", apiDoorCommandHandler(true))
	mux.HandleFunc("/api/v1/commands/antipassback/reset", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "998") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/image"):
			switch {
			case strings.HasSuffix(r.URL.Path, "/users/8/image"):
				w.WriteHeader(http.StatusNotFound)
			case strings.HasSuffix(r.URL.Path, "/users/998/image"):
				w.WriteHeader(http.StatusInternalServerError)
			default:
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write([]byte("jpegbytes"))
			}
		case strings.HasSuffix(r.URL.Path, "/doorpermissionset") && r.Method == http.MethodPut:
			if strings.Contains(string(body), "9999") || strings.Contains(r.URL.Path, "/users/998/") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/departments") && r.Method == http.MethodPut:
			if strings.Contains(r.URL.Path, "/users/998/") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut:
			if strings.Contains(string(body), "998") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	// The site config built below needs a real reachable address because
	// net2.GetSites constructs its own OAuth client, so start the server on
	// the loopback interface rather than using the in-memory network.
	srv := httptest.NewTestServer(t, mux)
	srv.Start()
	address := srv.Listener.Addr().(*net.TCPAddr)

	sites := net2.GetSites(&config.Config{
		ClientID: "test",
		Sites: []config.SiteConfig{
			{
				ID: 1, Name: "Test Site", IP: "127.0.0.1", Port: address.Port,
				Username: "u", Password: "p", LocalIDField: "StaffID",
				StaffDeptPrefix: "Staff", VisitorDeptPrefix: "Visitor",
				ContractorDeptPrefix: "Contractor", CleanerDeptPrefix: "Clean",
				CustomerDeptPrefix: "Customer", CancelledDeptPrefix: "Cancelled",
				MonitoredDoors: []config.MonitoredDoor{{ID: 8, Name: "Front Entrance", Zone: "Lobby"}},
				OpenableDoors: []config.OpenableDoor{
					{Name: "Garage", Sequence: []config.DoorSequence{{ID: 8}, {ID: 42}}},
				},
			},
		},
	}, &logger)
	manager := &net2.SiteManager{Logger: &logger}
	if err := manager.Start(sites); err != nil {
		t.Fatalf("unable to start site manager: %v", err)
	}
	t.Cleanup(manager.Stop)

	site := manager.GetSite(1)
	site.UpdateAll()
	site.UnknownTokens = []net2.Event{{Token: 12345, Location: "Front Door"}}

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
		{"unknown user", "/api/v1/sites/1/users/777", http.StatusNotFound, "userID not found"},
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

func TestSequenceDoorDataBindAcceptsAnything(t *testing.T) {
	data := &SequenceDoorData{Door: "8", Time: "0s"}
	if err := data.Bind(nil); err != nil {
		t.Errorf("expected Bind to always succeed, got %v", err)
	}
}
