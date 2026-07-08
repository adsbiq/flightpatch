//go:build !windows

package main

// donglePorts is Windows-only for now; elsewhere the supervisor falls back to the
// serial as the device key.
func donglePorts(decoderDir string) []string { return nil }
