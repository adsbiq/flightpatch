package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
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
	var history *bufio.Writer
	var historyFile *os.File
	if base := os.Getenv("ProgramData"); base != "" {
		dir := filepath.Join(base, "ADSBiq")
		_ = os.MkdirAll(dir, 0755)
		path := filepath.Join(dir, "vdl2-history.jsonl")
		if info, e := os.Stat(path); e == nil && info.Size() > 16*1024*1024 {
			_ = os.Rename(path, path+".previous")
		}
		if f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); e == nil {
			historyFile = f
			history = bufio.NewWriterSize(f, 64*1024)
			defer history.Flush()
			defer historyFile.Close()
		}
	}
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
		if history != nil && json.Valid(buf[:n]) {
			entry, _ := json.Marshal(struct {
				ReceivedAt string          `json:"receivedAt"`
				Payload    json.RawMessage `json:"payload"`
			}{time.Now().UTC().Format(time.RFC3339Nano), append(json.RawMessage(nil), buf[:n]...)})
			_, _ = history.Write(entry)
			_ = history.WriteByte('\n')
			_ = history.Flush()
		}
	}
}
