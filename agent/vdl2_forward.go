package main

import (
	"context"
	"log"
	"net"
	"time"
)

const vdl2LinkBit uint32 = 2

// runVDL2Forward relays decoder JSON datagrams through the agent so VDL2 gets
// the same truthful byte/link telemetry and installer verification as ADS-B.
func runVDL2Forward(ctx context.Context, local, remote string, st *Stats) error {
	pc, err := net.ListenPacket("udp", local)
	if err != nil {
		return err
	}
	defer pc.Close()
	stop := context.AfterFunc(ctx, func() { _ = pc.Close() })
	defer stop()

	dst, err := net.Dial("udp", remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	buf := make([]byte, 64*1024)
	for {
		_ = pc.SetReadDeadline(time.Now().Add(time.Second))
		n, _, rerr := pc.ReadFrom(buf)
		if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
			if ctx.Err() != nil {
				st.setLink(vdl2LinkBit, false)
				return ctx.Err()
			}
			continue
		}
		if rerr != nil {
			st.setLink(vdl2LinkBit, false)
			return rerr
		}
		wrote, werr := dst.Write(buf[:n])
		if werr != nil {
			st.setLink(vdl2LinkBit, false)
			log.Printf("vdl2 relay write failed: %v", werr)
			return werr
		}
		st.setLink(vdl2LinkBit, true)
		st.addBytes(wrote)
	}
}
