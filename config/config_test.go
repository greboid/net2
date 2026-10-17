package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("unable to write config: %v", err)
	}
	return path
}

func TestLoadConfigRejectsMissingSiteID(t *testing.T) {
	path := writeConfig(t, `
clientid: abc
sites:
  - name: SiteA
    ip: 1.2.3.4
    username: u
    password: p
    localIDField: Field1
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for site with missing id, got nil")
	}
}

func TestLoadConfigRejectsNegativeSiteID(t *testing.T) {
	path := writeConfig(t, `
clientid: abc
sites:
  - id: -1
    name: SiteA
    ip: 1.2.3.4
    username: u
    password: p
    localIDField: Field1
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for site with negative id, got nil")
	}
}

func TestLoadConfigAcceptsValidSite(t *testing.T) {
	path := writeConfig(t, `
clientid: abc
sites:
  - id: 7
    name: SiteA
    ip: 1.2.3.4
    username: u
    password: p
    localIDField: Field1
`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	if config.Sites[0].ID != 7 {
		t.Errorf("expected site id 7, got %d", config.Sites[0].ID)
	}
	if config.Sites[0].Port != 8080 {
		t.Errorf("expected default port 8080, got %d", config.Sites[0].Port)
	}
}

// The config key for enabling HTTPS is (confusingly) `http`; plain HTTP
// is the default.  Make sure the documented key keeps parsing as expected.
func TestLoadConfigHTTPOptIn(t *testing.T) {
	path := writeConfig(t, `
clientid: abc
sites:
  - id: 1
    name: SiteA
    ip: 1.2.3.4
    http: true
    username: u
    password: p
    localIDField: Field1
`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	if !config.Sites[0].Https {
		t.Error("expected http: true to enable https")
	}
}
