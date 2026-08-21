//go:build windows

package main

import (
	"log"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

// librtlsdr is loaded exactly ONCE and its procs cached — the supervisor
// enumerates on a cadence, and re-LoadDLL'ing every scan leaked a module
// reference each call (millions/year) for no benefit.
var (
	rtlOnce       sync.Once
	rtlDLL        *syscall.DLL // cached handle so EEPROM ops can FindProc too
	rtlGetCount   *syscall.Proc
	rtlGetStr     *syscall.Proc
	dongleCacheMu sync.Mutex
	dongleCache   = make(map[string]Dongle) // USB port -> last readable identity
)

func initRtlsdr(decoderDir string) {
	rtlOnce.Do(func() {
		if decoderDir != "" {
			setDllDirectory(decoderDir)
		}
		dll := loadRtlsdr(decoderDir)
		if dll == nil {
			return
		}
		gc, err1 := dll.FindProc("rtlsdr_get_device_count")
		gs, err2 := dll.FindProc("rtlsdr_get_device_usb_strings")
		if err1 != nil || err2 != nil {
			log.Printf("dongles: librtlsdr missing expected procs")
			return
		}
		rtlDLL, rtlGetCount, rtlGetStr = dll, gc, gs
	})
}

// enumerateDongles lists RTL-SDR devices via librtlsdr. The DLL and its
// dependencies (libusb, winpthread) ship next to the agent, so we point the
// loader at decoderDir first (once).
func enumerateDongles(decoderDir string) []Dongle {
	initRtlsdr(decoderDir)
	getCount, getStrings := rtlGetCount, rtlGetStr
	if getCount == nil || getStrings == nil {
		return nil
	}

	n, _, _ := getCount.Call()
	// Reading USB strings can fail while dump1090 has a device open.  Port
	// enumeration does not claim the device, so use the stable physical port to
	// retain its last readable identity instead of falsely reporting it removed.
	ports := donglePorts(decoderDir)
	dongleCacheMu.Lock()
	defer dongleCacheMu.Unlock()
	var out []Dongle
	for i := uintptr(0); i < n; i++ {
		port := ""
		if int(i) < len(ports) {
			port = ports[i]
		}
		var manu, prod, serial [256]byte
		r, _, _ := getStrings.Call(i,
			uintptr(unsafe.Pointer(&manu[0])),
			uintptr(unsafe.Pointer(&prod[0])),
			uintptr(unsafe.Pointer(&serial[0])))
		if int32(r) != 0 {
			if cached, ok := dongleCache[port]; ok && port != "" {
				cached.Index = int(i)
				out = append(out, cached)
			}
			continue
		}
		d := Dongle{
			Index:   int(i),
			Serial:  cstr(serial[:]),
			Product: cstr(prod[:]),
			Port:    port,
		}
		out = append(out, d)
		if port != "" {
			dongleCache[port] = d
		}
	}
	// Forget devices whose physical ports are no longer enumerated.
	present := make(map[string]bool, len(ports))
	for _, port := range ports {
		if port != "" {
			present[port] = true
		}
	}
	for port := range dongleCache {
		if !present[port] {
			delete(dongleCache, port)
		}
	}
	return out
}

// loadRtlsdr loads librtlsdr from decoderDir (preferred) or PATH, trying both
// common DLL names.
func loadRtlsdr(decoderDir string) *syscall.DLL {
	names := []string{"librtlsdr.dll", "rtlsdr.dll"}
	var candidates []string
	for _, n := range names {
		if decoderDir != "" {
			candidates = append(candidates, filepath.Join(decoderDir, n))
		}
	}
	candidates = append(candidates, names...) // bare names -> loader search path
	for _, c := range candidates {
		if dll, err := syscall.LoadDLL(c); err == nil {
			return dll
		}
	}
	log.Printf("dongles: librtlsdr.dll not loadable (looked in %s and PATH)", decoderDir)
	return nil
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var procSetDllDirectory = kernel32.NewProc("SetDllDirectoryW")

// setDllDirectory adds dir to the DLL search path so librtlsdr's dependencies
// resolve from the bundle.
func setDllDirectory(dir string) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return
	}
	procSetDllDirectory.Call(uintptr(unsafe.Pointer(p)))
}
