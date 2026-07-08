//go:build !windows

package main

import "fmt"

// EEPROM management is Windows-only for now (the feeder targets Windows office PCs).
func dumpEEPROM(decoderDir string) error {
	return fmt.Errorf("eeprom dump not supported on this platform")
}

func eepromSelfTest(decoderDir string, index int) error {
	return fmt.Errorf("eeprom selftest not supported on this platform")
}

func writeDongleSerial(decoderDir string, index int, serial string) error {
	return fmt.Errorf("eeprom serial write not supported on this platform")
}
