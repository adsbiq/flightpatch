package main

import "testing"

func TestDongleIdentityDetectsSamePortReplacement(t *testing.T) {
	a := Dongle{Index: 0, Serial: "BLOGV4", Product: "RTL-SDR Blog V4", Port: "1-2"}
	b := Dongle{Index: 0, Serial: "00000001", Product: "NESDR SMArt v5", Port: "1-2"}
	if a.key() != b.key() {
		t.Fatal("expected stable port key")
	}
	if a.identity() == b.identity() {
		t.Fatal("replacement identity must differ")
	}
}
