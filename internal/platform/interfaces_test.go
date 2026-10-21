package platform

import (
	"errors"
	"net"
	"testing"
)

func TestInterfaceFiltering(t *testing.T) {
	ipv4 := &net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)}
	ipv6 := &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)}
	good := net.FlagUp | net.FlagMulticast
	for _, tc := range []struct {
		name  string
		flags net.Flags
		addrs []net.Addr
		err   error
		want  bool
	}{
		{"Ethernet 2", good, []net.Addr{ipv4, ipv6}, nil, true},
		{"down", net.FlagMulticast, []net.Addr{ipv4}, nil, false},
		{"loop", good | net.FlagLoopback, []net.Addr{ipv4}, nil, false},
		{"no multicast", net.FlagUp, []net.Addr{ipv4}, nil, false},
		{"v6 only", good, []net.Addr{ipv6}, nil, false},
		{"address failure", good, nil, errors.New("gone"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := Inspect(net.Interface{Index: 2, Name: tc.name, Flags: tc.flags}, tc.addrs, tc.err)
			if a.Suitable != tc.want {
				t.Fatal(a)
			}
			if a.Suitable && len(a.Addresses) != 1 {
				t.Fatal(a)
			}
			if !a.Suitable && a.Reason == "" {
				t.Fatal("missing reason")
			}
		})
	}
}
func TestSelection(t *testing.T) {
	all := []Adapter{{Name: "Ethernet 2", Suitable: true}, {Name: "disconnected", Reason: "down"}}
	if selected, err := Select(all, ""); err != nil || len(selected) != 1 {
		t.Fatal(selected, err)
	}
	for _, name := range []string{"unknown", "disconnected"} {
		if _, err := Select(all, name); err == nil {
			t.Fatal(name)
		}
	}
}
