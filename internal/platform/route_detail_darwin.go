//go:build darwin

package platform

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os/exec"
	"time"
)

// DestinationRoute is read-only. Scope the kernel lookup to the adapter owning
// the requested source; macOS route(8) does not accept Linux's `from` argument.
func DestinationRoute(ctx context.Context, address, source string) (RouteDetail, error) {
	for _, value := range []string{address, source} {
		if ip, err := netip.ParseAddr(value); err != nil || !ip.Is4() || ip.IsUnspecified() {
			return RouteDetail{}, errors.New("invalid IPv4 route target/source")
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return RouteDetail{}, err
	}
	name := ""
	for _, iface := range interfaces {
		addresses, _ := iface.Addrs()
		for _, addr := range addresses {
			if prefix, e := netip.ParsePrefix(addr.String()); e == nil && prefix.Addr().String() == source {
				if name != "" && name != iface.Name {
					return RouteDetail{}, errors.New("route source belongs to more than one adapter")
				}
				name = iface.Name
			}
		}
	}
	if name == "" {
		return RouteDetail{}, errors.New("route source is no longer assigned to an adapter")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/sbin/route", "-n", "get", "-inet", "-ifscope", name, address).Output()
	if ctx.Err() != nil {
		return RouteDetail{}, ctx.Err()
	}
	if err != nil {
		return RouteDetail{}, errors.New("macOS scoped destination route is unavailable")
	}
	return parseDarwinRoute(data, source, name)
}
