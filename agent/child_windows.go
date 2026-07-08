//go:build windows

package main

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
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

// --- Job Object: kill decoder children when the agent dies ------------------
// A stopped/killed agent (or a WinSW `service stop`) must NOT leave orphaned
// decoder processes holding the dongle — we hit exactly that: a SYSTEM-owned
// dump1090 survived a service stop and made the dongle vanish from enumeration.
// Every decoder child is assigned to a Job Object with KILL_ON_JOB_CLOSE, so
// when the agent process exits the job handle closes and Windows terminates the
// whole decoder tree.
var (
	kernel32Job                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObject          = kernel32Job.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32Job.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32Job.NewProc("AssignProcessToJobObject")

	jobOnce sync.Once
	jobH    uintptr
)

const (
	jobObjectExtendedLimitInformation = 9
	jobLimitKillOnJobClose            = 0x00002000
	procTerminate                     = 0x0001
	procSetQuota                      = 0x0100
)

type jobBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobIoCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount    uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount       uint64
}

type jobExtendedLimitInformation struct {
	BasicLimitInformation jobBasicLimitInformation
	IoInfo                jobIoCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func ensureJob() {
	jobOnce.Do(func() {
		h, _, _ := procCreateJobObject.Call(0, 0)
		if h == 0 {
			return
		}
		var info jobExtendedLimitInformation
		info.BasicLimitInformation.LimitFlags = jobLimitKillOnJobClose
		procSetInformationJobObject.Call(h, jobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
		jobH = h // kept open for the agent's lifetime -> kill-on-close
	})
}

// assignChildToJob puts a just-started child in the kill-on-close job so it dies
// with the agent. Best-effort (a tiny race before assignment is acceptable).
func assignChildToJob(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	ensureJob()
	if jobH == 0 {
		return
	}
	h, err := syscall.OpenProcess(procTerminate|procSetQuota, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	procAssignProcessToJobObject.Call(jobH, uintptr(h))
}
