// Decoder supervisor: enumerate RTL-SDR dongles, assign each a role (ADS-B 1090
// or VDL2 136 MHz), and run + supervise the matching decoder — restarting on
// crash, hot-plugging new dongles, and freeing them when feeding is paused.
//
//	1090 dongle -> dump1090 (Beast :30005) -> forwarder -> feed.adsbiq.com:30004
//	136  dongle -> dumpvdl2 (JSON/UDP)     ----------->    feed.adsbiq.com:5552
//
// Decoders are child processes started windowless + at Idle priority so a feeder
// never disturbs the machine it shares.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Roles a dongle can be assigned.
const (
	RoleADSB = "adsb"
	RoleVDL2 = "vdl2"
	RoleOff  = "off"
)

// Dongle is one enumerated RTL-SDR device.
type Dongle struct {
	Index   int
	Serial  string
	Product string
}

type roleAssignment struct {
	Dongle Dongle
	Role   string
}

func adsbExeName() string {
	if runtime.GOOS == "windows" {
		return "dump1090.exe"
	}
	return "dump1090"
}

func vdl2ExeName() string {
	if runtime.GOOS == "windows" {
		return "dumpvdl2.exe"
	}
	return "dumpvdl2"
}

// exePath resolves a decoder binary: first next to the agent (DecoderDir), then
// on PATH. Returns ("", false) if not found.
func exePath(dir, name string) (string, bool) {
	if dir != "" {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	if lp, err := exec.LookPath(name); err == nil {
		return lp, true
	}
	return "", false
}

// DecoderManager owns the dongle->decoder lifecycle. Feeding runs exactly when
// enabled (server-controlled); flipping it stops/starts the whole decoder set.
type DecoderManager struct {
	cfg   *DeviceConfig
	stats *Stats

	mu      sync.Mutex
	enabled bool
	active  []string // "role@serial" of running decoders, for telemetry
}

func newDecoderManager(cfg *DeviceConfig, stats *Stats) *DecoderManager {
	return &DecoderManager{cfg: cfg, stats: stats}
}

func (m *DecoderManager) setEnabled(v bool) {
	m.mu.Lock()
	m.enabled = v
	m.mu.Unlock()
}

func (m *DecoderManager) isEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

func (m *DecoderManager) setActive(a []string) {
	m.mu.Lock()
	m.active = a
	m.mu.Unlock()
}

// status returns the roles currently being fed (for telemetry).
func (m *DecoderManager) status() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.active))
	copy(out, m.active)
	return out
}

// run reconciles desired vs actual every 10s until parent is cancelled: when
// enabled it enumerates dongles, assigns roles, and starts/stops decoders
// (hot-plug aware); when disabled it stops everything and frees the dongles.
func (m *DecoderManager) run(parent context.Context) {
	type child struct {
		cancel context.CancelFunc
		role   string
	}
	active := map[string]child{} // by dongle serial
	var fwdCancel context.CancelFunc

	stopAll := func() {
		for s, c := range active {
			c.cancel()
			delete(active, s)
		}
		if fwdCancel != nil {
			fwdCancel()
			fwdCancel = nil
		}
		m.setActive(nil)
	}

	reconcile := func() {
		if !m.isEnabled() {
			if len(active) > 0 || fwdCancel != nil {
				log.Printf("feeding disabled; stopping decoders")
				stopAll()
			}
			return
		}
		dongles := enumerateDongles(m.cfg.DecoderDir)
		if len(dongles) == 0 {
			// An in-use dongle can momentarily fail to enumerate; don't tear down
			// running decoders on a blip. A truly-gone dongle makes its decoder
			// process exit, which superviseDecoder handles. Only idle here.
			return
		}
		assigns := m.assignRoles(parent, dongles)
		seen := map[string]bool{}
		wantForward := false
		for _, a := range assigns {
			seen[a.Dongle.Serial] = true
			if a.Role == RoleADSB {
				wantForward = true
			}
			if a.Role == RoleOff {
				continue
			}
			if _, ok := active[a.Dongle.Serial]; ok {
				continue // already running
			}
			dctx, dcancel := context.WithCancel(parent)
			active[a.Dongle.Serial] = child{dcancel, a.Role}
			go m.superviseDecoder(dctx, a.Role, a.Dongle)
			log.Printf("decoder scheduled: role=%s dongle=#%d serial=%s", a.Role, a.Dongle.Index, a.Dongle.Serial)
		}
		// stop decoders whose dongle was unplugged
		for s, c := range active {
			if !seen[s] {
				log.Printf("dongle %s removed; stopping its decoder", s)
				c.cancel()
				delete(active, s)
			}
		}
		// forwarder runs whenever an ADS-B dongle is present
		if wantForward && fwdCancel == nil {
			fctx, fcancel := context.WithCancel(parent)
			fwdCancel = fcancel
			go func() {
				_ = runForward(fctx, Config{Local: m.cfg.LocalBeast, Feed: m.cfg.Feed,
					DialTimeout: 10 * time.Second, RetryDelay: 5 * time.Second}, m.stats)
			}()
			log.Printf("ADS-B forwarder started (%s -> %s)", m.cfg.LocalBeast, m.cfg.Feed)
		} else if !wantForward && fwdCancel != nil {
			fwdCancel()
			fwdCancel = nil
		}
		// publish status
		st := make([]string, 0, len(active))
		for s, c := range active {
			st = append(st, c.role+"@"+s)
		}
		m.setActive(st)
	}

	reconcile()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-parent.Done():
			stopAll()
			return
		case <-t.C:
			reconcile()
		}
	}
}

