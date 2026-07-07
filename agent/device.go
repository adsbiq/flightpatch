package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The control-plane (phone-home) API. Devices live behind NAT, so the device
// always initiates: it registers once, then polls telemetry, and any commands
// ride back on the telemetry response (one round-trip, no inbound port needed).
const (
	pathRegister  = "/api/v2/feeder/device/register"
	pathTelemetry = "/api/v2/feeder/device/telemetry"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

// --- registration ---

type registerReq struct {
	DeviceID  string `json:"device_id,omitempty"` // empty on first run; server assigns
	OrgName   string `json:"org_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	Platform  string `json:"platform"`
	Version   string `json:"version"`
}

type registerResp struct {
	DeviceID string `json:"device_id"`
	Token    string `json:"token"`
	FeedAddr string `json:"feed_addr"`
}

// Register claims (or re-confirms) this device's identity and returns its token.
// Idempotent on device_id, so it is safe to call whenever the token is missing.
func Register(server string, req registerReq) (*registerResp, error) {
	var out registerResp
	if err := postJSON(server+pathRegister, "", req, &out); err != nil {
		return nil, err
	}
	if out.Token == "" || out.DeviceID == "" {
		return nil, fmt.Errorf("register: server returned no device_id/token")
	}
	return &out, nil
}

// --- telemetry (heartbeat + command pull) ---

type Command struct {
	ID   int64          `json:"id"`
	Cmd  string         `json:"cmd"`
	Args map[string]any `json:"args"`
}

type telemetryReq struct {
	DeviceID  string         `json:"device_id"`
	Version   string         `json:"version"`
	UptimeS   int64          `json:"uptime_s"`
	BytesFed  int64          `json:"bytes_fed"`
	MsgRate   float64        `json:"msg_rate"`
	Connected bool           `json:"connected"`
	Acked     []int64        `json:"acked,omitempty"` // command ids handled since last poll
	Stats     map[string]any `json:"stats,omitempty"`
}

type telemetryResp struct {
	Enabled       bool           `json:"enabled"`
	Config        map[string]any `json:"config"`
	Commands      []Command      `json:"commands"`
	LatestRelease string         `json:"latest_release"`
}

// Telemetry sends a heartbeat and returns the server's desired state + any
// pending commands. Auth is the per-device token in X-Device-Token.
func Telemetry(server, token string, req telemetryReq) (*telemetryResp, error) {
	var out telemetryResp
	if err := postJSON(server+pathTelemetry, token, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- helpers ---

func postJSON(url, token string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "adsbiq-feed-agent/"+Version)
	if token != "" {
		httpReq.Header.Set("X-Device-Token", token)
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s -> %d: %s", url, resp.StatusCode, bytes.TrimSpace(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
