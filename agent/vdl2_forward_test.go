package main

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestVDL2RelayCountsOnlyDeliveredDatagrams(t *testing.T) {
	sink, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	local, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localAddr := local.LocalAddr().String()
	_ = local.Close() // reserve an ephemeral address, then let the relay bind it

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stats := newStats()
	done := make(chan error, 1)
	go func() { done <- runVDL2Forward(ctx, localAddr, sink.LocalAddr().String(), stats) }()
	time.Sleep(50 * time.Millisecond)

	var src net.Conn
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		src, err = net.Dial("udp", localAddr)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	payload := []byte(`{"vdl2":"test"}`)
	if _, err := src.Write(payload); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 256)
	_ = sink.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := sink.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != string(payload) {
		t.Fatalf("relayed %q, want %q", buf[:n], payload)
	}

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		bytes, connected, _, _ := stats.snapshot(0, time.Time{}, time.Now())
		if connected && bytes == int64(len(payload)) {
			cancel()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("VDL2 delivery did not update truthful telemetry")
}
