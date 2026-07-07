# ADSBiq Airport — turn-key ADS-B feeder

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

## How it works
```
RTL-SDR dongle  ->  decoder (1090 MHz ADS-B, Beast)  ->  feed agent  ->  ADSBiq network
```
The **feed agent** in this repo reads the decoder's Beast output on `127.0.0.1:30005` and
forwards it to the ADSBiq aggregator at `feed.adsbiq.com:30004`, reconnecting automatically.
The installer bundles the dongle driver, the decoder, and this agent so it all "just works."

## Coverage, honestly
A dongle at your desk reliably catches everything **airborne over your field and on approach
or departure from all runways** (aircraft up high have clear line of sight). For **taxi and
ground** traffic across the whole field, put the little antenna in a window facing the ramp.

## Repository layout
| Path | What |
|---|---|
| `agent/` | The Go feed agent (Beast forwarder, auto-reconnect). Single static binary, no runtime deps. |
| `installer/` | Windows / Mac installer sources (driver + decoder + agent + service). |

## Build the agent (developers)
```bash
cd agent
go test ./...                                   # unit tests
go build -ldflags="-s -w" -o adsbiq-feed-agent  # native
GOOS=windows GOARCH=amd64 go build -o adsbiq-feed-agent.exe   # cross-compile
```
Run it against a local decoder:
```bash
./adsbiq-feed-agent --local 127.0.0.1:30005 --feed feed.adsbiq.com:30004
```

## Status
Early development. The feed agent is working and tested; the one-click installer (silent
driver + bundled decoder) is in progress.

## Links
- Live map: [adsbiq.com/airport](https://adsbiq.com/airport)
- Become a feeder: [adsbiq.com/guide](https://adsbiq.com/guide) · [getadsbiq.com](https://getadsbiq.com)

## License
MIT — see [LICENSE](LICENSE).
