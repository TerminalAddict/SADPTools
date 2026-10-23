package platform

import (
	"errors"
	"net"
	"net/netip"
)

// IPv4RouteSource asks the OS which local IPv4 address it would use for a
// destination. Connecting a UDP socket selects a route without sending a
// datagram. This is a routing hint, not proof of TCP/HTTP reachability.
func IPv4RouteSource(address string, port int) (string, error) {
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || port < 1 || port > 65535 {
		return "", errors.New("route lookup requires an IPv4 address and port")
	}
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IP(ip.AsSlice()), Port: port})
	if err != nil {
		return "", errors.New("could not select an IPv4 route to the camera")
	}
	defer conn.Close()
	source, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || source.IP.To4() == nil || source.IP.IsUnspecified() {
		return "", errors.New("route lookup did not return an IPv4 source address")
	}
	return source.IP.String(), nil
}
