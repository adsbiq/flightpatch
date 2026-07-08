//go:build windows

package main

import (
	"log"
	"os/exec"
	"time"
)

// resetRtlDevices cycles all RTL-SDR (VID 0BDA:2838) USB devices so a freshly
// written EEPROM serial takes effect — the OS caches the USB descriptor until the
// device re-enumerates. Disable+Enable is the reliable equivalent of a replug.
// Requires elevation (install-time, or the SYSTEM service). Best-effort: logs and
// returns; the caller re-enumerates afterward.
func resetRtlDevices() {
	const ps = `Get-PnpDevice -InstanceId 'USB\VID_0BDA&PID_2838*' -EA SilentlyContinue | ` +
		`Disable-PnpDevice -Confirm:$false -EA SilentlyContinue; Start-Sleep -Milliseconds 800; ` +
		`Get-PnpDevice -InstanceId 'USB\VID_0BDA&PID_2838*' -EA SilentlyContinue | ` +
		`Enable-PnpDevice -Confirm:$false -EA SilentlyContinue`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	configureChild(cmd)
	if err := cmd.Run(); err != nil {
		log.Printf("resetRtlDevices: %v (needs elevation)", err)
	}
	time.Sleep(3 * time.Second) // let devices re-enumerate + WinUSB re-bind
}
