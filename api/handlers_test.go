package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

func doRequest(t *testing.T, handler http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// TestGetEndpoints covers every read-only route on the happy path; the
// wantBody assertions pin the source of the rendered data.
func TestGetEndpoints(t *testing.T) {
	handler := newTestRoutes(t)
	tests := []struct {
		name     string
		path     string
		wantBody string
	}{
		{"index", "/api/v1/", "Net2 Proxy API Index"},
		{"sites list", "/api/v1/sites/", "Test Site"},
		{"unknown tokens", "/api/v1/sites/1/unknownTokens", "12345"},
		{"up to date", "/api/v1/sites/1/uptodate", "true"},
		{"access levels", "/api/v1/sites/1/accesslevels/", "Standard"},
		{"departments", "/api/v1/sites/1/departments/", "Visitors"},
		{"doors", "/api/v1/sites/1/doors/", "Broken"},
		{"monitored doors", "/api/v1/sites/1/doors/monitored", "Front Entrance"},
		{"openable doors", "/api/v1/sites/1/doors/openable", "Garage"},
		{"users", "/api/v1/sites/1/users/", "Alice"},
		{"active users", "/api/v1/sites/1/users/active", "Alice"},
		{"active users today", "/api/v1/sites/1/users/activetoday", "Alice"},
		{"active staff", "/api/v1/sites/1/users/activestaff", "Alice"},
		{"active staff today", "/api/v1/sites/1/users/activestafftoday", "Alice"},
		{"active visitors", "/api/v1/sites/1/users/activevisitors", "Bob"},
		{"active visitors today", "/api/v1/sites/1/users/activevisitorstoday", "Bob"},
		{"active non staff", "/api/v1/sites/1/users/activenonstaff", "Bob"},
		{"cancelled users", "/api/v1/sites/1/users/cancelled", "Eve"},
		{"visitors", "/api/v1/sites/1/users/visitors", "Bob"},
		{"staff", "/api/v1/sites/1/users/staff", "Alice"},
		{"known user", "/api/v1/sites/1/users/7", "Alice"},
		{"user picture", "/api/v1/sites/1/users/7/picture", "jpegbytes"},
		{"user picture by local id", "/api/v1/sites/1/users/userpicturebylocalid/5001", "jpegbytes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doRequest(t, handler, http.MethodGet, tc.path, nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tc.wantBody) {
				t.Errorf("expected body to contain %q, got %q", tc.wantBody, recorder.Body.String())
			}
		})
	}
}

func TestGetEndpointsReturnEmptyCollections(t *testing.T) {
	handler := newTestRoutes(t)
	for _, path := range []string{
		"/api/v1/sites/1/users/contractors",
		"/api/v1/sites/1/users/cleaners",
		"/api/v1/sites/1/users/customers",
	} {
		recorder := doRequest(t, handler, http.MethodGet, path, nil)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: expected status 200, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestBlankPictureIsGif(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodGet, "/api/v1/sites/1/users/blankpicture", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/gif" {
		t.Errorf("expected image/gif content type, got %q", got)
	}
	if recorder.Body.Len() == 0 {
		t.Error("expected non-empty blank picture")
	}
}

func TestUserPictureByLocalIDNotFound(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodGet, "/api/v1/sites/1/users/userpicturebylocalid/99999", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Error getting picture") {
		t.Errorf("expected error message, got %q", recorder.Body.String())
	}
}

