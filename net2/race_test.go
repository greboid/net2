package net2

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/greboid/net2/config"
	"github.com/rs/zerolog"
)

const (
	testDoorsJSON        = `[{"id":1,"name":"Front Door"},{"id":2,"name":"Back Door"}]`
	testLevelsJSON       = `[{"id":1,"name":"Standard"},{"id":2,"name":"Manager"}]`
	testAreasJSON        = `[{"areaID":5,"name":"Lobby"}]`
	testDepartmentsJSON  = `[{"Id":3,"Name":"Staff"},{"Id":4,"Name":"Visitors"}]`
	testCustomFieldsJSON = `[{"id":1,"name":"Field1_100","type":0,"maxLength":50}]`
	testDevicesJSON      = `[{"Address":1,"StatusFlag":2},{"Address":2,"StatusFlag":0}]`
	testUsersJSON        = `[` +
		`{"UserGUID":"g1","userID":1,"FirstName":"Alice","Surname":"Smith","ActivateDate":"2024-01-01T10:00:00","ExpiryDate":"2099-01-01T10:00:00","PIN":"1234","lastAccessTime":"2024-05-01T10:00:00","lastKnownLocation":"Desk","DepartmentID":3,"DepartmentName":"Staff","AccessLevelName":"Individual: Full","LocalID":"L1"},` +
		`{"UserGUID":"g2","userID":2,"FirstName":"Bob","Surname":"Jones","ActivateDate":"2024-01-02T10:00:00","ExpiryDate":"2099-01-02T10:00:00","PIN":"5678","lastAccessTime":"2024-05-02T10:00:00","lastKnownLocation":"Lab","DepartmentID":4,"DepartmentName":"Visitors","AccessLevelName":"Standard","LocalID":"L2"}` +
		`]`
	testPermissionJSON = `{"accessLevels":[1],"individualPermissions":[{"id":99,"name":"Ghost Permission"}]}`
)

func newTestSite(t *testing.T) *Site {
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zerolog.New(io.Discard)
	site := &Site{
		BaseURL:    srv.URL,
		httpClient: srv.Client(),
		SiteID:     1,
		Name:       "Test Site",
		LastPolled: time.Time{},
		logger:     &logger,
		config: &config.SiteConfig{
			MonitoredDoors: []config.MonitoredDoor{{ID: 1, Name: "Front Door Override", Zone: "ZoneA"}},
		},
	}
	site.Users = make(map[int]*User)
	site.Departments = make(map[int]*Department)
	site.AccessLevels = make(map[int]*AccessLevel)
	site.Doors = make(map[uint64]*Door)
	return site
}

// TestConcurrentUpdateAndReads hammers the state readers while the update
// loop replaces the published maps; run with -race to verify the locking.
func TestConcurrentUpdateAndReads(t *testing.T) {
	site := newTestSite(t)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				site.UpdateAll()
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = site.GetUsers()
				_ = site.GetUser(1)
				_ = site.GetDoors()
				_ = site.GetDoor(1)
				_ = site.GetMonitoredDoors()
				_ = site.GetAccessLevels()
				_ = site.GetDepartments()
				_ = site.UpToDate()
				_ = site.GetStaff()
				_ = site.GetVisitors()
				_ = site.hasDoor(1)
				_, _ = site.GetUserPictureByLocalID(1)
				_ = site.RemoveUserAccessLevel(1, 1)
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()

	// Sanity-check the final state.
	if got := len(site.GetUsers()); got != 2 {
		t.Errorf("expected 2 users, got %d", got)
	}
	user := site.GetUser(1)
	if user == nil {
		t.Fatal("expected user 1 to exist")
	}
	if len(user.AccessLevels) != 1 || user.AccessLevels[0] != "Standard" {
		t.Errorf("expected user 1 access levels [Standard], got %v", user.AccessLevels)
	}
	doors := site.GetDoors()
	if len(doors) != 2 {
		t.Errorf("expected 2 doors, got %d", len(doors))
	}
	door := site.GetDoor(1)
	if door == nil {
		t.Fatal("expected door 1 to exist")
	}
	if door.AlarmZone != "ZoneA" {
		t.Errorf("expected door 1 alarm zone ZoneA, got %q", door.AlarmZone)
	}
	monitored := site.GetMonitoredDoors()
	if len(monitored) != 1 {
		t.Fatalf("expected 1 monitored door, got %d", len(monitored))
	}
	if monitored[1].Name != "Front Door Override" {
		t.Errorf("expected monitored door name override, got %q", monitored[1].Name)
	}
	// The override must not mutate the published door.
	if site.GetDoor(1).Name != "Front Door" {
		t.Errorf("published door name was mutated: %q", site.GetDoor(1).Name)
	}
	if !site.UpToDate() {
		t.Error("expected site to be up to date after successful update")
	}
}
