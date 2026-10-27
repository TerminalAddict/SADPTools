//go:build darwin

package ipscan

import (
	"golang.org/x/net/route"
	"net"
	"syscall"
	"testing"
)

func TestDarwinNeighbourEvidence(t *testing.T) {
	// Synthetic kernel ARP messages. A cached entry is not a successful ping.
	row := func(mac []byte) route.Message {
		return &route.RouteMessage{Index: 7, Flags: syscall.RTF_LLINFO,
			Addrs: []route.Addr{syscall.RTAX_DST: &route.Inet4Addr{IP: [4]byte{192, 0, 2, 5}}, syscall.RTAX_GATEWAY: &route.LinkAddr{Addr: mac}}}
	}
	lookup := func(int) (*net.Interface, error) { return &net.Interface{Name: "en7"}, nil }
	rows := []route.Message{row([]byte{2, 0, 0, 0, 0, 5}), row(nil), row(make([]byte, 6)), row([]byte{255, 255, 255, 255, 255, 255}), &route.RouteMessage{}}
	got := darwinNeighbours(rows, lookup)
	if len(got) != 1 || got[0].IP != "192.0.2.5" || got[0].MAC != "02:00:00:00:00:05" || got[0].Interface != "en7" || got[0].Reachable {
		t.Fatal(got)
	}
}
