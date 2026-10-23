package recovery

import (
	"errors"
	"net/netip"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

// ForAddress honours an explicit adapter. The camera may be reached through a
// gateway; its address need not share a local subnet. HTTP checks reachability.
func ForAddress(address string, port int, adapter string, all []platform.Adapter) (Target, error) {
	source, _ := platform.IPv4RouteSource(address, port)
	target, _, err := targetOnAdapter(address, port, adapter, all, source)
	return target, err
}

// ForDevice prefers the OS route for automatic HTTP connections, even when it
// differs from SADP's multicast discovery adapter. Explicit overrides remain
// restricted to adapters recorded for this device, as offered by the GUI.
func ForDevice(d devices.Device, requested, preferred string, all []platform.Adapter) (Target, error) {
	if d.HTTPPort == nil {
		port := 80
		d.HTTPPort = &port
	}
	source, _ := platform.IPv4RouteSource(d.IPv4, *d.HTTPPort)
	target, err := targetForDevice(d, requested, preferred, all, source)
	if err == nil && d.HTTPSPort != nil {
		if *d.HTTPSPort < 1 || *d.HTTPSPort > 65535 {
			return Target{}, errors.New("invalid advertised HTTPS port")
		}
		target.HTTPSPort = *d.HTTPSPort
	}
	return target, err
}

func targetForDevice(d devices.Device, requested, preferred string, all []platform.Adapter, routeSource string) (Target, error) {
	if d.HTTPPort == nil {
		port := 80
		d.HTTPPort = &port
	}
	if err := validateAddress(d.IPv4, *d.HTTPPort, all); err != nil {
		return Target{}, err
	}
	names := []string{}
	for _, observed := range d.Interfaces {
		if observed.Name == "" {
			continue
		}
		if requested == observed.Name {
			target, _, err := targetOnAdapter(d.IPv4, *d.HTTPPort, requested, all, routeSource)
			return target, err
		}
		if observed.Name == preferred {
			names = append([]string{observed.Name}, names...)
		} else {
			names = append(names, observed.Name)
		}
	}
	if requested != "" {
		return Target{}, errors.New("select an adapter on which this camera was discovered")
	}
	if validIP(routeSource) {
		for _, a := range all {
			target, _, err := targetOnAdapter(d.IPv4, *d.HTTPPort, a.Name, all, routeSource)
			if err == nil && target.Source == routeSource {
				return target, nil
			}
		}
	}
	// If route lookup is unavailable, still attempt HTTP on an observed adapter.
	// Prefer an on-link source, then the preferred/first usable adapter. No
	// request is retried on other adapters if the ensuing HTTP exchange fails.
	var fallback Target
	for _, name := range names {
		target, onLink, err := targetOnAdapter(d.IPv4, *d.HTTPPort, name, all, "")
		if err != nil {
			continue
		}
		if onLink {
			return target, nil
		}
		if fallback.Source == "" {
			fallback = target
		}
	}
	if fallback.Source != "" {
		return fallback, nil
	}
	return Target{}, errors.New("no usable local IPv4 adapter is available for this camera; check the network connection or rediscover")
}

func validateAddress(address string, port int, all []platform.Adapter) error {
	if !validIP(address) || port < 1 || port > 65535 {
		return errors.New("device has no usable advertised IPv4 address and HTTP port")
	}
	ip := netip.MustParseAddr(address)
	for _, a := range all {
		for _, value := range a.Addresses {
			if p, err := netip.ParsePrefix(value); err == nil && p.Addr() == ip {
				return errors.New("camera address is assigned to this computer; check for an IP address conflict")
			}
		}
	}
	return nil
}

func targetOnAdapter(address string, port int, adapter string, all []platform.Adapter, routeSource string) (Target, bool, error) {
	if err := validateAddress(address, port, all); err != nil {
		return Target{}, false, err
	}
	ip := netip.MustParseAddr(address)
	for _, a := range all {
		if a.Name != adapter || !a.UnicastSuitable {
			continue
		}
		var first, routed, direct netip.Prefix
		for _, value := range a.Addresses {
			p, err := netip.ParsePrefix(value)
			if err != nil || !validIP(p.Addr().String()) {
				continue
			}
			if !first.IsValid() {
				first = p
			}
			if p.Addr().String() == routeSource {
				routed = p
			}
			if p.Contains(ip) && (!direct.IsValid() || p.Bits() > direct.Bits()) {
				direct = p
			}
		}
		if direct.IsValid() && direct.Bits() <= 30 {
			v := ip.As4()
			n := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
			hostMask := ^uint32(0) >> direct.Bits()
			if ip == direct.Masked().Addr() || n&hostMask == hostMask {
				return Target{}, false, errors.New("camera address is a local network or broadcast address; check its network configuration")
			}
		}
		for _, p := range []netip.Prefix{routed, direct, first} {
			if p.IsValid() {
				return Target{Address: address, Port: port, Interface: a.Name, Source: p.Addr().String()}, direct.IsValid(), nil
			}
		}
	}
	return Target{}, false, errors.New("selected adapter has no usable local IPv4 address; check the adapter and network connection")
}
