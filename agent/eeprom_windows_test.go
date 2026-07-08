//go:build windows

package main

import "testing"

// buildTestImage builds a minimal valid RTL2832U EEPROM image with the three
// USB string descriptors, matching the layout decoded from a real V4.
func buildTestImage(manu, prod, serial string) []byte {
	img := make([]byte, 256)
	img[0], img[1] = 0x28, 0x32 // magic
	img[2], img[3] = 0xda, 0x0b // VID 0x0bda
	img[4], img[5] = 0x38, 0x28 // PID 0x2838
	img[6] = 0xa5               // have_serial
	img[7], img[8] = 0x16, 0x02
	pos := 9
	for _, s := range []string{manu, prod, serial} {
		img[pos] = byte(2 + 2*len(s))
		img[pos+1] = 0x03
		for i := 0; i < len(s); i++ {
			img[pos+2+2*i] = s[i]
		}
		pos += 2 + 2*len(s)
	}
	return img
}

func TestParseEEPROMStrings(t *testing.T) {
	img := buildTestImage("RTLSDRBlog", "Blog V4", "BLOGV4")
	m, p, s := parseEEPROMStrings(img)
	if m != "RTLSDRBlog" || p != "Blog V4" || s != "BLOGV4" {
		t.Fatalf("parsed %q/%q/%q", m, p, s)
	}
}

func TestSetEEPROMSerialReplacesOnlySerial(t *testing.T) {
	img := buildTestImage("RTLSDRBlog", "Blog V4", "BLOGV4")
	mod, err := setEEPROMSerial(img, "ADSBIQ1")
	if err != nil {
		t.Fatal(err)
	}
	m, p, s := parseEEPROMStrings(mod)
	if s != "ADSBIQ1" {
		t.Fatalf("serial = %q, want ADSBIQ1", s)
	}
	if m != "RTLSDRBlog" || p != "Blog V4" {
		t.Fatalf("manufacturer/product clobbered: %q / %q", m, p)
	}
	if mod[6] != 0xa5 {
		t.Fatalf("have_serial flag not set")
	}
}

func TestSetEEPROMSerialRoundTrip(t *testing.T) {
	// Two dongles sharing "BLOGV4" -> give #2 a unique serial, then confirm we can
	// still restore it exactly (the reversible-write property the selftest relies on).
	img := buildTestImage("RTLSDRBlog", "Blog V4", "BLOGV4")
	mod, err := setEEPROMSerial(img, "ADSBIQ2")
	if err != nil {
		t.Fatal(err)
	}
	back, err := setEEPROMSerial(mod, "BLOGV4")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, s := parseEEPROMStrings(back); s != "BLOGV4" {
		t.Fatalf("round-trip serial = %q, want BLOGV4", s)
	}
}

func TestSetEEPROMSerialRejectsBadImage(t *testing.T) {
	if _, err := setEEPROMSerial(make([]byte, 256), "X"); err == nil {
		t.Fatal("expected error on non-RTL image (no magic)")
	}
	img := buildTestImage("RTLSDRBlog", "Blog V4", "BLOGV4")
	if _, err := setEEPROMSerial(img, ""); err == nil {
		t.Fatal("expected error on empty serial")
	}
}
