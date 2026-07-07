// ADSBiq feed agent — the device core.
//
// One binary that: registers this machine once (optionally under a school/FBO
// name), forwards the local decoder's Beast stream to the adsbiq network, and
// phones home every 60s for state + commands (enable/disable, restart, update,
// decoder tuning). Devices sit behind NAT, so the agent always dials out; nothing
// needs an inbound port. Designed for N decoders — ADS-B (1090) today, VDL2 (136)
// slots in as a second managed source.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// Version is stamped into telemetry and drives auto-update comparisons.
const Version = "0.2.0"

func platformString() string { return runtime.GOOS + "/" + runtime.GOARCH }

func main() {
	var (
		cfgPath = flag.String("config", "", "config file path (default: per-OS ProgramData/etc)")
		org     = flag.String("org", "", "school / FBO / organization name (optional, first run)")
		email   = flag.String("email", "", "owner email (optional, first run)")
		server  = flag.String("server", "", "control-plane base URL (default https://adsbiq.com)")
		local   = flag.String("local", "", "local decoder Beast host:port (default 127.0.0.1:30005)")
		once    = flag.Bool("register-only", false, "register, print identity, and exit")
	)
	flag.Parse()

	cfg := LoadConfig(*cfgPath)
	if *server != "" {
		cfg.Server = *server
	}
	if *local != "" {
		cfg.LocalBeast = *local
	}
	if *org != "" {
		cfg.OrgName = *org
	}
	if *email != "" {
		cfg.UserEmail = *email
	}

	// Register once (or whenever the token is missing).
	if cfg.Token == "" {
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
		log.Printf("device_id=%s feed=%s org=%q", cfg.DeviceID, cfg.Feed, cfg.OrgName)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stats := &Stats{start: time.Now()}
	sup := newSupervisor(cfg, stats)
	sup.setEnabled(true) // feed by default until the server says otherwise
	go sup.run(ctx)

	log.Printf("adsbiq feed agent %s: %s -> %s (device %s)", Version, cfg.LocalBeast, cfg.Feed, cfg.DeviceID)
	phoneHome(ctx, cfg, stats, sup, stop)
	log.Printf("stopped")
}

// supervisor keeps the forwarder running exactly when enabled. Flipping enabled
// starts/stops the Beast forward without tearing down the whole agent, so the
// server can pause a misbehaving feeder and resume it, all via phone-home.
type supervisor struct {
	cfg     *DeviceConfig
	stats   *Stats
	mu      sync.Mutex
	enabled bool
	cancel  context.CancelFunc // cancels the running forwarder, if any
}

func newSupervisor(cfg *DeviceConfig, st *Stats) *supervisor {
	return &supervisor{cfg: cfg, stats: st}
}

func (s *supervisor) setEnabled(v bool) {
	s.mu.Lock()
	s.enabled = v
	s.mu.Unlock()
}

func (s *supervisor) isEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

// run reconciles desired (enabled) vs actual (forwarder goroutine) once a second
// until the parent ctx is cancelled.
func (s *supervisor) run(parent context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	running := false
	for {
		select {
		case <-parent.Done():
			s.mu.Lock()
			if s.cancel != nil {
				s.cancel()
			}
			s.mu.Unlock()
			return
		case <-t.C:
			want := s.isEnabled()
			if want && !running {
				fctx, fcancel := context.WithCancel(parent)
				s.mu.Lock()
				s.cancel = fcancel
				s.mu.Unlock()
				fc := Config{Local: s.cfg.LocalBeast, Feed: s.cfg.Feed, DialTimeout: 10 * time.Second, RetryDelay: 5 * time.Second}
				go func() { _ = runForward(fctx, fc, s.stats) }()
				running = true
				log.Printf("feed: enabled")
			} else if !want && running {
				s.mu.Lock()
				if s.cancel != nil {
					s.cancel()
					s.cancel = nil
				}
				s.mu.Unlock()
				running = false
				log.Printf("feed: disabled (paused by server)")
			}
		}
	}
}

// phoneHome sends telemetry every 60s, applies the returned desired state, and
// executes any queued commands, acking them on the next beat.
func phoneHome(ctx context.Context, cfg *DeviceConfig, stats *Stats, sup *supervisor, stop context.CancelFunc) {
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
			Stats: map[string]any{"platform": platformString(), "org": cfg.OrgName},
		})
		acked = acked[:0]
		if err != nil {
			log.Printf("telemetry: %v", err)
			return
		}
		sup.setEnabled(resp.Enabled)
		applyConfig(cfg, sup, resp.Config)
		for _, c := range resp.Commands {
			handleCommand(cfg, sup, c, stop)
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

// applyConfig honours server-pushed settings that the agent can change live.
func applyConfig(cfg *DeviceConfig, sup *supervisor, m map[string]any) {
	if m == nil {
		return
	}
	changed := false
	if v, ok := m["feed"].(string); ok && v != "" && v != cfg.Feed {
		cfg.Feed = v
		changed = true
	}
	if v, ok := m["local"].(string); ok && v != "" && v != cfg.LocalBeast {
		cfg.LocalBeast = v
		changed = true
	}
	if changed {
		_ = cfg.Save()
		// bounce the forwarder so new addrs take effect
		sup.setEnabled(false)
		time.Sleep(1200 * time.Millisecond)
		sup.setEnabled(true)
		log.Printf("config updated: feed=%s local=%s", cfg.Feed, cfg.LocalBeast)
	}
}

// handleCommand runs a one-shot server command. Decoder-tuning commands are
// acknowledged and logged now; they wire to the managed decoder in the next
// milestone (multi-decoder supervisor).
func handleCommand(cfg *DeviceConfig, sup *supervisor, c Command, stop context.CancelFunc) {
	log.Printf("command #%d: %s %v", c.ID, c.Cmd, c.Args)
	switch c.Cmd {
	case "enable":
		sup.setEnabled(true)
	case "disable":
		sup.setEnabled(false)
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
	case "set-gain", "biastee-on", "biastee-off", "pull-logs":
		log.Printf("decoder command %q noted (wired to managed decoder in next milestone)", c.Cmd)
	default:
		log.Printf("unknown command %q, ignoring", c.Cmd)
	}
}
