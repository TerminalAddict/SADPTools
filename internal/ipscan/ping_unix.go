//go:build !windows

package ipscan

import (
	"bytes"
	"context"
	"crypto/rand"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"net"
	"time"
)

func ping(ctx context.Context, ip, source string, timeout time.Duration) Evidence {
	ev := Evidence{Probe: "icmp", At: stamp(), State: "no-response"}
	if ctx.Err() != nil {
		return ev
	}
	c, e := icmp.ListenPacket("udp4", source)
	raw := false
	if e != nil {
		c, e = icmp.ListenPacket("ip4:icmp", source)
		raw = true
	}
	if e != nil {
		ev.State = "unsupported"
		ev.Detail = "ICMP socket unavailable on this adapter; grant ping-socket capability or enable optional TCP probes. No privileges were changed."
		return ev
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	c.SetDeadline(time.Now().Add(timeout))
	payload := make([]byte, 24)
	rand.Read(payload)
	seq := int(payload[0])<<8 | int(payload[1])
	body := &icmp.Echo{ID: seq, Seq: seq, Data: payload}
	data, _ := (&icmp.Message{Type: ipv4.ICMPTypeEcho, Code: 0, Body: body}).Marshal(nil)
	var destination net.Addr = &net.UDPAddr{IP: net.ParseIP(ip)}
	if raw {
		destination = &net.IPAddr{IP: net.ParseIP(ip)}
	}
	start := time.Now()
	if _, e = c.WriteTo(data, destination); e != nil {
		ev.Detail = "ICMP send failed"
		return ev
	}
	buf := make([]byte, 1500)
	for {
		n, peer, e := c.ReadFrom(buf)
		if e != nil {
			ev.Detail = "No ICMP echo reply"
			return ev
		}
		peerIP := peer.String()
		if host, _, e := net.SplitHostPort(peerIP); e == nil {
			peerIP = host
		}
		if peerIP != ip {
			continue
		}
		m, e := icmp.ParseMessage(1, buf[:n])
		if e != nil || m.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := m.Body.(*icmp.Echo)
		if !ok || echo.Seq != seq || !bytes.Equal(echo.Data, payload) {
			continue
		}
		ms := float64(time.Since(start).Microseconds()) / 1000
		ev.State = "responded"
		ev.Detail = "ICMP echo reply"
		ev.LatencyMS = &ms
		return ev
	}
}
