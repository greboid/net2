package net2

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/greboid/net2/config"
)

// setUserPrefixes configures the department prefixes the filters rely on;
// the shared test site config leaves them empty, which would match every
// department.
func setUserPrefixes(site *Site) {
	site.config.StaffDeptPrefix = "Staff"
	site.config.VisitorDeptPrefix = "Visitor"
	site.config.ContractorDeptPrefix = "Contractor"
	site.config.CleanerDeptPrefix = "Clean"
	site.config.CustomerDeptPrefix = "Customer"
	site.config.CancelledDeptPrefix = "Cancelled"
}

func newFilterSite(t *testing.T) *Site {
	t.Helper()
	site := newTestSite(t)
	setUserPrefixes(site)
	now := time.Now()
	site.Users = map[int]*User{
		1: {ID: 1, Departments: []Department{{ID: 3, Name: "Staff - Reception"}}, Expiry: now.Add(24 * time.Hour), LastUpdated: now},
		2: {ID: 2, Departments: []Department{{ID: 3, Name: "Staff - Bar"}}, Expiry: now.Add(-24 * time.Hour), LastUpdated: now.AddDate(0, 0, -2)},
		3: {ID: 3, Departments: []Department{{ID: 4, Name: "Visitors"}}, Expiry: now.Add(24 * time.Hour), LastUpdated: now},
		4: {ID: 4, Departments: []Department{{ID: 5, Name: "Contractors"}}, Expiry: now.Add(24 * time.Hour), LastUpdated: now},
		5: {ID: 5, Departments: []Department{{ID: 6, Name: "Cleaners"}}, Expiry: now.Add(24 * time.Hour), LastUpdated: now},
		6: {ID: 6, Departments: []Department{{ID: 7, Name: "Customers"}}, Expiry: now.Add(24 * time.Hour), LastUpdated: now},
		7: {ID: 7, Departments: []Department{{ID: 8, Name: "Cancelled People"}}, Expiry: now.Add(24 * time.Hour), LastUpdated: now},
		// User 8 has a zero expiry, which the active filters must treat as
		// never-expiring.
		8: {ID: 8, Departments: []Department{{ID: 3, Name: "Staff - Reception"}}, LastUpdated: now},
	}
	return site
}

