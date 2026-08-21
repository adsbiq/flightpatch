package main

import (
	"bytes"
	"testing"
)

func TestReceiverIdentityStableAndPerRadio(t *testing.T) {
	a, ak := receiverIdentity("dev-example", "usb-1")
	b, bk := receiverIdentity("dev-example", "usb-2")
	a2, ak2 := receiverIdentity("dev-example", "usb-1")
	if len(a) != 8 || len(b) != 8 || ak == bk || bytes.Equal(a, b) {
		t.Fatal("receiver IDs must be distinct 8-byte values")
	}
	if ak != ak2 || !bytes.Equal(a, a2) {
		t.Fatal("receiver identity must be stable")
	}
}

func TestBeastReceiverFrameEscapesLiteralEscape(t *testing.T) {
	got := beastReceiverFrame([]byte{1, 2, 0x1a, 3, 4, 5, 6, 7})
	want := []byte{0x1a, 0xe3, 1, 2, 0x1a, 0x1a, 3, 4, 5, 6, 7}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
	if beastReceiverFrame([]byte{1}) != nil {
		t.Fatal("invalid receiver ID length must be rejected")
	}
}