func TestUserPictureServerError(t *testing.T) {
	handler := newTestRoutes(t)
	// User 998 makes the stub image endpoint return 500.
	recorder := doRequest(t, handler, http.MethodGet, "/api/v1/sites/1/users/998/picture", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

// TestPostEndpoints covers every action route on the happy path.
func TestPostEndpoints(t *testing.T) {
	handler := newTestRoutes(t)
	tests := []struct {
		name     string
		path     string
		body     string
		wantBody string
	}{
		{"open door", "/api/v1/sites/1/doors/8/open", "", "Door opened"},
		{"relay 1", "/api/v1/sites/1/doors/8/relay1", "", "Door opened"},
		{"relay 2", "/api/v1/sites/1/doors/8/relay2", "", "Door opened"},
		{"close door", "/api/v1/sites/1/doors/8/close", "", "Door closed"},
		{"reset anti passback", "/api/v1/sites/1/users/7/resetantipassback", "", "Anti passback reset"},
		{"activate user", "/api/v1/sites/1/users/7/activate", "", "User activated"},
		{"deactivate user", "/api/v1/sites/1/users/7/deactivate", "", "User deactivated"},
		{"extend expiry", "/api/v1/sites/1/users/7/extendexpiry", "", "User expiry extended"},
		{"set access level", "/api/v1/sites/1/users/7/setaccesslevel?level=2", "", "User access level set"},
		{"add access level", "/api/v1/sites/1/users/7/addaccesslevel?level=2", "", "User access level added"},
		{"remove access level", "/api/v1/sites/1/users/7/removeaccesslevel?level=1", "", "User access level removed"},
		{"change department", "/api/v1/sites/1/users/7/changedepartment?department=4", "", "User department set"},
		{"open openable door", "/api/v1/sites/1/doors/openable/Garage/open", "", "Openable door sequence triggered"},
		{"activate department", "/api/v1/sites/1/departments/Staff%20-%20Front%20Desk/activate", "", "Successfully activated 2 users in department 'Staff - Front Desk'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doRequest(t, handler, http.MethodPost, tc.path, strings.NewReader(tc.body))
			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tc.wantBody) {
				t.Errorf("expected body to contain %q, got %q", tc.wantBody, recorder.Body.String())
			}
		})
	}
}

