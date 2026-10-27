//go:build windows

package platform

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"strconv"
	"unsafe"
)

var bestRoute2 = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetBestRoute2")

func DestinationRoute(ctx context.Context, address, source string) (RouteDetail, error) {
	if ctx.Err() != nil {
		return RouteDetail{}, ctx.Err()
	}
	ip, local := net.ParseIP(address).To4(), net.ParseIP(source).To4()
	if ip == nil || local == nil {
		return RouteDetail{}, errors.New("invalid IPv4 route target/source")
	}
	if e := bestRoute2.Find(); e != nil {
		return RouteDetail{}, errors.New("GetBestRoute2 unavailable")
	}
	var dst, src, best windows.RawSockaddrInet
	d := (*windows.RawSockaddrInet4)(unsafe.Pointer(&dst))
	d.Family = windows.AF_INET
	copy(d.Addr[:], ip)
	s := (*windows.RawSockaddrInet4)(unsafe.Pointer(&src))
	s.Family = windows.AF_INET
	copy(s.Addr[:], local)
	var row windows.MibIpForwardRow2
	code, _, _ := bestRoute2.Call(0, 0, uintptr(unsafe.Pointer(&src)), uintptr(unsafe.Pointer(&dst)), 0, uintptr(unsafe.Pointer(&row)), uintptr(unsafe.Pointer(&best)))
	if code != 0 {
		return RouteDetail{}, errors.New("Windows destination route unavailable")
	}
	next := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.NextHop))
	prefix := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.DestinationPrefix.Prefix))
	chosen := (*windows.RawSockaddrInet4)(unsafe.Pointer(&best))
	name := "interface index " + strconv.Itoa(int(row.InterfaceIndex))
	if a, e := net.InterfaceByIndex(int(row.InterfaceIndex)); e == nil {
		name = a.Name
	}
	gateway := net.IP(next.Addr[:]).String()
	if gateway == "0.0.0.0" {
		gateway = ""
	}
	return RouteDetail{Source: net.IP(chosen.Addr[:]).String(), Interface: name, Gateway: gateway, Prefix: net.IP(prefix.Addr[:]).String() + "/" + strconv.Itoa(int(row.DestinationPrefix.PrefixLength)), Evidence: "Windows GetBestRoute2"}, nil
}
