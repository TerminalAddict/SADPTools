package platform

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
)

type Adapter struct {
	Index     int      `json:"index"`
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	Suitable  bool     `json:"suitable"`
	Reason    string   `json:"reason"`
}

// Inspect is separate from enumeration so filtering can be tested without hardware.
func Inspect(iface net.Interface, addrs []net.Addr, addrErr error) Adapter {
	a := Adapter{Index: iface.Index, Name: iface.Name, Addresses: []string{}}
	for _, addr := range addrs {
		p, err := netip.ParsePrefix(addr.String())
		if err == nil && p.Addr().Is4() && !p.Addr().IsUnspecified() && !p.Addr().IsLoopback() && !p.Addr().IsMulticast() {
			a.Addresses = append(a.Addresses, p.String())
		}
	}
	switch {
	case iface.Flags&net.FlagUp == 0:
		a.Reason = "interface is down"
	case iface.Flags&net.FlagLoopback != 0:
		a.Reason = "loopback interface"
	case iface.Flags&net.FlagMulticast == 0:
		a.Reason = "multicast is unavailable"
	case addrErr != nil:
		a.Reason = "cannot read addresses: " + addrErr.Error()
	case len(a.Addresses) == 0:
		a.Reason = "no usable IPv4 address"
	default:
		a.Suitable = true
	}
	return a
}

func Interfaces() ([]Adapter, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate network interfaces: %w", err)
	}
	result := make([]Adapter, 0, len(interfaces))
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		result = append(result, Inspect(iface, addrs, err))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}

func Select(all []Adapter, name string) ([]Adapter, error) {
	result := []Adapter{}
	for _, a := range all {
		if name != "" && a.Name != name {
			continue
		}
		if !a.Suitable {
			if name != "" {
				return nil, fmt.Errorf("interface %q is unavailable: %s", name, a.Reason)
			}
			continue
		}
		result = append(result, a)
	}
	if len(result) == 0 {
		if name != "" {
			return nil, fmt.Errorf("interface %q not found", name)
		}
		return nil, fmt.Errorf("no active IPv4 multicast interfaces available")
	}
	return result, nil
}

func (a Adapter) Address() string {
	if len(a.Addresses) == 0 {
		return ""
	}
	p, err := netip.ParsePrefix(a.Addresses[0])
	if err != nil {
		return ""
	}
	return p.Addr().String()
}
