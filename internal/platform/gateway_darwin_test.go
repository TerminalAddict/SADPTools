//go:build darwin

package platform

import (
	"golang.org/x/net/route"
	"syscall"
	"testing"
)

func TestDarwinDefaultGateway(t *testing.T) {
	// Generated kernel-route fixture, not a captured network table.
	row := func(index int, ip [4]byte) *route.RouteMessage {
		return &route.RouteMessage{Index: index, Flags: syscall.RTF_UP | syscall.RTF_GATEWAY,
			Addrs: []route.Addr{syscall.RTAX_DST: &route.Inet4Addr{}, syscall.RTAX_GATEWAY: &route.Inet4Addr{IP: ip}, syscall.RTAX_NETMASK: &route.Inet4Addr{}}}
	}
	rows := []route.Message{row(7, [4]byte{192, 0, 2, 1}), row(8, [4]byte{198, 51, 100, 1}), &route.RouteMessage{}}
	if got := darwinDefaultGateway(rows, 7); got != "192.0.2.1" {
		t.Fatal(got)
	}
	if got := darwinDefaultGateway(rows, 9); got != "" {
		t.Fatal(got)
	}
	rows = append(rows, row(7, [4]byte{192, 0, 2, 254}))
	if got := darwinDefaultGateway(rows, 7); got != "" {
		t.Fatal("ambiguous gateway", got)
	}
}
