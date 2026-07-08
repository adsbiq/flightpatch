//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"unsafe"
)

const eepromSize = 256

// readEEPROM opens dongle `index`, reads its raw 256-byte EEPROM image, and
// closes it. Read-only. Fails if the device is already open (a decoder has it).
func readEEPROM(decoderDir string, index int) ([]byte, error) {
	initRtlsdr(decoderDir)
	if rtlDLL == nil {
		return nil, fmt.Errorf("librtlsdr not loaded")
	}
	open, e1 := rtlDLL.FindProc("rtlsdr_open")
	readEE, e2 := rtlDLL.FindProc("rtlsdr_read_eeprom")
	closeP, e3 := rtlDLL.FindProc("rtlsdr_close")
	if e1 != nil || e2 != nil || e3 != nil {
		return nil, fmt.Errorf("librtlsdr missing eeprom procs")
	}
	var dev uintptr
	// int rtlsdr_open(rtlsdr_dev_t **dev, uint32_t index)
	r, _, _ := open.Call(uintptr(unsafe.Pointer(&dev)), uintptr(index))
	if int32(r) != 0 || dev == 0 {
		return nil, fmt.Errorf("rtlsdr_open(%d) failed rc=%d (device busy?)", index, int32(r))
	}
	defer closeP.Call(dev)
	buf := make([]byte, eepromSize)
	// int rtlsdr_read_eeprom(dev, uint8_t *data, uint8_t offset, uint16_t len)
	rr, _, _ := readEE.Call(dev, uintptr(unsafe.Pointer(&buf[0])), 0, uintptr(eepromSize))
	if int32(rr) < 0 {
		return nil, fmt.Errorf("rtlsdr_read_eeprom failed rc=%d", int32(rr))
	}
	return buf, nil
}

// dumpEEPROM reads dongle 0's EEPROM, backs it up to a file, hex-prints it, and
// prints the parsed serial. Purely diagnostic (read-only) — used to learn the
// exact byte layout before we ever write a serial.
func dumpEEPROM(decoderDir string) error {
	data, err := readEEPROM(decoderDir, 0)
	if err != nil {
		return err
	}
	bk := os.TempDir() + string(os.PathSeparator) + "adsbiq-eeprom-backup.bin"
	if werr := os.WriteFile(bk, data, 0o600); werr == nil {
		log.Printf("EEPROM backed up to %s", bk)
	}
	log.Printf("EEPROM (%d bytes):", len(data))
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		log.Printf("  %03d: % x  |%s|", i, data[i:end], asciiOnly(data[i:end]))
	}
	manu, prod, serial := parseEEPROMStrings(data)
	log.Printf("parsed strings: manufacturer=%q product=%q serial=%q", manu, prod, serial)
	return nil
}

