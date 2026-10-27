//go:build darwin

package platform

import (
	"net"
	"syscall"

	"golang.org/x/net/route"
)

func gateway(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	data, err := route.FetchRIB(syscall.AF_INET, route.RIBTypeRoute, 0)
	if err != nil || len(data) > 16<<20 {
		return ""
	}
	messages, err := route.ParseRIB(route.RIBTypeRoute, data)
	if err != nil {
		return ""
	}
	return darwinDefaultGateway(messages, iface.Index)
}

func darwinDefaultGateway(messages []route.Message, index int) string {
	result := ""
	for _, message := range messages {
		r, ok := message.(*route.RouteMessage)
		if !ok || r.Err != nil || r.Index != index || r.Flags&syscall.RTF_UP == 0 || r.Flags&syscall.RTF_GATEWAY == 0 || len(r.Addrs) <= syscall.RTAX_NETMASK {
			continue
		}
		dst, ok := r.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		if !ok || dst.IP != [4]byte{} {
			continue
		}
		mask, ok := r.Addrs[syscall.RTAX_NETMASK].(*route.Inet4Addr)
		if !ok || mask.IP != [4]byte{} {
			continue
		}
		gw, ok := r.Addrs[syscall.RTAX_GATEWAY].(*route.Inet4Addr)
		if !ok || gw.IP == [4]byte{} {
			continue
		}
		value := net.IP(gw.IP[:]).String()
		if result != "" && result != value {
			return ""
		} // ambiguous default
		result = value
	}
	return result
}
