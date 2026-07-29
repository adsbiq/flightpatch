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

func TestDefaultRoleChoiceReplacesOldAssignmentsOnlyWhenChanged(t *testing.T) {
	cfg := &DeviceConfig{
		DefaultRole: RoleADSB,
		Roles:       []DongleRole{{Serial: "port-1", Role: RoleADSB}},
	}
	applyDefaultRole(cfg, RoleADSB)
	if len(cfg.Roles) != 1 {
		t.Fatal("same install choice must preserve stable assignments")
	}
	applyDefaultRole(cfg, RoleVDL2)
	if cfg.DefaultRole != RoleVDL2 || len(cfg.Roles) != 0 {
		t.Fatal("changed install choice must replace old assignments")
	}
}

func TestLoadConfigDefaultsNewInstallToAuto(t *testing.T) {
	cfg := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if cfg.DefaultRole != "auto" {
		t.Fatalf("default role = %q, want auto", cfg.DefaultRole)
	}
}
