package toolkit

import (
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

// Synthetic adapter/ranges only; no probes are sent.
func TestApprovedScanPreservesCIDRAndPinsDefaultRange(t *testing.T) {
	adapters := []platform.Adapter{{Name: "Synthetic Ethernet", UnicastSuitable: true, Addresses: []string{"192.168.10.12/24"}}}
	for _, tc := range []struct{ name, cidr, start, end string }{
		{"narrower subnet", "192.168.10.128/25", "", ""},
		{"default subnet", "", "", ""},
		{"explicit range", "", "192.168.10.40", "192.168.10.42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := ipscan.Options{Interface: adapters[0].Name, CIDR: tc.cidr, Start: tc.start, End: tc.end, ICMP: true, TimeoutMS: 100, Workers: 2, Rate: 100}
			approved, err := ipscan.Prepare(o, adapters)
			if err != nil {
				t.Fatal(err)
			}
			pinned := pinScanScope(o, approved)
			worker, err := ipscan.Prepare(pinned, adapters)
			if err != nil || worker.CIDR != approved.CIDR || worker.Start != approved.Start || worker.End != approved.End {
				t.Fatal("approved prefix or range changed", approved, worker, err)
			}
			changed := []platform.Adapter{{Name: adapters[0].Name, UnicastSuitable: true, Addresses: []string{"192.168.20.12/24"}}}
			worker, err = ipscan.Prepare(pinned, changed)
			if err != nil || worker.Start != approved.Start || worker.End != approved.End || !worker.NeedsConfirmation {
				t.Fatal("adapter change silently selected a new range", worker, err)
			}
		})
	}
}