func TestSequenceDoors(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodPost, "/api/v1/sites/1/doors/sequence",
		strings.NewReader(`[{"door":"8","time":"0s"},{"door":"42"}]`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Sequence triggered") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}
}

func TestSequenceDoorsDefaultsMissingTime(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodPost, "/api/v1/sites/1/doors/sequence",
		strings.NewReader(`[{"door":"8"}]`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSequenceDoorsBadRequests(t *testing.T) {
	handler := newTestRoutes(t)
	tests := []struct {
		name string
		body io.Reader
	}{
		{"malformed json", strings.NewReader(`[{"door":`)},
		{"invalid duration", strings.NewReader(`[{"door":"8","time":"later"}]`)},
		{"invalid door number", strings.NewReader(`[{"door":"garage"}]`)},
		{"unreadable body", iotest.ErrReader(errors.New("body unreadable"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doRequest(t, handler, http.MethodPost, "/api/v1/sites/1/doors/sequence", tc.body)
			if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusInternalServerError {
				t.Fatalf("expected status 400 or 500, got %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "Error sequencing doors") {
				t.Errorf("unexpected body %q", recorder.Body.String())
			}
		})
	}
}

// TestPostEndpointsErrors covers the failure branches of every action
// route: stub sentinels make door 999, user 998 and access level 9999 fail.
func TestPostEndpointsErrors(t *testing.T) {
	handler := newTestRoutes(t)
	validBody := `{"FirstName":"Alice","LastName":"Smith","AccessLevel":1}`
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{"open failing door", "/api/v1/sites/1/doors/999/open", "", 500, "Error opening door"},
		{"relay 1 on failing door", "/api/v1/sites/1/doors/999/relay1", "", 500, "Error opening door"},
		{"relay 2 on failing door", "/api/v1/sites/1/doors/999/relay2", "", 500, "Error opening door"},
		{"close failing door", "/api/v1/sites/1/doors/999/close", "", 500, "Error closing door"},
		{"unknown openable door", "/api/v1/sites/1/doors/openable/Nope/open", "", 404, "openable door not found"},
		{"unknown department", "/api/v1/sites/1/departments/Nope/activate", "", 404, "department not found"},
		{"reset anti passback failure", "/api/v1/sites/1/users/998/resetantipassback", "", 500, "Error resetting anti passback"},
		{"activate failure", "/api/v1/sites/1/users/998/activate", "", 500, "Error activating user"},
		{"deactivate failure", "/api/v1/sites/1/users/998/deactivate", "", 500, "Error deactivating user"},
		{"activate and update failure", "/api/v1/sites/1/users/998/activateAndUpdate", validBody, 500, "Error activating user"},
		{"deactivate and update failure", "/api/v1/sites/1/users/998/deactivateAndUpdate", validBody, 500, "Error deactivating user"},
		{"extend expiry failure", "/api/v1/sites/1/users/998/extendexpiry", "", 500, "Error extending user expiry"},
		{"set access level failure", "/api/v1/sites/1/users/7/setaccesslevel?level=9999", "", 500, "error setting access level"},
		{"add access level failure", "/api/v1/sites/1/users/7/addaccesslevel?level=9999", "", 500, "error adding access level"},
		{"remove access level failure", "/api/v1/sites/1/users/998/removeaccesslevel?level=1", "", 500, "error removing access level"},
		{"change department failure", "/api/v1/sites/1/users/7/changedepartment?department=999", "", 500, "error changing user department"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doRequest(t, handler, http.MethodPost, tc.path, strings.NewReader(tc.body))
			if recorder.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tc.wantBody) {
				t.Errorf("expected body to contain %q, got %q", tc.wantBody, recorder.Body.String())
			}
		})
	}
}

func TestAccessLevelAndDepartmentParamsMustBeNumeric(t *testing.T) {
	handler := newTestRoutes(t)
	tests := []struct {
		name string
		path string
	}{
		{"set access level missing", "/api/v1/sites/1/users/7/setaccesslevel"},
		{"set access level non numeric", "/api/v1/sites/1/users/7/setaccesslevel?level=abc"},
		{"add access level missing", "/api/v1/sites/1/users/7/addaccesslevel"},
		{"remove access level non numeric", "/api/v1/sites/1/users/7/removeaccesslevel?level=abc"},
		{"change department missing", "/api/v1/sites/1/users/7/changedepartment"},
		{"change department non numeric", "/api/v1/sites/1/users/7/changedepartment?department=abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := doRequest(t, handler, http.MethodPost, tc.path, nil)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestActivateAndUpdateBindsBody(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodPost, "/api/v1/sites/1/users/7/activateAndUpdate",
		strings.NewReader(`{"FirstName":"Alice","LastName":"Smith","AccessLevel":1}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "User activated") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}
}

func TestDeactivateAndUpdateBindsBody(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodPost, "/api/v1/sites/1/users/7/deactivateAndUpdate",
		strings.NewReader(`{"FirstName":"Alice","LastName":"Smith","AccessLevel":1}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "User deactivated") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}
}

func TestActivateAndUpdateRejectsInvalidBody(t *testing.T) {
	handler := newTestRoutes(t)
	for _, path := range []string{
		"/api/v1/sites/1/users/7/activateAndUpdate",
		"/api/v1/sites/1/users/7/deactivateAndUpdate",
	} {
		recorder := doRequest(t, handler, http.MethodPost, path,
			strings.NewReader(`{"FirstName":"Alice"}`))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status 400, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "missing required field") {
			t.Errorf("%s: unexpected body %q", path, recorder.Body.String())
		}
	}
}

// TestActivateDepartmentPartialFailure covers the branch where some users
// in the department fail to activate (user 998 fails on the stub).
func TestActivateDepartmentPartialFailure(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodPost, "/api/v1/sites/1/departments/Visitors/activate", nil)
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("expected status 206, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1 failed") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}
}

func TestUpdateEndpoints(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodGet, "/api/v1/update/now", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Update complete") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}

	recorder = doRequest(t, handler, http.MethodGet, "/api/v1/update/trigger", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Update triggered") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	handler := newTestRoutes(t)
	recorder := doRequest(t, handler, http.MethodGet, "/api/v1/doesnotexist", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Resource not found") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}

	recorder = doRequest(t, handler, http.MethodDelete, "/api/v1/sites/1/users/7", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Method not allowed") {
		t.Errorf("unexpected body %q", recorder.Body.String())
	}
}

// TestPlaceholderPictureForMissingImage checks that a user without a photo
// gets the embedded "photo needed" placeholder rather than an error.
func TestPlaceholderPictureForMissingImage(t *testing.T) {
	handler := newTestRoutes(t)
	// User 8 makes the stub image endpoint return 404.
	recorder := doRequest(t, handler, http.MethodGet, "/api/v1/sites/1/users/8/picture", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if recorder.Body.Len() == 0 {
		t.Error("expected non-empty placeholder image")
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("expected image/jpeg content type, got %q", got)
	}
}
