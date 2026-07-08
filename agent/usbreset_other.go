//go:build !windows

package main

// resetRtlDevices is a no-op off Windows (Linux/macOS re-read the descriptor or
// use udev; the feeder targets Windows office PCs for now).
func resetRtlDevices() {}
