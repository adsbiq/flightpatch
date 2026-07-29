package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitDefaultRoleSkipsProbe(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{adsbExeName(), vdl2ExeName()} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := LoadConfig(filepath.Join(t.TempDir(), "agent.json"))
	cfg.DecoderDir = dir
	cfg.DefaultRole = RoleADSB
	mgr := newDecoderManager(cfg, newStats())

	got := mgr.assignRoles(context.Background(), []Dongle{{Index: 0, Serial: "radio", Port: "usb-1"}})
	if len(got) != 1 || got[0].Role != RoleADSB {
		t.Fatalf("assignment = %#v, want one ADS-B role", got)
	}
}