func userIDs(users map[int]*User) []int {
	ids := make([]int, 0, len(users))
	for id := range users {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func assertUserIDs(t *testing.T, name string, got map[int]*User, want ...int) {
	t.Helper()
	gotIDs := userIDs(got)
	if !slices.Equal(gotIDs, want) {
		t.Errorf("%s: expected users %v, got %v", name, want, gotIDs)
	}
}

func TestDepartmentFilters(t *testing.T) {
	site := newFilterSite(t)

	assertUserIDs(t, "GetStaff", site.GetStaff(), 1, 2, 8)
	assertUserIDs(t, "GetActiveStaff", site.GetActiveStaff(), 1, 8)
	assertUserIDs(t, "GetActiveStaffToday", site.GetActiveStaffToday(), 1, 8)
	assertUserIDs(t, "GetVisitors", site.GetVisitors(), 3)
	assertUserIDs(t, "GetActiveVisitors", site.GetActiveVisitors(), 3)
	assertUserIDs(t, "GetActiveVisitorsToday", site.GetActiveVisitorsToday(), 3)
	assertUserIDs(t, "GetContractors", site.GetContractors(), 4)
	assertUserIDs(t, "GetActiveContractors", site.GetActiveContractors(), 4)
	assertUserIDs(t, "GetActiveContractorsToday", site.GetActiveContractorsToday(), 4)
	assertUserIDs(t, "GetCleaners", site.GetCleaners(), 5)
	assertUserIDs(t, "GetActiveCleaners", site.GetActiveCleaners(), 5)
	assertUserIDs(t, "GetActiveCleanersToday", site.GetActiveCleanersToday(), 5)
	assertUserIDs(t, "GetCustomers", site.GetCustomers(), 6)
	assertUserIDs(t, "GetActiveCustomers", site.GetActiveCustomers(), 6)
	assertUserIDs(t, "GetActiveCustomersToday", site.GetActiveCustomersToday(), 6)
	assertUserIDs(t, "GetActiveUsers", site.GetActiveUsers(), 1, 3, 4, 5, 6, 7, 8)
	assertUserIDs(t, "GetActiveUsersToday", site.GetActiveUsersToday(), 1, 3, 4, 5, 6, 7, 8)
	assertUserIDs(t, "GetActiveNonStaff", site.GetActiveNonStaff(), 3, 4, 5, 6, 7)

	cancelled, ok := site.GetCancelledUsers().(map[int]*User)
	if !ok {
		t.Fatalf("GetCancelledUsers did not return a user map")
	}
	assertUserIDs(t, "GetCancelledUsers", cancelled, 7)
}

func TestGetUnknownTokens(t *testing.T) {
	site := newTestSite(t)
	tokens := site.GetUnknownTokens()
	if tokens == nil {
		t.Fatal("expected empty non-nil slice for no tokens, got nil")
	}
	if len(tokens) != 0 {
		t.Errorf("expected no tokens, got %d", len(tokens))
	}
	site.UnknownTokens = []Event{{Token: 12345, Location: "Front Door"}}
	tokens = site.GetUnknownTokens()
	if len(tokens) != 1 || tokens[0].Token != 12345 {
		t.Errorf("expected the recorded token event, got %v", tokens)
	}
}

func TestGetBlankPicture(t *testing.T) {
	site := newTestSite(t)
	picture, err := site.GetBlankPicture()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(picture) == 0 {
		t.Error("expected non-empty blank picture")
	}
}

func TestGetUserPicture(t *testing.T) {
	site := newTestSite(t)
	picture, err := site.GetUserPicture(1)
	if err != nil {
		t.Fatalf("expected picture for user 1, got %v", err)
	}
	if string(picture) != "jpegbytes" {
		t.Errorf("unexpected picture body %q", picture)
	}

	// A missing picture falls back to the "photo needed" placeholder; the
	// stub serves a 404 image for user 8.
	placeholder, err := site.GetUserPicture(8)
	if err != nil {
		t.Fatalf("expected placeholder for missing picture, got %v", err)
	}
	if string(placeholder) == "jpegbytes" {
		t.Error("expected placeholder to differ from a real picture")
	}

	if _, err := site.GetUserPicture(998); err == nil {
		t.Error("expected error for failed picture request, got nil")
	}
}

func TestGetUserPictureByLocalID(t *testing.T) {
	site := newTestSite(t)
	site.Users = map[int]*User{
		1: {ID: 1, LocalID: "5001"},
		2: {ID: 2, LocalID: "5002"},
		3: {ID: 3, LocalID: "5001"},
	}
	picture, err := site.GetUserPictureByLocalID(5002)
	if err != nil {
		t.Fatalf("expected picture for local ID 5002, got %v", err)
	}
	if string(picture) != "jpegbytes" {
		t.Errorf("unexpected picture body %q", picture)
	}
	if _, err := site.GetUserPictureByLocalID(5001); err == nil {
		t.Error("expected error when several users share a local ID, got nil")
	}
	if _, err := site.GetUserPictureByLocalID(9999); err == nil {
		t.Error("expected error for unknown local ID, got nil")
	}
}

func TestGetOpenableDoors(t *testing.T) {
	site := newTestSite(t)
	site.config.OpenableDoors = []config.OpenableDoor{
		{Name: "Garage", Sequence: []config.DoorSequence{{ID: 8, Duration: config.Duration(time.Second)}}},
	}
	doors := site.GetOpenableDoors()
	sequence, ok := doors["Garage"]
	if !ok {
		t.Fatalf("expected openable door Garage, got %v", doors)
	}
	if len(sequence) != 1 || sequence[0].ID != 8 {
		t.Errorf("unexpected sequence for Garage: %v", sequence)
	}
}

func TestDoorCommands(t *testing.T) {
	site := newTestSite(t)
	site.Doors = map[uint64]*Door{8: {ID: 8}, 42: {ID: 42}, 999: {ID: 999}}

	if err := site.OpenDoor(8); err != nil {
		t.Errorf("expected OpenDoor(8) to succeed, got %v", err)
	}
	if err := site.OpenDoor(999); err == nil {
		t.Error("expected OpenDoor of an unknown door to fail")
	}

	if err := site.OpenDoorWithRelay(8, false); err != nil {
		t.Errorf("expected relay 1 open to succeed, got %v", err)
	}
	if err := site.OpenDoorWithRelay(8, true); err != nil {
		t.Errorf("expected relay 2 open to succeed, got %v", err)
	}
	if err := site.OpenDoorWithRelay(42, false); err == nil {
		t.Error("expected relay open of failing door to fail")
	}
	if err := site.OpenDoorWithRelay(999, false); err == nil {
		t.Error("expected relay open of an unknown door to fail")
	}

	if err := site.CloseDoor(8); err != nil {
		t.Errorf("expected CloseDoor(8) to succeed, got %v", err)
	}
	if err := site.CloseDoor(42); err == nil {
		t.Error("expected CloseDoor of failing door to fail")
	}
	if err := site.CloseDoor(999); err == nil {
		t.Error("expected CloseDoor of an unknown door to fail")
	}
}

func TestResetAntiPassback(t *testing.T) {
	site := newTestSite(t)
	if err := site.ResetAntiPassback(1); err != nil {
		t.Errorf("expected reset to succeed, got %v", err)
	}
	if err := site.ResetAntiPassback(998); err == nil {
		t.Error("expected reset for failing user to fail")
	}
}

func TestUpdateUserInfo(t *testing.T) {
	site := newTestSite(t)

	if err := site.UpdateUserInfo(1, map[string]any{"ExpiryDate": GetTomorrow()}); err != nil {
		t.Errorf("expected update with explicit expiry to succeed, got %v", err)
	}

	// Without an explicit expiry the cached user expiry is used.
	site.Users[1] = &User{ID: 1, Expiry: GetTomorrow()}
	if err := site.UpdateUserInfo(1, map[string]any{"PIN": "4321"}); err != nil {
		t.Errorf("expected update with cached expiry to succeed, got %v", err)
	}

	if err := site.UpdateUserInfo(404, map[string]any{"PIN": "4321"}); err == nil {
		t.Error("expected update of unknown user without expiry to fail")
	}
	if err := site.UpdateUserInfo(998, map[string]any{"ExpiryDate": GetTomorrow()}); err == nil {
		t.Error("expected update rejected by the server to fail")
	}
}

func TestActivateDeactivateUser(t *testing.T) {
	site := newTestSite(t)
	site.Users[1] = &User{ID: 1, Expiry: GetTomorrow()}
	if err := site.ActivateUser(1); err != nil {
		t.Errorf("expected activate to succeed, got %v", err)
	}
	if err := site.DeactivateUser(1); err != nil {
		t.Errorf("expected deactivate to succeed, got %v", err)
	}
}

func TestChangeUserDepartment(t *testing.T) {
	site := newTestSite(t)
	site.UpdateAll()
	if err := site.ChangeUserDepartment(1, 3); err != nil {
		t.Errorf("expected department change to succeed, got %v", err)
	}
	if err := site.ChangeUserDepartment(1, 999); err == nil {
		t.Error("expected department change to unknown department to fail")
	}
	if err := site.ChangeUserDepartment(998, 3); err == nil {
		t.Error("expected department change rejected by the server to fail")
	}
}

func TestUserAccessLevelMethods(t *testing.T) {
	site := newTestSite(t)
	site.UpdateAll()

	if err := site.UpdateUserAccessLevel(1, 2); err != nil {
		t.Errorf("expected access level update to succeed, got %v", err)
	}
	if err := site.UpdateUserAccessLevel(1, -1); err != nil {
		t.Errorf("expected clearing via level -1 to succeed, got %v", err)
	}
	if err := site.UpdateUserAccessLevel(1, 9999); err == nil {
		t.Error("expected access level update rejected by the server to fail")
	}
	if err := site.SetUserAccessLevel(1, 2); err != nil {
		t.Errorf("expected set access level to succeed, got %v", err)
	}
	if err := site.AddUserAccessLevel(1, 2); err != nil {
		t.Errorf("expected add access level to succeed, got %v", err)
	}
	if err := site.AddUserAccessLevel(998, 2); err == nil {
		t.Error("expected add access level for unknown user to fail")
	}
	if err := site.RemoveUserAccessLevel(1, 1); err != nil {
		t.Errorf("expected remove access level to succeed, got %v", err)
	}
	if err := site.RemoveUserAccessLevel(998, 1); err == nil {
		t.Error("expected remove access level for unknown user to fail")
	}
}

func TestUpdateUserNameAndExpiryAndAccessLevel(t *testing.T) {
	site := newTestSite(t)
	site.UpdateAll()
	if err := site.UpdateUserNameAndExpiryAndAccessLevel(1, "Alice", "Smith", GetTomorrow(), 1); err != nil {
		t.Errorf("expected combined update to succeed, got %v", err)
	}
	if err := site.UpdateUserNameAndExpiryAndAccessLevel(1, "Alice", "Smith", GetTomorrow(), 9999); err == nil {
		t.Error("expected combined update with failing access level to fail")
	}
}

func TestSequenceDoor(t *testing.T) {
	site := newTestSite(t)
	site.Doors = map[uint64]*Door{8: {ID: 8}, 999: {ID: 999}}
	// A failing door must be logged and skipped, not panic or block.
	site.SequenceDoor(
		DoorSequenceItem{Door: 8},
		DoorSequenceItem{Door: 999},
	)
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TestSequenceDoorOrderAndTiming uses a fake clock (testing/synctest) and a
// fake network transport to verify doors are opened in sequence order and
// that the configured pauses are honoured between them, without taking any
// real time.
func TestSequenceDoorOrderAndTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var opened []uint64
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			var payload struct {
				DoorID uint64 `json:"doorId"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				return nil, err
			}
			mu.Lock()
			opened = append(opened, payload.DoorID)
			mu.Unlock()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})
		site := &Site{
			BaseURL:    "http://net2.example",
			httpClient: &http.Client{Transport: transport},
			logger:     discardLogger(),
			config:     &config.SiteConfig{},
			Doors:      map[uint64]*Door{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}},
		}

		start := time.Now()
		site.SequenceDoor(
			DoorSequenceItem{Door: 1, Time: time.Hour},
			DoorSequenceItem{Door: 2, Time: time.Minute},
			DoorSequenceItem{Door: 3},
		)
		if elapsed := time.Since(start); elapsed != time.Hour+time.Minute {
			t.Errorf("expected sequence to take exactly 1h2m of sequence time, got %v", elapsed)
		}
		if !slices.Equal(opened, []uint64{1, 2, 3}) {
			t.Errorf("expected doors opened in order [1 2 3], got %v", opened)
		}
	})
}

func TestGetTomorrowAndYesterday(t *testing.T) {
	now := time.Now()
	tomorrow := GetTomorrow()
	if !tomorrow.After(now) {
		t.Errorf("expected tomorrow %v to be after now %v", tomorrow, now)
	}
	if tomorrow.Hour() != 23 || tomorrow.Minute() != 59 {
		t.Errorf("expected tomorrow at 23:59, got %02d:%02d", tomorrow.Hour(), tomorrow.Minute())
	}
	yesterday := GetYesterday()
	if !yesterday.Before(now) {
		t.Errorf("expected yesterday %v to be before now %v", yesterday, now)
	}
}

func TestGetSitesFromConfig(t *testing.T) {
	logger := discardLogger()
	conf := &config.Config{
		ClientID: "client",
		Sites: []config.SiteConfig{
			{ID: 1, Name: "Plain", IP: "10.0.0.1", Port: 8080, Username: "u", Password: "p"},
			{ID: 2, Name: "Secure", IP: "10.0.0.2", Port: 8443, Https: true, Username: "u", Password: "p"},
		},
	}
	sites := GetSites(conf, logger)
	if len(sites) != 2 {
		t.Fatalf("expected 2 sites, got %d", len(sites))
	}
	if sites[0].BaseURL != "http://10.0.0.1:8080" {
		t.Errorf("unexpected plain site URL %q", sites[0].BaseURL)
	}
	if sites[1].BaseURL != "https://10.0.0.2:8443" {
		t.Errorf("unexpected secure site URL %q", sites[1].BaseURL)
	}
	if sites[0].SiteID != 1 || sites[0].Name != "Plain" {
		t.Errorf("site fields not copied from config: %+v", sites[0])
	}
	if sites[0].QuitChan == nil {
		t.Error("expected quit channel to be created")
	}
}

// TestUpdateAccessLevelsFailsWhenAreasUnavailable covers the branch where
// the area query fails after the access level query succeeded.
func TestUpdateAccessLevelsFailsWhenAreasUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/accesslevels", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testLevelsJSON)) })
	mux.HandleFunc("/api/v1/accesslevels/areas", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	srv := httptest.NewTestServer(t, mux)
	client := srv.Client()

	site := &Site{BaseURL: srv.URL, httpClient: client, logger: discardLogger(), config: &config.SiteConfig{}}
	if err := site.UpdateAccessLevels(); err == nil {
		t.Fatal("expected UpdateAccessLevels to fail when areas cannot be fetched, got nil")
	}
}

// TestUpdateAllRecordsFailure ensures a full update against a dead server
// marks the site as not up to date instead of refreshing LastPolled.
func TestUpdateAllRecordsFailure(t *testing.T) {
	site := newDeadSite(t)
	site.UpdateAll()
	if site.UpToDate() {
		t.Error("expected site not to be up to date after failed update")
	}
	if !strings.Contains(site.Name, "Dead") {
		t.Errorf("unexpected site name %q", site.Name)
	}
}
