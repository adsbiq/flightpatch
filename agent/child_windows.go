//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow    = 0x08000000 // CREATE_NO_WINDOW  — no console window (no focus steal)
	idlePriorityClass = 0x00000040 // IDLE_PRIORITY_CLASS — the true "nice -19"
)

// configureChild launches a decoder windowless and at Idle priority so it never
// pops a console or starves the machine it shares.
func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow | idlePriorityClass,
		HideWindow:    true,
	}
}

// niceChild is a no-op on Windows: priority is set via the creation flags above.
func niceChild(pid int) { _ = pid }
