# Installing Flightpatch

Flightpatch turns any Windows PC plus a $20–50 USB SDR dongle into a live
**ADS-B** (and optional **VDL2**) feeder for the ADSBiq network. No Raspberry Pi,
no configuration.

## What you need
- A **Windows 10/11 PC (64-bit)** that can stay powered on.
- An **RTL-SDR dongle** — any RTL2832U dongle works. Tested: **RTL-SDR Blog V4**
  (~$41) and **NooElec NESDR SMArt** (~$54). Feeding two bands? Use two dongles
  (see below) — mixed or identical, both are fine.
- An **antenna** for the band you want:
  - **ADS-B (1090 MHz):** a short 1090 antenna (often bundled with the dongle).
  - **VDL2 (136 MHz):** a taller VHF/airband antenna (~54 cm quarter-wave).
- A spot with **sky view** — a window sill is ideal.

## Install (about a minute)
1. Download **flightpatch-setup.exe** from <https://flightpatch.app>.
2. **Plug your dongle in** when the installer asks — the driver only binds to a
   dongle that's connected during setup.
3. Run the installer. Optionally **name your airfield** (flight school / FBO) so
   your feeder shows up under it. Click through.
4. You're live — it opens **flightpatch.app/airport**, and your aircraft appear
   within a minute.

Flightpatch installs the USB driver, auto-tunes the radio by listening on your
antenna, and runs quietly in the background as a low-priority Windows service
that restarts itself and survives reboots.

## One dongle or two?
- **One dongle → ADS-B** (1090 MHz) — the common case.
- **Two dongles → ADS-B + VDL2.** Plug both in. Flightpatch listens on each and
  **auto-detects** which band its antenna is for (it confirms VDL2 by actually
  decoding a message). Two *identical* dongles are fine — Flightpatch tells them
  apart by their USB port, so nothing to configure.

VDL2 is the scarcer, higher-value data (OOOI times, position reports, weather),
so a second dongle on 136 MHz is especially welcome.

## Antenna placement — this matters most
Reception is line-of-sight. Best → worst: **outdoors up high > window with sky
view > near a window > inner room.** If you see few or no aircraft, **move the
antenna toward a window first** — placement matters far more than the dongle or
brand.

## Network / firewall
Flightpatch feeds outbound to:
- **ADS-B:** `feed.adsbiq.com:30004`
- **VDL2:** `feed.adsbiq.com:5552`

plus a small HTTPS heartbeat to `adsbiq.com`. On a corporate or school network,
if your feeder shows offline, ask IT to allow **outbound** to those ports.

## Troubleshooting
- **No aircraft on the map** → almost always antenna placement (see above), or
  simply no traffic overhead right now. Give it a few minutes near a window.
- **Feeder shows offline** → make sure the PC is on and the dongle is plugged in.
  A sleeping PC won't feed; the service itself restarts automatically.
- **"Windows protected your PC" / SmartScreen on download** → click **More info →
  Run anyway**. (Code signing is being finalized; once live this prompt goes away.)

## Uninstall
**Settings → Apps → Flightpatch → Uninstall.** This stops and removes the
background service and cleans up completely.
