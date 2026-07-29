package main

import (
	"testing"
	"time"
)

func TestStateEdgesWakeTelemetryImmediately(t *testing.T) {
	s := newStats()
	s.setConnected(true)
	select {
	case <-s.changed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("connection edge did not wake telemetry")
	}

	// Repeating the same state is not an edge and must not create a request storm.
	s.setConnected(true)
	select {
	case <-s.changed:
		t.Fatal("unchanged connection state triggered telemetry")
	case <-time.After(20 * time.Millisecond):
	}

	s.setConnected(false)
	select {
	case <-s.changed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("disconnect edge did not wake telemetry")
	}
}
