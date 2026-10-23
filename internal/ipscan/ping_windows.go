//go:build windows

package ipscan

import (
	"context"
	"encoding/binary"
	"golang.org/x/sys/windows"
	"net/netip"
	"time"
	"unsafe"
)

var iphelper = windows.NewLazySystemDLL("iphlpapi.dll")
var icmpCreate = iphelper.NewProc("IcmpCreateFile")
var icmpClose = iphelper.NewProc("IcmpCloseHandle")
var icmpEcho = iphelper.NewProc("IcmpSendEcho2Ex")

func ping(ctx context.Context, ip, source string, timeout time.Duration) Evidence {
	ev := Evidence{Probe: "icmp", At: stamp(), State: "no-response"}
	if ctx.Err() != nil {
		return ev
	}
	h, _, _ := icmpCreate.Call()
	if h == 0 || h == ^uintptr(0) {
		ev.State = "unsupported"
		ev.Detail = "Windows ICMP API is unavailable"
		return ev
	}
	defer icmpClose.Call(h)
	dest, e := netip.ParseAddr(ip)
	if e != nil || !dest.Is4() {
		return ev
	}
	src, e := netip.ParseAddr(source)
	if e != nil || !src.Is4() {
		return ev
	}
	da, sa := dest.As4(), src.As4()
	data := []byte("SADPTools IP investigation")
	reply := make([]byte, 256)
	count, _, _ := icmpEcho.Call(h, 0, 0, 0, uintptr(binary.LittleEndian.Uint32(sa[:])), uintptr(binary.LittleEndian.Uint32(da[:])), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	if ctx.Err() == nil && count > 0 && binary.LittleEndian.Uint32(reply[4:8]) == 0 && binary.LittleEndian.Uint32(reply[:4]) == binary.LittleEndian.Uint32(da[:]) {
		ms := float64(binary.LittleEndian.Uint32(reply[8:12]))
		ev.State = "responded"
		ev.Detail = "Windows ICMP echo reply"
		ev.LatencyMS = &ms
	} else {
		ev.Detail = "No ICMP echo reply"
	}
	return ev
}