// assignRoles resolves a role for each dongle: persisted/server config first,
// then (if only one decoder is bundled) that decoder, then an ADS-B presence
// probe to tell 1090 antennas from VHF ones. Results are persisted so probing
// happens at most once per dongle.
func (m *DecoderManager) assignRoles(ctx context.Context, dongles []Dongle) []roleAssignment {
	_, adsbAvail := exePath(m.cfg.DecoderDir, adsbExeName())
	_, vdl2Avail := exePath(m.cfg.DecoderDir, vdl2ExeName())

	out := make([]roleAssignment, 0, len(dongles))
	for _, d := range dongles {
		role := m.cfg.roleFor(d.Serial)
		switch {
		case role != "":
			// keep configured role
		case adsbAvail && !vdl2Avail:
			role = RoleADSB
		case vdl2Avail && !adsbAvail:
			role = RoleVDL2
		case adsbAvail && vdl2Avail:
			if m.probeADSB(ctx, d) {
				role = RoleADSB
			} else {
				role = RoleVDL2
			}
		default:
			role = RoleOff // no decoders bundled
		}
		if role != RoleOff && m.cfg.setRole(d.Serial, role) {
			_ = m.cfg.Save()
		}
		out = append(out, roleAssignment{Dongle: d, Role: role})
	}
	return out
}

// probeADSB briefly runs the ADS-B decoder on a dedicated Beast port and checks
// whether any frames arrive — true means a 1090 antenna is attached.
func (m *DecoderManager) probeADSB(ctx context.Context, d Dongle) bool {
	exe, ok := exePath(m.cfg.DecoderDir, adsbExeName())
	if !ok {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	// Probe on a dedicated Beast port so it can't collide with a real decoder.
	const probePort = "31005"
	cmd := exec.CommandContext(pctx, exe, "--device-index", strconv.Itoa(d.Index),
		"--net", "--net-beast", "--net-bo-port", probePort)
	configureChild(cmd)
	if err := cmd.Start(); err != nil {
		log.Printf("probe: cannot start ADS-B decoder: %v", err)
		return false
	}
	defer func() { _ = cmd.Wait() }()
	time.Sleep(3 * time.Second) // let it bind + tune
	got := false
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+probePort, 3*time.Second); err == nil {
		_ = c.SetReadDeadline(time.Now().Add(9 * time.Second))
		buf := make([]byte, 64)
		if n, _ := c.Read(buf); n > 0 {
			got = true
		}
		c.Close()
	}
	cancel()
	log.Printf("probe dongle #%d (%s): ADS-B %s", d.Index, d.Serial, map[bool]string{true: "detected", false: "not detected"}[got])
	return got
}

// superviseDecoder runs the decoder for a role, restarting it (with backoff)
// until ctx is cancelled.
func (m *DecoderManager) superviseDecoder(ctx context.Context, role string, d Dongle) {
	backoff := 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		cmd, err := m.buildDecoderCmd(ctx, role, d)
		if err != nil {
			log.Printf("%s decoder unavailable for dongle #%d: %v (retry 30s)", role, d.Index, err)
			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
			continue
		}
		configureChild(cmd)
		start := time.Now()
		if err := cmd.Start(); err != nil {
			log.Printf("%s decoder failed to start: %v", role, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = minDur(backoff*2, 30*time.Second)
			continue
		}
		niceChild(cmd.Process.Pid)
		log.Printf("%s decoder running (pid %d, dongle #%d %s)", role, cmd.Process.Pid, d.Index, d.Serial)
		_ = cmd.Wait()
		if ctx.Err() != nil {
			return // intentional stop
		}
		if time.Since(start) > 60*time.Second {
			backoff = 2 * time.Second // ran healthy for a while; reset backoff
		}
		log.Printf("%s decoder exited; restarting in %s", role, backoff)
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = minDur(backoff*2, 30*time.Second)
	}
}

// buildDecoderCmd constructs the decoder command for a role/dongle.
func (m *DecoderManager) buildDecoderCmd(ctx context.Context, role string, d Dongle) (*exec.Cmd, error) {
	switch role {
	case RoleADSB:
		exe, ok := exePath(m.cfg.DecoderDir, adsbExeName())
		if !ok {
			return nil, fmt.Errorf("%s not found in %s", adsbExeName(), m.cfg.DecoderDir)
		}
		// dump1090 serves Beast on :30005; the forwarder relays it to the network.
		return exec.CommandContext(ctx, exe, "--device-index", strconv.Itoa(d.Index),
			"--net", "--net-beast", "--net-bo-port", "30005"), nil
	case RoleVDL2:
		exe, ok := exePath(m.cfg.DecoderDir, vdl2ExeName())
		if !ok {
			return nil, fmt.Errorf("%s not found in %s", vdl2ExeName(), m.cfg.DecoderDir)
		}
		host, port := splitHostPort(m.cfg.VDL2Feed)
		args := []string{"--rtlsdr", strconv.Itoa(d.Index), "--gain", m.cfg.Gain,
			"--output", fmt.Sprintf("decoded:json:udp:address=%s,port=%s", host, port)}
		args = append(args, m.cfg.VDL2Freqs...)
		return exec.CommandContext(ctx, exe, args...), nil
	default:
		return nil, fmt.Errorf("unknown role %q", role)
	}
}

func splitHostPort(hp string) (host, port string) {
	if h, p, err := net.SplitHostPort(hp); err == nil {
		return h, p
	}
	return hp, "5552"
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// cstr converts a NUL-terminated byte buffer to a Go string.
func cstr(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return strings.TrimRight(string(b), "\x00")
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
