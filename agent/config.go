package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DeviceConfig is the agent's persisted identity + settings. It survives restarts
// so a device registers exactly once and keeps its server-issued token.
type DeviceConfig struct {
	DeviceID   string `json:"device_id"`            // locally generated, server-confirmed, stable
	Token      string `json:"token"`               // per-device auth (X-Device-Token)
	OrgName    string `json:"org_name,omitempty"`  // optional school/FBO name (first-run)
	UserEmail  string `json:"user_email,omitempty"`// optional owner email
	Server     string `json:"server"`              // https://adsbiq.com (control plane)
	Feed       string `json:"feed"`                // feed.adsbiq.com:30004 (ADS-B Beast)
	LocalBeast string `json:"local_beast"`         // 127.0.0.1:30005 (decoder Beast out)

	// Decoder / dongle management (the installer bundles the decoders next to the
	// agent; the agent enumerates dongles, assigns a role to each, and supervises
	// the matching decoder).
	DecoderDir string       `json:"decoder_dir,omitempty"` // dir holding the decoder exes (default: <exe dir>)
	VDL2Feed   string       `json:"vdl2_feed,omitempty"`   // feed.adsbiq.com:5552 (VDL2 UDP)
	Gain       string       `json:"gain,omitempty"`        // rtl tuner gain (default "40")
	VDL2Freqs  []string     `json:"vdl2_freqs,omitempty"`  // VDL2 channels in Hz
	Roles      []DongleRole `json:"roles,omitempty"`       // persisted per-serial role assignments

	path string `json:"-"`
}

// EnsureDeviceIdentity creates and persists an installation identity before
// the first network request. A timeout after server registration can therefore
// be retried idempotently instead of creating another server-side device row.
func (c *DeviceConfig) EnsureDeviceIdentity() error {
	if c.DeviceID != "" {
		return nil
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generate device identity: %w", err)
	}
	c.DeviceID = "dev-" + hex.EncodeToString(raw)
	if err := c.Save(); err != nil {
		c.DeviceID = ""
		return fmt.Errorf("persist device identity: %w", err)
	}
	return nil
}

// DongleRole pins a dongle (by USB serial) to a decoder role so the agent does
// not have to re-probe on every start, and so the server can override a role.
type DongleRole struct {
	Serial string `json:"serial"`
	Role   string `json:"role"` // "adsb" | "vdl2" | "off"
}

// defaultConfigPath is a per-OS location a service can write to.
func defaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "ADSBiq", "agent.json")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "ADSBiq", "agent.json")
	default: // linux and friends
		if _, err := os.Stat("/var/lib"); err == nil {
			return "/var/lib/adsbiq/agent.json"
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "adsbiq", "agent.json")
	}
}

// LoadConfig reads config from path (creating an empty one in memory if absent),
// fills in defaults, and remembers the path for Save.
func LoadConfig(path string) *DeviceConfig {
	if path == "" {
		path = defaultConfigPath()
	}
	c := &DeviceConfig{path: path}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, c)
	}
	c.path = path
	if c.Server == "" {
		c.Server = "https://adsbiq.com"
	}
	if c.Feed == "" {
		c.Feed = "feed.adsbiq.com:30004"
	}
	if c.LocalBeast == "" {
		c.LocalBeast = "127.0.0.1:30005"
	}
	if c.VDL2Feed == "" {
		c.VDL2Feed = "feed.adsbiq.com:5552"
	}
	if c.Gain == "" {
		c.Gain = "40"
	}
	if len(c.VDL2Freqs) == 0 {
		// Common VDL2 channels (Hz), within one 2.1 Msps window.
		c.VDL2Freqs = []string{
			"136650000", "136700000", "136725000", "136750000", "136775000",
			"136800000", "136825000", "136875000", "136975000",
		}
	}
	if c.DecoderDir == "" {
		if exe, err := os.Executable(); err == nil {
			c.DecoderDir = filepath.Dir(exe)
		}
	}
	return c
}

// roleFor returns the persisted role for a dongle serial, or "" if unassigned.
func (c *DeviceConfig) roleFor(serial string) string {
	for _, r := range c.Roles {
		if r.Serial == serial {
			return r.Role
		}
	}
	return ""
}

// setRole records (or updates) a dongle's role and returns whether it changed.
func (c *DeviceConfig) setRole(serial, role string) bool {
	for i := range c.Roles {
		if c.Roles[i].Serial == serial {
			if c.Roles[i].Role == role {
				return false
			}
			c.Roles[i].Role = role
			return true
		}
	}
	c.Roles = append(c.Roles, DongleRole{Serial: serial, Role: role})
	return true
}

// Save writes the config atomically (temp + rename) so a crash mid-write can't
// corrupt the device identity.
func (c *DeviceConfig) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
