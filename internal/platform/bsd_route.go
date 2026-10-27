package platform

import (
	"errors"
	"net"
	"net/netip"
	"strings"
)

// parseDarwinRoute is portable so its synthetic fixtures run on every CI host.
// Link-layer next hops and missing masks stay unknown, never fabricated.
func parseDarwinRoute(data []byte, source, adapter string) (RouteDetail, error) {
	if len(data) > 64<<10 {
		return RouteDetail{}, errors.New("macOS route output exceeds bound")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if _, exists := fields[key]; exists {
				return RouteDetail{}, errors.New("ambiguous macOS route output")
			}
			fields[key] = value
		}
	}
	if fields["interface"] != adapter || adapter == "" {
		return RouteDetail{}, errors.New("macOS route does not use the selected source adapter")
	}
	out := RouteDetail{Source: source, Interface: adapter, Evidence: "macOS route get scoped to source adapter; routing hint, not reachability proof"}
	if ip, err := netip.ParseAddr(fields["gateway"]); err == nil && ip.Is4() && !ip.IsUnspecified() {
		out.Gateway = ip.String()
	}
	destination := fields["destination"]
	if destination == "default" {
		out.Prefix = "0.0.0.0/0"
	} else if ip, err := netip.ParseAddr(destination); err == nil && ip.Is4() {
		if mask := net.ParseIP(fields["mask"]).To4(); mask != nil {
			if ones, bits := net.IPMask(mask).Size(); bits == 32 {
				out.Prefix = netip.PrefixFrom(ip, ones).Masked().String()
			}
		} else if strings.Contains(","+strings.Trim(fields["flags"], "<>")+",", ",HOST,") {
			out.Prefix = netip.PrefixFrom(ip, 32).String()
		}
	}
	return out, nil
}
