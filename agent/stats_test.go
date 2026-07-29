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

func TestOneProtocolDisconnectDoesNotHideOtherLiveProtocol(t *testing.T) {
	s := newStats()
	s.setConnected(true) // ADS-B
	<-s.changed
	s.setLink(vdl2LinkBit, true) // VDL2 joins; aggregate stays connected
	s.setConnected(false)        // ADS-B drops, VDL2 remains
	_, connected, _, _ := s.snapshot(0, time.Time{}, time.Now())
	if !connected {
		t.Fatal("VDL2 link was hidden by ADS-B disconnect")
	}
	s.setLink(vdl2LinkBit, false)
	_, connected, _, _ = s.snapshot(0, time.Time{}, time.Now())
	if connected {
		t.Fatal("aggregate remained connected after all protocol links dropped")
	}
}
