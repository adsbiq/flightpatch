# Building the Flightpatch installer

The installer (`flightpatch.iss`, Inno Setup 6) assembles a single
`flightpatch-setup-<ver>.exe` that installs the driver, the agent, the
decoders, and a background service. It expects its inputs under `installer/dist/`.

## Layout the compiler expects

```
installer/
  flightpatch.iss
  dist/
    adsbiq-feed-agent.exe          # the Go agent (GOOS=windows GOARCH=amd64)
    decoders/
      dumpvdl2.exe + *.dll         # VDL2 decoder bundle (CI: dumpvdl2-windows-amd64)
      dump1090.exe + *.dll         # ADS-B decoder bundle (CI: dump1090-windows-amd64)
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
| `decoders/dump1090.exe` + DLLs | GitHub Actions **build-decoders → dump1090-windows-amd64** artifact (unzip into `decoders/`) |
| `driver/wdi-simple.exe` | GitHub Actions **build-decoders → wdi-simple-windows-amd64** artifact (single self-contained exe, WinUSB driver embedded — no DLLs) |
| `service/WinSW.exe` | [WinSW v3](https://github.com/winsw/winsw/releases) `WinSW-x64.exe`, renamed `WinSW.exe` (MIT) |

The release workflow builds and bundles both decoders.

## Compile

```
cd installer
iscc flightpatch.iss      # -> installer/out/flightpatch-setup-<ver>.exe
```

## What the installed product does

1. `wdi-simple.exe --vid 0x0BDA --pid 0x2838 --type 0 --silent` binds WinUSB to the
   RTL2832U dongle silently (no Zadig UI).
2. Files land in `%ProgramFiles%\ADSBiq`; the agent's config/state live in
   `%ProgramData%\ADSBiq`.
3. WinSW registers `adsbiq-agent` as an auto-start service running the agent at
   **idle priority** with `--decoders <app>\decoders` and the optional `--org`
   name the user typed. The agent then registers the device, enumerates the
   dongle(s), uses the role chosen in Setup (or auto-detect), and feeds.
4. Uninstall stops + removes the service.

## Still to build

- **Signing** — apply for [SignPath Foundation](https://signpath.org/) (free OSS
  code signing) so the setup .exe and service aren't flagged by SmartScreen.
