package platform

import (
	"strings"
	"testing"
)

func TestDarwinRouteFixtures(t *testing.T) {
	// Synthetic route(8) output and documentation-only addresses.
	for _, tc := range []struct{ text, gateway, prefix string }{
		{"destination: default\ngateway: 192.0.2.1\ninterface: en7", "192.0.2.1", "0.0.0.0/0"},
		{"destination: 192.0.2.0\nmask: 255.255.255.0\ngateway: link#8\ninterface: en7", "", "192.0.2.0/24"},
		{"destination: 192.0.2.5\nflags: <UP,HOST,DONE,LLINFO>\ninterface: en7", "", "192.0.2.5/32"},
		{"destination: 192.0.2.5\ninterface: en7", "", ""},
		{"destination: 192.0.2.5\nmask: 255.0.255.0\ninterface: en7", "", ""},
	} {
		got, err := parseDarwinRoute([]byte(tc.text), "192.0.2.2", "en7")
		if err != nil || got.Source != "192.0.2.2" || got.Gateway != tc.gateway || got.Prefix != tc.prefix {
			t.Fatalf("%+v %v", got, err)
		}
	}
	for _, data := range []string{"", "interface: en8", "interface: en7\ninterface: en7", strings.Repeat("x", 65537)} {
		if _, err := parseDarwinRoute([]byte(data), "192.0.2.2", "en7"); err == nil {
			t.Fatal("accepted invalid route output")
		}
	}
}
