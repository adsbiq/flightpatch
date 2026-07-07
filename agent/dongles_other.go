//go:build !windows

package main

// enumerateDongles is a stub on non-Windows builds (the shipped product targets
// Windows and macOS installers; the Linux build is used for CI/cross-compiling).
// Roles then come purely from explicit config.
func enumerateDongles(decoderDir string) []Dongle {
	_ = decoderDir
	return nil
}
