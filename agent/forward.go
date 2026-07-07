// Beast/TCP forwarder: pump bytes from a local decoder to the adsbiq network,
// reconnecting both sides forever.
//
//	dongle -> decoder (127.0.0.1:30005 Beast) -> [forwarder] -> feed.adsbiq.com:30004
package main

import (
	"context"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

type Config struct {
	Local       string // local decoder Beast output, e.g. 127.0.0.1:30005
	Feed        string // adsbiq Beast ingest, e.g. feed.adsbiq.com:30004
	DialTimeout time.Duration
	RetryDelay  time.Duration
}

func DefaultConfig() Config {
	return Config{
		Local: "127.0.0.1:30005", Feed: "feed.adsbiq.com:30004",
		DialTimeout: 10 * time.Second, RetryDelay: 5 * time.Second,
	}
}

func keepAlive(c net.Conn) {
	if t, ok := c.(*net.TCPConn); ok {
		_ = t.SetKeepAlive(true)
		_ = t.SetKeepAlivePeriod(30 * time.Second)
	}
}

// dialLoop dials addr until success, or returns ctx.Err() if cancelled.
func dialLoop(ctx context.Context, name, addr string, timeout, retry time.Duration) (net.Conn, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := net.DialTimeout("tcp", addr, timeout)
		if err == nil {
			log.Printf("%s: connected %s", name, addr)
			keepAlive(c)
			return c, nil
		}
		log.Printf("%s: dial %s failed: %v (retry %s)", name, addr, err, retry)
		select {
		case <-time.After(retry):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// countWriter tallies forwarded bytes into a Stats (for telemetry) as it writes.
type countWriter struct {
	w  io.Writer
	st *Stats
}

func (c countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if c.st != nil && n > 0 {
		atomic.AddInt64(&c.st.bytesFed, int64(n))
	}
	return n, err
}

// Run forwards Beast bytes local->feed forever, reconnecting on any drop, until ctx is
// cancelled (then returns ctx.Err()). Byte/connection telemetry is off (nil stats).
func Run(ctx context.Context, cfg Config) error { return runForward(ctx, cfg, nil) }

// runForward is Run with an optional Stats sink so the supervisor can report
// bytes-fed and link-up status to the server.
func runForward(ctx context.Context, cfg Config, st *Stats) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		src, err := dialLoop(ctx, "decoder", cfg.Local, cfg.DialTimeout, cfg.RetryDelay)
		if err != nil {
			return err
		}
		dst, err := dialLoop(ctx, "adsbiq", cfg.Feed, cfg.DialTimeout, cfg.RetryDelay)
		if err != nil {
			src.Close()
			return err
		}
		if st != nil {
			atomic.StoreInt32(&st.connected, 1)
		}
		// Tear both conns down if the context is cancelled while io.Copy is blocked.
		stop := context.AfterFunc(ctx, func() { src.Close(); dst.Close() })
		n, cerr := io.Copy(countWriter{dst, st}, src)
		stop()
		if st != nil {
			atomic.StoreInt32(&st.connected, 0)
		}
		src.Close()
		dst.Close()
		log.Printf("link dropped after %d bytes (%v); reconnecting", n, cerr)
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
