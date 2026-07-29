package main

import (
	"sync/atomic"
	"time"
)

// Stats is the live counters the phone-home loop reports to the server.
// Fields are touched atomically by the forwarder and read by telemetry.
type Stats struct {
	bytesFed  int64 // atomic: total bytes forwarded to adsbiq this run
	connected int32 // atomic bool: is the feed link currently up
	start     time.Time
	changed   chan struct{}
}

func newStats() *Stats {
	return &Stats{start: time.Now(), changed: make(chan struct{}, 1)}
}

func (s *Stats) notify() {
	if s == nil || s.changed == nil {
		return
	}
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Stats) setConnected(v bool) {
	var next int32
	if v {
		next = 1
	}
	if atomic.SwapInt32(&s.connected, next) != next {
		s.notify()
	}
}

// snapshot returns a consistent-enough read of the counters plus a byte-rate
// estimate (bytes/sec since the previous snapshot).
func (s *Stats) snapshot(prevBytes int64, prevAt time.Time, now time.Time) (bytes int64, connected bool, uptimeS int64, rate float64) {
	bytes = atomic.LoadInt64(&s.bytesFed)
	connected = atomic.LoadInt32(&s.connected) == 1
	uptimeS = int64(now.Sub(s.start).Seconds())
	if dt := now.Sub(prevAt).Seconds(); dt > 0 && !prevAt.IsZero() {
		rate = float64(bytes-prevBytes) / dt
	}
	return
}
