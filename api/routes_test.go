package api

import (
	"strings"
	"testing"
)

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
