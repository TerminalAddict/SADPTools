package ipscan

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"net/netip"
)

const MaxAddresses = 65536

type Options struct {
	Interface    string `json:"interface"`
	CIDR         string `json:"cidr"`
	Start        string `json:"start"`
	End          string `json:"end"`
	TimeoutMS    int    `json:"timeoutMS"`
	Workers      int    `json:"workers"`
	Rate         int    `json:"rate"`
	TCPPorts     []int  `json:"tcpPorts"`
	ICMP         bool   `json:"icmp"`
	ConfirmScope bool   `json:"confirmScope"`
}
type Scope struct {
	Interface         string   `json:"interface"`
	Source            string   `json:"source"`
	CIDR              string   `json:"cidr"`
	Start             string   `json:"start"`
	End               string   `json:"end"`
	Total             int      `json:"total"`
	Gateway           string   `json:"gateway"`
	Local             []string `json:"local"`
	Routed            bool     `json:"routed"`
	Public            bool     `json:"public"`
	NeedsConfirmation bool     `json:"needsConfirmation"`
}

func number(a netip.Addr) uint32 { v := a.As4(); return binary.BigEndian.Uint32(v[:]) }
func address(n uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return netip.AddrFrom4(b)
}
func usable(a netip.Addr) bool {
	return a.Is4() && !a.IsUnspecified() && !a.IsLoopback() && !a.IsMulticast() && a.String() != "255.255.255.255"
}
func Prepare(o Options, adapters []platform.Adapter) (Scope, error) {
	s := Scope{Interface: o.Interface, Local: []string{}}
	if o.CIDR != "" && (o.Start != "" || o.End != "") {
		return s, errors.New("choose CIDR or start/end, not both")
	}
	if !o.ICMP && len(o.TCPPorts) == 0 {
		return s, errors.New("enable ICMP or at least one TCP probe")
	}
	var adapter *platform.Adapter
	for _, a := range adapters {
		if a.Name == o.Interface && a.UnicastSuitable {
			v := a
			adapter = &v
		}
		for _, v := range a.Addresses {
			if p, e := netip.ParsePrefix(v); e == nil {
				s.Local = append(s.Local, p.Addr().String())
			}
		}
	}
	if adapter == nil {
		return s, errors.New("select an available IPv4 adapter")
	}
	s.Gateway = adapter.Gateway
	var prefix netip.Prefix
	var start, end netip.Addr
	var err error
	if o.CIDR != "" {
		prefix, err = netip.ParsePrefix(o.CIDR)
		if err != nil || !prefix.Addr().Is4() {
			return s, errors.New("enter an IPv4 CIDR")
		}
		prefix = prefix.Masked()
		start = prefix.Addr()
		count := uint64(1) << uint(32-prefix.Bits())
		if count > MaxAddresses {
			return s, fmt.Errorf("scan at most %d addresses at once (/16 or smaller)", MaxAddresses)
		}
		end = address(number(start) + uint32(count-1))
		s.CIDR = prefix.String()
	} else if o.Start != "" || o.End != "" {
		start, err = netip.ParseAddr(o.Start)
		if err != nil || !start.Is4() {
			return s, errors.New("invalid start IPv4 address")
		}
		end, err = netip.ParseAddr(o.End)
		if err != nil || !end.Is4() {
			return s, errors.New("invalid end IPv4 address")
		}
	} else {
		if len(adapter.Addresses) == 0 {
			return s, errors.New("adapter has no IPv4 subnet")
		}
		o.CIDR = adapter.Addresses[0]
		return Prepare(o, adapters)
	}
	if number(end) < number(start) || uint64(number(end))-uint64(number(start))+1 > MaxAddresses {
		return s, errors.New("invalid or excessively large address range")
	}
	// The address iterator includes subnet boundaries for display, but never probes
	// unspecified, multicast, loopback, broadcast or reserved/local entries.
	for _, a := range []netip.Addr{start, end} {
		if a.IsLoopback() || a.IsMulticast() || a.IsUnspecified() || a.String() == "255.255.255.255" {
			return s, errors.New("loopback, unspecified and multicast ranges cannot be scanned")
		}
	}
	s.Start = start.String()
	s.End = end.String()
	s.Total = int(uint64(number(end)) - uint64(number(start)) + 1)
	s.Routed = true
	for _, raw := range adapter.Addresses {
		p, e := netip.ParsePrefix(raw)
		if e != nil {
			continue
		}
		if s.Source == "" {
			s.Source = p.Addr().String()
		}
		if p.Contains(start) && p.Contains(end) {
			s.Routed = false
			s.Source = p.Addr().String()
			if s.CIDR == "" {
				s.CIDR = p.Masked().String()
			}
			break
		}
	}
	s.Public = !start.IsPrivate() || !end.IsPrivate()
	s.NeedsConfirmation = s.Routed || s.Public || s.Total > 1024
	if o.TimeoutMS < 100 || o.TimeoutMS > 3000 || o.Workers < 1 || o.Workers > 64 || o.Rate < 1 || o.Rate > 200 || len(o.TCPPorts) > 8 {
		return s, errors.New("probe timeout 100–3000 ms, workers 1–64, rate 1–200 hosts/s and at most 8 TCP ports are supported")
	}
	for _, p := range o.TCPPorts {
		if p < 1 || p > 65535 {
			return s, errors.New("TCP ports must be 1–65535")
		}
	}
	return s, nil
}
func Addresses(s Scope) []string {
	start, _ := netip.ParseAddr(s.Start)
	out := make([]string, s.Total)
	for i := range out {
		out[i] = address(number(start) + uint32(i)).String()
	}
	return out
}
func protected(ip string, s Scope, reserved map[string]string) string {
	for _, a := range s.Local {
		if ip == a {
			return "local interface"
		}
	}
	if ip == s.Gateway {
		return "gateway"
	}
	if note, ok := reserved[ip]; ok {
		return "reserved: " + note
	}
	p, e := netip.ParsePrefix(s.CIDR)
	a, err := netip.ParseAddr(ip)
	if err != nil || !usable(a) {
		return "non-host address"
	}
	if e == nil && p.Bits() < 31 {
		first := p.Masked().Addr()
		last := address(number(first) + uint32((uint64(1)<<uint(32-p.Bits()))-1))
		if a == first {
			return "network address"
		}
		if a == last {
			return "broadcast address"
		}
	}
	return ""
}
