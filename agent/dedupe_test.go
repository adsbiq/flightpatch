package main

import "testing"

func TestDongleKeyDistinguishesIdenticalSerials(t *testing.T) {
	// Two V4s both report "BLOGV4" but sit in different USB ports -> distinct keys,
	// so the supervisor tracks (and feeds) both. This is the bulletproof property.
	a := Dongle{Serial: "BLOGV4", Port: "1-2"}
	b := Dongle{Serial: "BLOGV4", Port: "1-3"}
	if a.key() == b.key() {
		t.Fatalf("identical serials in different ports must have different keys, got %q", a.key())
	}
	if a.key() != "1-2" {
		t.Fatalf("key=%q, want port 1-2", a.key())
	}
	// No port available -> fall back to serial (still works for distinct serials).
	c := Dongle{Serial: "95115225"}
	if c.key() != "95115225" {
		t.Fatalf("key=%q, want serial fallback", c.key())
	}
}

func TestUniqueSerial(t *testing.T) {
	if got := uniqueSerial(map[string]bool{}); got != "ADSBIQ001" {
		t.Fatalf("empty -> %q, want ADSBIQ001", got)
	}
	taken := map[string]bool{"ADSBIQ001": true, "BLOGV4": true}
	got := uniqueSerial(taken)
	if got != "ADSBIQ002" {
		t.Fatalf("got %q, want ADSBIQ002", got)
	}
	if taken[got] {
		t.Fatalf("returned an already-taken serial %q", got)
	}
}
