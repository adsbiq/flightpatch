// ADSBiq feed agent — the device core.
//
// One binary that: registers this machine once (optionally under a school/FBO
// name), enumerates the attached RTL-SDR dongle(s), auto-assigns each a role
// (ADS-B 1090 or VDL2 136 MHz), and supervises the matching decoder — forwarding
// ADS-B Beast to the network and letting the VDL2 decoder feed directly. It
// phones home every 60s for state + commands (enable/disable, restart, update).
// Devices sit behind NAT, so the agent always dials out; nothing needs an
// inbound port.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Version is stamped into telemetry and drives auto-update comparisons.
const Version = "0.4.0"

func platformString() string { return runtime.GOOS + "/" + runtime.GOARCH }

func main() {
	var (
		cfgPath    = flag.String("config", "", "config file path (default: per-OS ProgramData/etc)")
		org        = flag.String("org", "", "school / FBO / organization name (optional, first run)")
		email      = flag.String("email", "", "owner email (optional, first run)")
		server     = flag.String("server", "", "control-plane base URL (default https://adsbiq.com)")
		local      = flag.String("local", "", "local decoder Beast host:port (default 127.0.0.1:30005)")
		decoderDir = flag.String("decoders", "", "directory holding the decoder binaries (default: agent dir)")
		once       = flag.Bool("register-only", false, "register, print identity, and exit")
		probeOnly  = flag.Bool("probe-only", false, "enumerate dongles, probe each role, print, and exit (no register/feed)")
		eepromDump = flag.Bool("eeprom-dump", false, "read + back up + print dongle 0's EEPROM, then exit (read-only diagnostic)")
		watch      = flag.Bool("watch", false, "log dongle enumeration changes every 2s (hot-plug demo; no register/feed)")
		eepromTest = flag.Int("eeprom-selftest", -1, "reversible EEPROM write test on the given dongle index, then exit")
		setSerial  = flag.String("set-serial", "", "diagnostic/repair: write a dongle serial, e.g. --set-serial 1=ADSBIQ001, then exit")
		dedupe     = flag.Bool("dedupe", false, "diagnostic: enumerate, de-duplicate colliding serials, print, and exit")
		resetUSB   = flag.Bool("reset-usb", false, "diagnostic: cycle RTL-SDR USB devices (needs elevation) so EEPROM serial writes take effect, then exit")
		jobTest    = flag.Bool("job-test", false, "diagnostic: spawn a child in the kill-on-close job then exit; the child must die with us (no orphans)")
	)
	flag.Parse()

	cfg := LoadConfig(*cfgPath)
	if *server != "" {
		cfg.Server = *server
	}
	if *local != "" {
		cfg.LocalBeast = *local
	}
	if *decoderDir != "" {
		cfg.DecoderDir = *decoderDir
	}
	if *org != "" {
		cfg.OrgName = *org
	}
	if *email != "" {
		cfg.UserEmail = *email
	}

	if *eepromDump {
		if err := dumpEEPROM(cfg.DecoderDir); err != nil {
			log.Fatalf("eeprom dump: %v", err)
		}
		return
	}

	if *eepromTest >= 0 {
		if err := eepromSelfTest(cfg.DecoderDir, *eepromTest); err != nil {
			log.Fatalf("eeprom selftest: %v", err)
		}
		return
	}

	if *setSerial != "" {
		parts := strings.SplitN(*setSerial, "=", 2)
		if len(parts) != 2 {
			log.Fatalf("--set-serial expects idx=serial (e.g. 1=ADSBIQ001)")
		}
		idx, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			log.Fatalf("--set-serial bad index: %v", err)
		}
		if err := writeDongleSerial(cfg.DecoderDir, idx, strings.TrimSpace(parts[1])); err != nil {
			log.Fatalf("set-serial: %v", err)
		}
		log.Printf("dongle #%d serial set to %q", idx, strings.TrimSpace(parts[1]))
		return
	}

	if *jobTest {
		cmd := exec.Command("ping", "-n", "60", "127.0.0.1") // ~60s child
		configureChild(cmd)
		if err := cmd.Start(); err != nil {
			log.Fatalf("job-test: start child: %v", err)
		}
		assignChildToJob(cmd)
		log.Printf("job-test: spawned child pid %d in kill-on-close job; exiting now — it must die with us", cmd.Process.Pid)
		return
	}

	if *resetUSB {
		log.Printf("cycling RTL-SDR USB devices...")
		resetRtlDevices()
		for _, d := range enumerateDongles(cfg.DecoderDir) {
			log.Printf("after reset: #%d serial=%q %q", d.Index, d.Serial, d.Product)
		}
		return
	}

	if *dedupe {
		mgr := newDecoderManager(cfg, &Stats{start: time.Now()})
		for _, d := range enumerateDongles(cfg.DecoderDir) {
			log.Printf("before: #%d serial=%q %q", d.Index, d.Serial, d.Product)
		}
		for _, d := range mgr.dedupeSerials(enumerateDongles(cfg.DecoderDir)) {
			log.Printf("after:  #%d serial=%q %q", d.Index, d.Serial, d.Product)
		}
		return
	}

	if *watch {
		log.Printf("watch: polling every 2s; plug/unplug dongles to see changes (kill to stop)")
		prev := ""
		for {
			ds := enumerateDongles(cfg.DecoderDir)
			cur := ""
			for _, d := range ds {
				cur += fmt.Sprintf("[#%d serial=%q port=%q %q] ", d.Index, d.Serial, d.Port, d.Product)
			}
			if cur != prev {
				log.Printf("dongles(%d): %s", len(ds), cur)
				prev = cur
			}
			time.Sleep(2 * time.Second)
		}
	}

	// Diagnostic: enumerate + probe roles against real hardware, then exit.
	// Never registers or feeds — safe to run any time to see what the agent sees.
	if *probeOnly {
		dongles := enumerateDongles(cfg.DecoderDir)
		log.Printf("probe-only: enumerated %d dongle(s); decoders in %s", len(dongles), cfg.DecoderDir)
		mgr := newDecoderManager(cfg, &Stats{start: time.Now()})
		for _, d := range dongles {
			log.Printf("dongle #%d serial=%q product=%q", d.Index, d.Serial, d.Product)
			role := mgr.probeRole(context.Background(), d)
			log.Printf("  => role=%s", role)
		}
		return
	}

	// Register once (or whenever the token is missing).
	if cfg.Token == "" {
		if err := cfg.EnsureDeviceIdentity(); err != nil {
			log.Fatalf("cannot establish durable device identity: %v", err)
		}
		log.Printf("registering device with %s ...", cfg.Server)
		r, err := Register(cfg.Server, registerReq{
			DeviceID: cfg.DeviceID, OrgName: cfg.OrgName, UserEmail: cfg.UserEmail,
			Platform: platformString(), Version: Version,
		})
		if err != nil {
			log.Fatalf("registration failed: %v", err)
		}
		cfg.DeviceID, cfg.Token = r.DeviceID, r.Token
		if r.FeedAddr != "" {
			cfg.Feed = r.FeedAddr
		}
		if err := cfg.Save(); err != nil {
			log.Fatalf("cannot save config to %s: %v", cfg.path, err)
		}
		log.Printf("registered as device %s", cfg.DeviceID)
	}
	if *once {
		log.Printf("device_id=%s feed=%s vdl2=%s org=%q", cfg.DeviceID, cfg.Feed, cfg.VDL2Feed, cfg.OrgName)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stats := &Stats{start: time.Now()}
	mgr := newDecoderManager(cfg, stats)
	mgr.setEnabled(true) // feed by default until the server says otherwise
	go mgr.run(ctx)

	log.Printf("adsbiq feed agent %s (device %s); decoders in %s", Version, cfg.DeviceID, cfg.DecoderDir)
	phoneHome(ctx, cfg, stats, mgr, stop)
	log.Printf("stopped")
}

