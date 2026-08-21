package main

import "testing"

func TestAVRToBeastLong(t *testing.T) {
	f, err := avrToBeast("*8DABCDEF58C382D690C8AC2863A7;")
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 23 || f[0] != 0x1a || f[1] != 0x33 {
		t.Fatalf("unexpected Beast frame: %x", f)
	}
}

func TestAVRToBeastRejectsBadInput(t *testing.T) {
	if _, err := avrToBeast("not-a-frame"); err == nil {
		t.Fatal("expected invalid AVR error")
	}
}

func TestAVRToBeastEscapes(t *testing.T) {
	f, err := avrToBeast("*1A000000000000;")
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 17 || f[9] != 0x1a || f[10] != 0x1a {
		t.Fatalf("escape missing: %x", f)
	}
}
