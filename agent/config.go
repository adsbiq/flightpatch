package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// DeviceConfig is the agent's persisted identity + settings. It survives restarts
// so a device registers exactly once and keeps its server-issued token.
type DeviceConfig struct {
	DeviceID   string `json:"device_id"`            // server-issued, stable
	Token      string `json:"token"`               // per-device auth (X-Device-Token)
	OrgName    string `json:"org_name,omitempty"`  // optional school/FBO name (first-run)
	UserEmail  string `json:"user_email,omitempty"`// optional owner email
	Server     string `json:"server"`              // https://adsbiq.com (control plane)
	Feed       string `json:"feed"`                // feed.adsbiq.com:30004 (ADS-B Beast)
	LocalBeast string `json:"local_beast"`         // 127.0.0.1:30005 (decoder Beast out)

	path string `json:"-"`
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
	return c
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
