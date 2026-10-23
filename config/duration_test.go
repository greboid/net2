package config

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDurationMarshalJSON(t *testing.T) {
	duration := Duration(90 * time.Minute)
	data, err := json.Marshal(&duration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `"1h30m0s"` {
		t.Errorf("expected quoted duration string, got %s", data)
	}
}

func TestDurationMarshalYAML(t *testing.T) {
	duration := Duration(90 * time.Minute)
	node, err := yaml.Marshal(&duration)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(node) != "1h30m0s\n" {
		t.Errorf("expected duration string, got %q", node)
	}
}

func TestDurationUnmarshalYAML(t *testing.T) {
	var duration Duration
	if err := yaml.Unmarshal([]byte("5m30s"), &duration); err != nil {
		t.Fatalf("expected duration to parse, got %v", err)
	}
	if time.Duration(duration) != 5*time.Minute+30*time.Second {
		t.Errorf("expected 5m30s, got %s", time.Duration(duration))
	}
	if err := yaml.Unmarshal([]byte("notaduration"), &duration); err == nil {
		t.Error("expected error for invalid duration, got nil")
	}
}

// TestLoadConfigDoorSequenceDurations checks the openable door sequence
// durations parse through the custom Duration unmarshaller.
func TestLoadConfigDoorSequenceDurations(t *testing.T) {
	path := writeConfig(t, `
clientid: abc
sites:
  - id: 1
    name: SiteA
    ip: 1.2.3.4
    username: u
    password: p
    localIDField: Field1
    openableDoors:
      - name: Garage
        sequence:
          - id: 8
            duration: 2s
          - id: 9
            duration: 500ms
`)
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}
	sequence := config.Sites[0].OpenableDoors[0].Sequence
	if len(sequence) != 2 {
		t.Fatalf("expected 2 sequence steps, got %d", len(sequence))
	}
	if time.Duration(sequence[0].Duration) != 2*time.Second {
		t.Errorf("expected 2s first step, got %s", time.Duration(sequence[0].Duration))
	}
	if time.Duration(sequence[1].Duration) != 500*time.Millisecond {
		t.Errorf("expected 500ms second step, got %s", time.Duration(sequence[1].Duration))
	}
}
