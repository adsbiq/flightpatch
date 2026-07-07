package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// mockServer accepts TCP connections and hands each to onConn.
func mockServer(t *testing.T, onConn func(net.Conn)) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go onConn(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// The core M1 guarantee: bytes from the local decoder reach the adsbiq ingest.
func TestForwardsBytesEndToEnd(t *testing.T) {
	payload := []byte("\x1a\x33BEAST-FRAME-TEST-1234567890")
	decAddr, decClose := mockServer(t, func(c net.Conn) {
		c.Write(payload)
		time.Sleep(200 * time.Millisecond)
		c.Close()
	})
	defer decClose()

	got := make(chan []byte, 1)
	ingAddr, ingClose := mockServer(t, func(c net.Conn) {
		buf := make([]byte, 512)
		n, _ := c.Read(buf)
		got <- buf[:n]
		c.Close()
	})
	defer ingClose()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := Config{Local: decAddr, Feed: ingAddr, DialTimeout: 2 * time.Second, RetryDelay: 200 * time.Millisecond}
	go Run(ctx, cfg)

	select {
	case b := <-got:
		if string(b) != string(payload) {
			t.Fatalf("ingest got %q, want %q", b, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: ingest never received forwarded bytes")
	}
}

// Reconnect: after the decoder drops, the agent re-establishes and forwards again.
func TestReconnectsAfterDrop(t *testing.T) {
	round := make(chan int, 4)
	decAddr, decClose := mockServer(t, func(c net.Conn) {
		c.Write([]byte("frame"))
		round <- 1
		c.Close()
	})
	defer decClose()
	ingAddr, ingClose := mockServer(t, func(c net.Conn) {
		io.Copy(discard{}, c)
	})
	defer ingClose()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, Config{Local: decAddr, Feed: ingAddr, DialTimeout: time.Second, RetryDelay: 100 * time.Millisecond})

	// expect at least two connect rounds (initial + one reconnect)
	for i := 0; i < 2; i++ {
		select {
		case <-round:
		case <-time.After(4 * time.Second):
			t.Fatalf("only got %d decoder connects, want >=2 (reconnect failed)", i)
		}
	}
}

// Cancelling the context stops Run promptly even while it is dialing.
func TestRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := Config{Local: "127.0.0.1:1", Feed: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, RetryDelay: 200 * time.Millisecond}
	errc := make(chan error, 1)
	go func() { errc <- Run(ctx, cfg) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err != context.Canceled {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
