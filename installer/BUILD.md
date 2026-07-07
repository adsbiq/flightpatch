# Building the ADSBiq Airport installer

The installer (`adsbiq-airport.iss`, Inno Setup 6) assembles a single
`adsbiq-airport-setup-<ver>.exe` that installs the driver, the agent, the
decoders, and a background service. It expects its inputs under `installer/dist/`.

## Layout the compiler expects

```
installer/
  adsbiq-airport.iss
  dist/
    adsbiq-feed-agent.exe          # the Go agent (GOOS=windows GOARCH=amd64)
    decoders/
      dumpvdl2.exe + *.dll         # VDL2 decoder bundle (CI: dumpvdl2-windows-amd64)
      dump1090.exe + *.dll         # ADS-B decoder (TODO — see below)
    service/
      WinSW.exe                    # service host (see below)
    driver/
      wdi-simple.exe + *.dll       # silent WinUSB installer (CI: wdi-simple-windows-amd64)
```

## Where each input comes from

| Input | Source |
|---|---|
| `adsbiq-feed-agent.exe` | `cd agent && GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o adsbiq-feed-agent.exe` |
| `decoders/dumpvdl2.exe` + DLLs | GitHub Actions **build-decoders → dumpvdl2-windows-amd64** artifact (unzip into `decoders/`) |
| `driver/wdi-simple.exe` + DLLs | GitHub Actions **build-decoders → wdi-simple-windows-amd64** artifact |
| `service/WinSW.exe` | [WinSW v3](https://github.com/winsw/winsw/releases) `WinSW-x64.exe`, renamed `WinSW.exe` (MIT) |
| `decoders/dump1090.exe` | **TODO** — not yet built (see below) |

The agent runs whatever decoders are present: ship VDL2-only today and it feeds
VDL2; the ADS-B decoder is picked up automatically once added to `decoders/`.

## Compile

```
cd installer
iscc adsbiq-airport.iss      # -> installer/out/adsbiq-airport-setup-<ver>.exe
```

## What the installed product does

1. `wdi-simple.exe --vid 0x0BDA --pid 0x2838 --type 0 -b` binds WinUSB to the
   RTL2832U dongle silently (no Zadig UI).
2. Files land in `%ProgramFiles%\ADSBiq`; the agent's config/state live in
   `%ProgramData%\ADSBiq`.
3. WinSW registers `adsbiq-agent` as an auto-start service running the agent at
   **idle priority** with `--decoders <app>\decoders` and the optional `--org`
   name the user typed. The agent then registers the device, enumerates the
   dongle(s), auto-assigns a role (ADS-B vs VDL2), and feeds.
4. Uninstall stops + removes the service.

## Still to build

- **`dump1090.exe`** — ADS-B (1090) decoder for Windows. Same MSYS2/CI approach
  as dumpvdl2 (likely needs a small Windows port); until then the installer is
  VDL2-capable and ADS-B-ready (drop the exe into `decoders/`).
- **Signing** — apply for [SignPath Foundation](https://signpath.org/) (free OSS
  code signing) so the setup .exe and service aren't flagged by SmartScreen.