func asciiOnly(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

// parseEEPROMStrings extracts the three USB string descriptors (manufacturer,
// product, serial) from an RTL2832U EEPROM image. Layout (rtl_eeprom): magic
// 0x28 0x32 at 0, VID at 2, PID at 4, have_serial at 6, flags at 7, then the
// string descriptors begin at offset 0x09, each: [len][0x03][UTF-16LE chars].
func parseEEPROMStrings(d []byte) (manu, prod, serial string) {
	if len(d) < 10 || d[0] != 0x28 || d[1] != 0x32 {
		return "", "", ""
	}
	pos := 0x09
	out := make([]string, 0, 3)
	for k := 0; k < 3; k++ {
		if pos+1 >= len(d) {
			break
		}
		l := int(d[pos])
		if l < 2 || d[pos+1] != 0x03 || pos+l > len(d) {
			break
		}
		var sb strings.Builder
		for j := pos + 2; j+1 < pos+l; j += 2 {
			sb.WriteByte(d[j]) // low byte of each UTF-16LE unit (ASCII names)
		}
		out = append(out, sb.String())
		pos += l
	}
	for len(out) < 3 {
		out = append(out, "")
	}
	return out[0], out[1], out[2]
}

// setEEPROMSerial returns a copy of the EEPROM image with the SERIAL string
// descriptor (the 3rd, at the end of the strings block) replaced. Pure/testable.
// The serial must be ASCII. Manufacturer + product descriptors are untouched.
func setEEPROMSerial(orig []byte, serial string) ([]byte, error) {
	if len(orig) < 128 || orig[0] != 0x28 || orig[1] != 0x32 {
		return nil, fmt.Errorf("not an RTL2832U eeprom image")
	}
	// Walk past the manufacturer (1st) and product (2nd) descriptors to find the
	// serial (3rd) descriptor start.
	pos := 0x09
	for k := 0; k < 2; k++ {
		if pos+1 >= len(orig) {
			return nil, fmt.Errorf("truncated string descriptors")
		}
		l := int(orig[pos])
		if l < 2 || orig[pos+1] != 0x03 {
			return nil, fmt.Errorf("bad string descriptor at offset %d", pos)
		}
		pos += l
	}
	serStart := pos
	dlen := 2 + 2*len(serial)
	if len(serial) == 0 || dlen > 254 || serStart+dlen > 128 {
		return nil, fmt.Errorf("serial length %d does not fit", len(serial))
	}
	out := make([]byte, len(orig))
	copy(out, orig)
	out[6] = 0xa5 // have_serial
	out[serStart] = byte(dlen)
	out[serStart+1] = 0x03
	for i := 0; i < len(serial); i++ {
		out[serStart+2+2*i] = serial[i]
		out[serStart+2+2*i+1] = 0x00
	}
	// Zero any leftover in the strings area so no stale bytes remain.
	for i := serStart + dlen; i < 128; i++ {
		out[i] = 0x00
	}
	return out, nil
}

// writeEEPROM writes len(data) bytes to the dongle's EEPROM starting at `offset`.
// The device must be free (no decoder running). rtlsdr_read/write_eeprom hit the
// I2C chip directly, so a read-back reflects the write without a replug.
func writeEEPROM(decoderDir string, index int, data []byte, offset int) error {
	initRtlsdr(decoderDir)
	if rtlDLL == nil {
		return fmt.Errorf("librtlsdr not loaded")
	}
	open, e1 := rtlDLL.FindProc("rtlsdr_open")
	writeEE, e2 := rtlDLL.FindProc("rtlsdr_write_eeprom")
	closeP, e3 := rtlDLL.FindProc("rtlsdr_close")
	if e1 != nil || e2 != nil || e3 != nil {
		return fmt.Errorf("librtlsdr missing eeprom procs")
	}
	var dev uintptr
	r, _, _ := open.Call(uintptr(unsafe.Pointer(&dev)), uintptr(index))
	if int32(r) != 0 || dev == 0 {
		return fmt.Errorf("rtlsdr_open(%d) failed rc=%d (device busy?)", index, int32(r))
	}
	defer closeP.Call(dev)
	// int rtlsdr_write_eeprom(dev, uint8_t *data, uint8_t offset, uint16_t len)
	rr, _, _ := writeEE.Call(dev, uintptr(unsafe.Pointer(&data[0])), uintptr(offset), uintptr(len(data)))
	if int32(rr) < 0 {
		return fmt.Errorf("rtlsdr_write_eeprom failed rc=%d", int32(rr))
	}
	return nil
}

// writeDongleSerial sets a dongle's USB serial (EEPROM) and verifies via chip
// read-back. Used by the collision de-duplicator and the --set-serial tool.
func writeDongleSerial(decoderDir string, index int, serial string) error {
	orig, err := readEEPROM(decoderDir, index)
	if err != nil {
		return err
	}
	mod, err := setEEPROMSerial(orig, serial)
	if err != nil {
		return err
	}
	if err := writeEEPROM(decoderDir, index, mod[:128], 0); err != nil {
		return err
	}
	after, err := readEEPROM(decoderDir, index)
	if err != nil {
		return err
	}
	if _, _, s := parseEEPROMStrings(after); s != serial {
		return fmt.Errorf("serial verify failed: got %q want %q", s, serial)
	}
	return nil
}

// eepromSelfTest proves the write path on real hardware WITHOUT permanently
// changing the dongle: read current serial, back it up, write a test serial,
// verify via chip read-back, then restore the original and verify. Leaves the
// dongle exactly as found (or points at the backup if restore ever fails).
func eepromSelfTest(decoderDir string, index int) error {
	orig, err := readEEPROM(decoderDir, index)
	if err != nil {
		return err
	}
	_, _, origSerial := parseEEPROMStrings(orig)
	bk := os.TempDir() + string(os.PathSeparator) + fmt.Sprintf("adsbiq-eeprom-%d-%s.bak", index, origSerial)
	_ = os.WriteFile(bk, orig, 0o600)
	log.Printf("selftest #%d: current serial=%q (backup %s)", index, origSerial, bk)

	testSerial := fmt.Sprintf("ADSBIQ%d", index)
	mod, err := setEEPROMSerial(orig, testSerial)
	if err != nil {
		return err
	}
	if err := writeEEPROM(decoderDir, index, mod[:128], 0); err != nil {
		return fmt.Errorf("writing test serial: %w", err)
	}
	after, err := readEEPROM(decoderDir, index)
	if err != nil {
		return fmt.Errorf("read-back after write: %w (restore from %s)", err, bk)
	}
	_, _, gotSerial := parseEEPROMStrings(after)
	writeOK := gotSerial == testSerial
	log.Printf("selftest #%d: after write serial=%q (wanted %q) -> %v", index, gotSerial, testSerial, writeOK)

	// Always attempt restore.
	if err := writeEEPROM(decoderDir, index, orig[:128], 0); err != nil {
		return fmt.Errorf("RESTORE FAILED: %w -- restore manually from %s", err, bk)
	}
	restored, err := readEEPROM(decoderDir, index)
	if err != nil {
		return fmt.Errorf("read-back after restore: %w (restore from %s)", err, bk)
	}
	_, _, rSerial := parseEEPROMStrings(restored)
	log.Printf("selftest #%d: after restore serial=%q (wanted %q)", index, rSerial, origSerial)

	if !writeOK {
		return fmt.Errorf("WRITE VERIFY FAILED: serial did not become %q", testSerial)
	}
	if rSerial != origSerial {
		return fmt.Errorf("RESTORE VERIFY FAILED: serial=%q, expected %q (backup %s)", rSerial, origSerial, bk)
	}
	log.Printf("selftest #%d PASS: wrote %q, verified, restored %q -- dongle unchanged", index, testSerial, origSerial)
	return nil
}