// phoneHome sends telemetry every 60s, applies the returned desired state, and
// executes any queued commands, acking them on the next beat.
func phoneHome(ctx context.Context, cfg *DeviceConfig, stats *Stats, mgr *DecoderManager, stop context.CancelFunc) {
	var acked []int64
	var prevBytes int64
	var prevAt time.Time
	beat := func() {
		now := time.Now()
		bytes, connected, uptime, rate := stats.snapshot(prevBytes, prevAt, now)
		prevBytes, prevAt = bytes, now
		resp, err := Telemetry(cfg.Server, cfg.Token, telemetryReq{
			DeviceID: cfg.DeviceID, Version: Version, UptimeS: uptime,
			BytesFed: bytes, MsgRate: rate, Connected: connected, Acked: acked,
			Stats: map[string]any{
				"platform": platformString(),
				"org":      cfg.OrgName,
				"decoders": mgr.status(),
			},
		})
		acked = acked[:0]
		if err != nil {
			log.Printf("telemetry: %v", err)
			return
		}
		mgr.setEnabled(resp.Enabled)
		applyConfig(cfg, mgr, resp.Config)
		for _, c := range resp.Commands {
			handleCommand(cfg, mgr, c, stop)
			acked = append(acked, c.ID)
		}
	}

	beat() // report immediately on startup
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			beat()
		}
	}
}

