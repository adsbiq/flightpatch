//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// configureChild puts each decoder in its own process group so the whole tree
// can be signalled/killed together.
func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// niceChild lowers the decoder's scheduling priority (best effort) so a feeder
// never starves the host.
func niceChild(pid int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, pid, 19)
}

// assignChildToJob is a no-op off Windows — Setpgid (above) already lets the
// process group be killed together.
func assignChildToJob(cmd *exec.Cmd) { _ = cmd }
