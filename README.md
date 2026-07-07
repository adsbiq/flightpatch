# ADSBiq Airport — turn-key ADS-B / VDL2 feeder

Put your airfield on the live map. Plug a ~$50 USB dongle into the **computer you already
have**, run one installer, and your airport goes live at
**[adsbiq.com/airport](https://adsbiq.com/airport)** — every takeoff, landing and pattern,
in real time. No Raspberry Pi, no config, no hassle.

Built for flight schools and FBOs, but anyone is welcome.

## What you need
- Any Windows 10/11 or Mac computer (the office desktop is perfect — it can run this in the background).
- A ~$50 RTL-SDR dongle + 1090 MHz antenna. A good, proven pick:
  [NooElec RTL-SDR v5 (R820T2)](https://www.amazon.com/NooElec-NESDR-Smart-Bundle-R820T2-Based/dp/B01GDN1T4S)
  or an [RTL-SDR Blog V4](https://www.rtl-sdr.com/). (Any R820T2 / R828D based receiver works;
  make sure it tunes 1090 MHz.)
- Power and Wi-Fi. Indoors is fine — the antenna in a window facing the ramp is even better.

## What you get, free
- A **live traffic board** for your field (great on a lobby TV).
- A **per-tail tracker** — watch your own aircraft live, with instructor alerts.
- A permanent **pilot logbook** with one-click **KML export to ForeFlight**.

## One or two dongles — the software figures it out
The agent is built for **multiple receivers on the same PC** and auto-detects each one's role,
so it "just works" whether you have one dongle or two:

| Dongle | Band | Feeds |
|---|---|---|
| ADS-B | **1090 MHz** | aircraft positions (the live map) |
| VDL2 / ACARS | **131–136 MHz** (VHF) | datalink messages (flight/route data) |

Run it with just a 1090 dongle and you feed ADS-B. Add a **second dongle on a 136 MHz antenna**
later — even after the first is already running — and the agent picks it up and starts feeding
VDL2 automatically. Have only a 136 MHz dongle? It runs as a VDL2-only feeder. Nothing to
configure either way. *(VDL2 support is landing now; the ADS-B path is live today.)*

## How it works
```
1090 dongle -> ADS-B decoder (Beast)   \
                                         >-- feed agent -- registers, feeds, phones home --> ADSBiq network
136  dongle -> VDL2 decoder  (JSON)     /
```
The **feed agent** in this repo registers your device once (optionally under your school/FBO
name), forwards the decoder output to the ADSBiq network, and phones home every 60s — so it
keeps itself **up to date automatically** and can be paused, resumed, or retuned remotely
without you touching the machine. It reconnects on its own after any drop, reboot, or outage.
Because it always dials **out**, nothing needs an open inbound port or a static IP.

## Downloads
Pre-built, single-file binaries for **Windows, macOS (Intel + Apple Silicon), and Linux** are
published on the [Releases](https://github.com/adsbiq/adsbiq-airport/releases) page. The
one-click installer bundles the dongle driver, the decoder(s), and this agent as a background
service so the whole thing installs and connects with no terminal.

## Privacy
The agent sends only what's needed to feed and stay healthy: your aircraft data, a byte/rate
heartbeat, version, and the optional name you chose. No personal files, no browsing, nothing
else. It's open source — read [`agent/`](agent/) and see for yourself. MIT licensed.

## Coverage, honestly
A dongle at your desk reliably catches everything **airborne over your field and on approach
or departure from all runways** (aircraft up high have clear line of sight). For **taxi and
ground** traffic across the whole field, put the little antenna in a window facing the ramp.

## Repository layout
| Path | What |
|---|---|
| `agent/` | The Go feed agent — registration, Beast forwarder, phone-home management, auto-update. Single static binary, no runtime deps. |
| `installer/` | Windows / Mac installer sources (driver + decoder(s) + agent + service). |

## Build the agent (developers)
```bash
cd agent
go test ./...                                   # unit tests
go build -ldflags="-s -w" -o adsbiq-feed-agent  # native
GOOS=windows GOARCH=amd64 go build -o adsbiq-feed-agent.exe   # cross-compile
```
Register a device and run it against a local decoder:
```bash
./adsbiq-feed-agent --org "My Flight School"    # first run: registers, saves identity
./adsbiq-feed-agent --local 127.0.0.1:30005     # subsequent runs: feed + phone home
```

## Status
The feed agent is working and tested end-to-end: registration, live feeding, and remote
management (enable/disable, restart, auto-update) all verified against the production network.
VDL2 (second-dongle) decoding and the one-click installer (silent driver + bundled decoders)
are in progress.

## Links
- Live map: [adsbiq.com/airport](https://adsbiq.com/airport)
- Become a feeder: [adsbiq.com/guide](https://adsbiq.com/guide) · [getadsbiq.com](https://getadsbiq.com)

## License
MIT — see [LICENSE](LICENSE).