// applyConfig honours server-pushed settings the agent can change live: feed
// endpoints and explicit per-dongle role overrides.
func applyConfig(cfg *DeviceConfig, mgr *DecoderManager, m map[string]any) {
	if m == nil {
		return
	}
	changed := false
	if v, ok := m["feed"].(string); ok && v != "" && v != cfg.Feed {
		cfg.Feed = v
		changed = true
	}
	if v, ok := m["vdl2_feed"].(string); ok && v != "" && v != cfg.VDL2Feed {
		cfg.VDL2Feed = v
		changed = true
	}
	if v, ok := m["gain"].(string); ok && v != "" && v != cfg.Gain {
		cfg.Gain = v
		changed = true
	}
	// {"roles": {"<serial>": "adsb|vdl2|off"}} — server can force a role.
	if roles, ok := m["roles"].(map[string]any); ok {
		for serial, r := range roles {
			if rs, ok := r.(string); ok && cfg.setRole(serial, rs) {
				changed = true
			}
		}
	}
	if changed {
		_ = cfg.Save()
		// bounce decoders so new settings/roles take effect
		mgr.setEnabled(false)
		time.Sleep(1500 * time.Millisecond)
		mgr.setEnabled(true)
		log.Printf("config updated (feed=%s vdl2=%s)", cfg.Feed, cfg.VDL2Feed)
	}
}

// handleCommand runs a one-shot server command.
func handleCommand(cfg *DeviceConfig, mgr *DecoderManager, c Command, stop context.CancelFunc) {
	log.Printf("command #%d: %s %v", c.ID, c.Cmd, c.Args)
	switch c.Cmd {
	case "enable":
		mgr.setEnabled(true)
	case "disable":
		mgr.setEnabled(false)
	case "restart":
		log.Printf("restart requested; exiting for the service to relaunch")
		stop()
	case "update":
		tag, url, err := checkUpdate(Version)
		if err != nil {
			log.Printf("update check failed: %v", err)
			return
		}
		if url == "" {
			log.Printf("already up to date (%s)", Version)
			return
		}
		log.Printf("updating to %s ...", tag)
		if err := selfUpdate(url); err != nil {
			log.Printf("self-update failed: %v", err)
			return
		}
		log.Printf("update staged; exiting for the service to relaunch into %s", tag)
		stop()
	default:
		log.Printf("unknown command %q, ignoring", c.Cmd)
	}
}
