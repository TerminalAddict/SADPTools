//go:build darwin

package ipscan

import (
	"errors"
	"golang.org/x/net/route"
	"net"
	"syscall"
)

// The kernel's ARP cache is evidence only, never proof of current reachability.
// Reading the routing information base needs neither a subprocess nor root.
func neighbours() ([]Neighbour, error) {
	data, err := route.FetchRIB(syscall.AF_INET, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, errors.New("macOS neighbour table exceeds bound")
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, data)
	if err != nil {
		return nil, err
	}
	return darwinNeighbours(messages, net.InterfaceByIndex), nil
}

func darwinNeighbours(messages []route.Message, lookup func(int) (*net.Interface, error)) []Neighbour {
	out := []Neighbour{}
	for _, message := range messages {
		r, ok := message.(*route.RouteMessage)
		if !ok || r.Err != nil || r.Flags&syscall.RTF_LLINFO == 0 || len(r.Addrs) <= syscall.RTAX_GATEWAY {
			continue
		}
		ip, ok := r.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		if !ok || !net.IP(ip.IP[:]).IsGlobalUnicast() {
			continue
		}
		link, ok := r.Addrs[syscall.RTAX_GATEWAY].(*route.LinkAddr)
		if !ok || len(link.Addr) != 6 {
			continue
		}
		mac := net.HardwareAddr(link.Addr)
		if mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
			continue
		}
		iface, err := lookup(r.Index)
		if err != nil {
			continue
		}
		out = append(out, Neighbour{IP: net.IP(ip.IP[:]).String(), MAC: mac.String(), Interface: iface.Name, Source: "macOS ARP cache (age unknown)"})
	}
	return out
}
