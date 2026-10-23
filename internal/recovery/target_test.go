package recovery

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

// All addresses, adapter names and routes below are synthetic. No camera
// connections or operating-system route changes are made by these tests.
func targetFixtures() (devices.Device, []platform.Adapter) {
	port := 80
	d := devices.Device{IPv4: "192.0.2.40", HTTPPort: &port, Interfaces: []devices.Interface{{Name: "Wi-Fi Fixture"}, {Name: "Ethernet Fixture"}}}
	all := []platform.Adapter{
		{Name: "Wi-Fi Fixture", Suitable: true, UnicastSuitable: true, Addresses: []string{"198.51.100.10/24"}},
		{Name: "Ethernet Fixture", Suitable: true, UnicastSuitable: true, Addresses: []string{"203.0.113.10/24", "192.0.2.10/24"}},
		{Name: "VPN Fixture", UnicastSuitable: true, Addresses: []string{"198.51.100.20/32"}},
	}
	return d, all
}

func TestAutomaticRecoveryUsesOSRoute(t *testing.T) {
	for _, tc := range []struct{ name, route, adapter, source string }{
		{"gateway", "198.51.100.10", "Wi-Fi Fixture", "198.51.100.10"},
		{"secondary address", "192.0.2.10", "Ethernet Fixture", "192.0.2.10"},
		{"VPN not used for multicast discovery", "198.51.100.20", "VPN Fixture", "198.51.100.20"},
		{"no route hint prefers direct secondary", "", "Ethernet Fixture", "192.0.2.10"},
		{"stale route hint", "203.0.113.200", "Ethernet Fixture", "192.0.2.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, all := targetFixtures()
			target, err := targetForDevice(d, "", "Wi-Fi Fixture", all, tc.route)
			if err != nil || target.Interface != tc.adapter || target.Source != tc.source || target.Address != d.IPv4 {
				t.Fatal(target, err)
			}
		})
	}
}

func TestExplicitRecoveryAdapterAllowsRoutedAddress(t *testing.T) {
	d, all := targetFixtures()
	// An explicit adapter wins even when the OS would use a different one.
	target, err := targetForDevice(d, "Wi-Fi Fixture", "", all, "192.0.2.10")
	if err != nil || target.Interface != "Wi-Fi Fixture" || target.Source != "198.51.100.10" {
		t.Fatal(target, err)
	}
	if _, err := targetForDevice(d, "unobserved", "", all, "192.0.2.10"); err == nil {
		t.Fatal("unobserved explicit override accepted")
	}
	// A failed route query does not establish that HTTP cannot connect.
	d.IPv4 = "203.0.113.40"
	d.Interfaces = d.Interfaces[:1]
	target, err = targetForDevice(d, "", "", all[:1], "")
	if err != nil || target.Source != "198.51.100.10" {
		t.Fatal(target, err)
	}
	// CLI may explicitly select a unicast-only adapter.
	target, _, err = targetOnAdapter(d.IPv4, 8080, "VPN Fixture", all, "")
	if err != nil || target.Source != "198.51.100.20" || target.Port != 8080 {
		t.Fatal(target, err)
	}
}

func TestRecoveryRejectsInvalidTargetsAndUnavailableAdapters(t *testing.T) {
	d, all := targetFixtures()
	for _, bad := range []string{"192.0.2.0", "192.0.2.255", "192.0.2.10", "127.0.0.1", "239.1.1.1", "0.0.0.0", "255.255.255.255", "2001:db8::1", "not-an-ip"} {
		if _, _, err := targetOnAdapter(bad, 80, "Ethernet Fixture", all, ""); err == nil {
			t.Fatal("invalid target accepted", bad)
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if _, _, err := targetOnAdapter(d.IPv4, port, "Ethernet Fixture", all, ""); err == nil {
			t.Fatal("invalid port accepted")
		}
	}
	for i := range all {
		all[i].UnicastSuitable = false
	}
	if _, err := targetForDevice(d, "", "", all, "192.0.2.10"); err == nil {
		t.Fatal("unavailable route source accepted")
	}
	all[1].UnicastSuitable = true
	all[1].Addresses = []string{"malformed", "::1/128", "0.0.0.0/24"}
	if _, err := targetForDevice(d, "", "", all, ""); err == nil {
		t.Fatal("invalid source accepted")
	}
	d.HTTPPort = nil
	all[1].Addresses = []string{"192.0.2.10/24"}
	if target, err := ForDevice(d, "", "", all); err != nil || target.Port != 80 {
		t.Fatal("unknown port should allow web detection", target, err)
	}
}

func TestRecoveryPointToPointSource(t *testing.T) {
	all := []platform.Adapter{{Name: "Point-to-point fixture", UnicastSuitable: true, Addresses: []string{"192.0.2.10/31"}}}
	target, _, err := targetOnAdapter("192.0.2.11", 80, all[0].Name, all, "")
	if err != nil || target.Source != "192.0.2.10" {
		t.Fatal(target, err)
	}
}

func TestRoutedRecoveryChecksActualHTTPResponse(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		code        int
		err         error
		calls       int
	}{
		{"ready", "ready", 200, nil, 3},
		{"reachable but unsupported", "unsupported", 401, nil, 1},
		{"connection failure", "unavailable", 0, errors.New("UNTRUSTED-SECRET"), 1},
		{"connection timeout", "unavailable", 0, &net.DNSError{IsTimeout: true, Err: "UNTRUSTED-SECRET"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, all := targetFixtures()
			target, err := targetForDevice(d, "", "", all, "198.51.100.10")
			if err != nil {
				t.Fatal(err)
			}
			f := &fixture{t: t, change: func(r *http.Request) (*http.Response, error, bool) {
				if r.URL.Host != "192.0.2.40:80" || r.Method != http.MethodGet {
					t.Fatal("unexpected request", r.Method, r.URL.Host)
				}
				if tc.err != nil {
					return nil, tc.err, true
				}
				if tc.code != 200 {
					return response(tc.code, ""), nil, true
				}
				return nil, nil, false
			}}
			status, err := f.client().Check(context.Background(), target)
			if status.State != tc.state || (err != nil) != (tc.err != nil) || len(f.paths) != tc.calls || f.puts != 0 || f.posts != 0 {
				t.Fatal(status, err, f.paths)
			}
			if tc.err != nil && (!strings.Contains(status.Message, target.Source) || !strings.Contains(status.Message, "192.0.2.40:80") || strings.Contains(status.Message, "UNTRUSTED-SECRET")) {
				t.Fatal("connection diagnostics missing or unsafe", status.Message)
			}
			if tc.name == "connection timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("timeout not preserved", err)
			}
		})
	}
}
