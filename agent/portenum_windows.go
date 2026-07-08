//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// donglePorts returns the USB bus-port path for each RTL-SDR device, indexed to
// match librtlsdr's device index (the k-th VID 0BDA:2838 device in libusb order).
// The port path is stable per physical USB port and UNIQUE even when two dongles
// share a serial, so it's the key the supervisor tracks decoders by (Plan A —
// no EEPROM writes, no USB reset). Reading it needs no device open.
func donglePorts(decoderDir string) []string {
	dll := loadLibusb(decoderDir)
	if dll == nil {
		return nil
	}
	defer dll.Release()
	initP, e1 := dll.FindProc("libusb_init")
	exitP, e2 := dll.FindProc("libusb_exit")
	getList, e3 := dll.FindProc("libusb_get_device_list")
	freeList, e4 := dll.FindProc("libusb_free_device_list")
	getDesc, e5 := dll.FindProc("libusb_get_device_descriptor")
	getBus, e6 := dll.FindProc("libusb_get_bus_number")
	getPorts, e7 := dll.FindProc("libusb_get_port_numbers")
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil || e7 != nil {
		return nil
	}
	var ctx uintptr
	if r, _, _ := initP.Call(uintptr(unsafe.Pointer(&ctx))); int32(r) != 0 {
		return nil
	}
	defer exitP.Call(ctx)
	var list unsafe.Pointer // libusb writes the device-array pointer here
	n, _, _ := getList.Call(ctx, uintptr(unsafe.Pointer(&list)))
	cnt := int(n)
	if cnt <= 0 || cnt > 10000 {
		return nil
	}
	defer freeList.Call(uintptr(list), 1)
	ptrSize := unsafe.Sizeof(uintptr(0))
	desc := make([]byte, 18)
	var out []string
	for i := 0; i < cnt; i++ {
		dev := *(*uintptr)(unsafe.Add(list, uintptr(i)*ptrSize))
		if r, _, _ := getDesc.Call(dev, uintptr(unsafe.Pointer(&desc[0]))); int32(r) != 0 {
			continue
		}
		vid := uint16(desc[8]) | uint16(desc[9])<<8
		pid := uint16(desc[10]) | uint16(desc[11])<<8
		if vid != 0x0bda || pid != 0x2838 {
			continue
		}
		bus, _, _ := getBus.Call(dev)
		path := make([]byte, 8)
		pn, _, _ := getPorts.Call(dev, uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)))
		np := int(int32(pn))
		parts := make([]string, 0, 8)
		for j := 0; j < np && j < len(path); j++ {
			parts = append(parts, fmt.Sprintf("%d", path[j]))
		}
		out = append(out, fmt.Sprintf("%d-%s", int(byte(bus)), strings.Join(parts, ".")))
	}
	return out
}

func loadLibusb(decoderDir string) *syscall.DLL {
	name := "libusb-1.0.dll"
	if decoderDir != "" {
		if dll, err := syscall.LoadDLL(filepath.Join(decoderDir, name)); err == nil {
			return dll
		}
	}
	if dll, err := syscall.LoadDLL(name); err == nil {
		return dll
	}
	return nil
}
