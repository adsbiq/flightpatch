// ADSBiq feed agent (M1): forward Beast/TCP from a local decoder to the adsbiq network,
// reconnecting both sides forever. Single static binary, zero external deps.
//
//	dongle -> dump1090 (127.0.0.1:30005 Beast) -> [this agent] -> feed.adsbiq.com:30004
//
// M3 will add feeder registration (tie to the user's account) + run-as-service.
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
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

// Run forwards Beast bytes local->feed forever, reconnecting on any drop, until ctx is
// cancelled (then returns ctx.Err()).
func Run(ctx context.Context, cfg Config) error {
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
		// Tear both conns down if the context is cancelled while io.Copy is blocked.
		stop := context.AfterFunc(ctx, func() { src.Close(); dst.Close() })
		n, cerr := io.Copy(dst, src)
		stop()
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

func main() {
	cfg := DefaultConfig()
	flag.StringVar(&cfg.Local, "local", cfg.Local, "local decoder Beast output host:port")
	flag.StringVar(&cfg.Feed, "feed", cfg.Feed, "adsbiq Beast ingest host:port")
	flag.Parse()
	log.Printf("adsbiq feed agent: %s -> %s", cfg.Local, cfg.Feed)
	if err := Run(context.Background(), cfg); err != nil {
		log.Printf("stopped: %v", err)
	}
}
