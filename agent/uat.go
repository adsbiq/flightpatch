package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// avrToBeast wraps a 7- or 14-byte AVR Mode-S message as Beast binary.
// uat2esnt emits AVR; ADSBiq's existing :30004 ingest consumes Beast.
func avrToBeast(line string) ([]byte, error) {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "*")
	s = strings.TrimSuffix(s, ";")
	raw, err := hex.DecodeString(s)
	if err != nil || (len(raw) != 7 && len(raw) != 14) {
		return nil, fmt.Errorf("invalid AVR frame")
	}
	typ := byte(0x32)
	if len(raw) == 14 {
		typ = 0x33
	}
	payload := append([]byte{0, 0, 0, 0, 0, 0, 0x80}, raw...)
	out := []byte{0x1a, typ}
	for _, b := range payload {
		out = append(out, b)
		if b == 0x1a {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *DecoderManager) superviseUAT(ctx context.Context, d Dongle) {
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		if err := m.runUAT(ctx, d); err != nil && ctx.Err() == nil {
			log.Printf("uat decoder stopped: %v; restarting in %s", err, backoff)
		}
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = minDur(backoff*2, 30*time.Second)
	}
}

func (m *DecoderManager) runUAT(ctx context.Context, d Dongle) error {
	names := uatExeNames()
	paths := make([]string, len(names))
	for i, name := range names {
		p, ok := exePath(m.cfg.DecoderDir, name)
		if !ok {
			return fmt.Errorf("%s not found", name)
		}
		paths[i] = p
	}
	rtl := exec.CommandContext(ctx, paths[0], "-d", strconv.Itoa(d.Index), "-f", "978000000", "-s", "2083334", "-g", m.cfg.Gain, "-")
	demod := exec.CommandContext(ctx, paths[1])
	convert := exec.CommandContext(ctx, paths[2])
	for _, cmd := range []*exec.Cmd{rtl, demod, convert} {
		configureChild(cmd)
		cmd.Stderr = io.Discard
	}
	rtlOut, err := rtl.StdoutPipe()
	if err != nil {
		return err
	}
	demod.Stdin = rtlOut
	demodOut, err := demod.StdoutPipe()
	if err != nil {
		return err
	}
	convert.Stdin = demodOut
	avr, err := convert.StdoutPipe()
	if err != nil {
		return err
	}
	started := []*exec.Cmd{}
	cleanup := func() {
		for _, c := range started {
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}
		for _, c := range started {
			_ = c.Wait()
		}
	}
	defer cleanup()
	// Consumers first prevents pipe backpressure during startup.
	for _, cmd := range []*exec.Cmd{convert, demod, rtl} {
		if err = cmd.Start(); err != nil {
			return err
		}
		started = append(started, cmd)
		niceChild(cmd.Process.Pid)
		assignChildToJob(cmd)
	}
	dst, err := dialLoop(ctx, "adsbiq-uat", m.cfg.Feed, 10*time.Second, 5*time.Second)
	if err != nil {
		return err
	}
	defer dst.Close()
	rawID, _ := receiverIdentity(m.cfg.DeviceID, d.key())
	if _, err = dst.Write(beastReceiverFrame(rawID)); err != nil {
		return err
	}
	m.stats.setConnected(true)
	defer m.stats.setConnected(false)
	stop := context.AfterFunc(ctx, func() { _ = dst.Close() })
	defer stop()
	sc := bufio.NewScanner(avr)
	sc.Buffer(make([]byte, 4096), 64*1024)
	frames := 0
	for sc.Scan() {
		frame, ferr := avrToBeast(sc.Text())
		if ferr != nil {
			continue
		}
		if _, err = (countWriter{dst, m.stats}).Write(frame); err != nil {
			return err
		}
		frames++
	}
	if err = sc.Err(); err != nil {
		return err
	}
	return fmt.Errorf("uat pipeline ended after %d frames", frames)
}
