package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDeviceIdentityPersistsAndReusesID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	cfg := LoadConfig(path)
	if err := cfg.EnsureDeviceIdentity(); err != nil {
		t.Fatalf("EnsureDeviceIdentity: %v", err)
	}
	first := cfg.DeviceID
	if !strings.HasPrefix(first, "dev-") || len(first) != 28 {
		t.Fatalf("unexpected device ID %q", first)
	}

	reloaded := LoadConfig(path)
	if reloaded.DeviceID != first {
		t.Fatalf("identity not persisted: got %q, want %q", reloaded.DeviceID, first)
	}
	if err := reloaded.EnsureDeviceIdentity(); err != nil {
		t.Fatalf("second EnsureDeviceIdentity: %v", err)
	}
	if reloaded.DeviceID != first {
		t.Fatalf("identity changed: got %q, want %q", reloaded.DeviceID, first)
	}
}
