package ipscan

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/platform"
)

var fixtureAdapters = []platform.Adapter{{Name: "Synthetic Ethernet", UnicastSuitable: true, Addresses: []string{"192.168.10.12/24"}, Gateway: "192.168.10.1"}}

func fixtureOptions() Options {
	return Options{Interface: "Synthetic Ethernet", CIDR: "192.168.10.0/24", ICMP: true, TimeoutMS: 100, Workers: 4, Rate: 200}
}
func TestRangeBoundariesAndConfirmation(t *testing.T) {
	for _, tc := range []struct {
		cidr        string
		count       int
		first, last string
	}{{"192.168.10.0/24", 256, "192.168.10.0", "192.168.10.255"}, {"192.168.10.128/25", 128, "192.168.10.128", "192.168.10.255"}, {"192.168.10.0/31", 2, "192.168.10.0", "192.168.10.1"}} {
		o := fixtureOptions()
		o.CIDR = tc.cidr
		s, e := Prepare(o, fixtureAdapters)
		if e != nil || s.Total != tc.count || s.Start != tc.first || s.End != tc.last {
			t.Fatal(s, e)
		}
		if len(Addresses(s)) != tc.count {
			t.Fatal("truncated range")
		}
		if tc.count > 2 && (protected(s.Start, s, nil) != "network address" || protected(s.End, s, nil) != "broadcast address") {
			t.Fatal("wrong subnet boundaries")
		}
		if tc.count == 2 && protected(s.Start, s, nil) != "" {
			t.Fatal("/31 host excluded")
		}
	}
	for _, cidr := range []string{"0.0.0.0/0", "127.0.0.0/24", "224.0.0.0/24", "10.0.0.0/8"} {
		o := fixtureOptions()
		o.CIDR = cidr
		if _, e := Prepare(o, fixtureAdapters); e == nil {
			t.Fatal("bad scope accepted", cidr)
		}
	}
	o := fixtureOptions()
	o.CIDR = "192.0.2.0/24"
	s, e := Prepare(o, fixtureAdapters)
	if e != nil || !s.NeedsConfirmation || !s.Routed {
		t.Fatal(s, e)
	}
}
func TestScanPreservesMultipleEvidenceAndNoResponse(t *testing.T) {
	o := fixtureOptions()
	o.CIDR = ""
	o.Start = "192.168.10.40"
	o.End = "192.168.10.43"
	e := &Engine{Interfaces: func() ([]platform.Adapter, error) { return fixtureAdapters, nil }, Neighbours: func() ([]Neighbour, error) {
		return []Neighbour{{IP: o.Start, MAC: "02:00:00:00:00:02", Interface: o.Interface, Source: "synthetic cache"}}, nil
	}, Ping: func(_ context.Context, ip, _ string, _ time.Duration) Evidence {
		state := "no-response"
		if ip == "192.168.10.41" {
			state = "responded"
		}
		return Evidence{Probe: "icmp", State: state, At: stamp()}
	}}
	known := []Known{{IP: o.Start, MAC: "02:00:00:00:00:01", ID: "synthetic", Source: "saved"}}
	result, err := e.Scan(context.Background(), o, known, map[string]string{"192.168.10.42": "synthetic reserved"}, nil)
	if err != nil || result.Completed != 4 {
		t.Fatal(err, result)
	}
	got := []string{}
	for _, h := range result.Hosts {
		got = append(got, h.Status)
	}
	if !reflect.DeepEqual(got, []string{"conflict", "responding", "reserved", "no-response"}) {
		t.Fatal(got)
	}
	if !result.Hosts[0].Known || result.Hosts[0].Responding {
		t.Fatal("cached entry became a new response")
	}
}
func TestCancellationStopsWorkersAndRetainsEveryAddress(t *testing.T) {
	o := fixtureOptions()
	started := make(chan struct{}, 4)
	e := &Engine{Interfaces: func() ([]platform.Adapter, error) { return fixtureAdapters, nil }, Neighbours: func() ([]Neighbour, error) { return nil, errors.New("synthetic unsupported") }, Ping: func(ctx context.Context, _, _ string, _ time.Duration) Evidence {
		started <- struct{}{}
		<-ctx.Done()
		return Evidence{State: "no-response"}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { r, _ := e.Scan(ctx, o, nil, nil, nil); done <- r }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("no probe started")
	}
	cancel()
	select {
	case r := <-done:
		if !r.Cancelled || len(r.Hosts) != 256 || r.Completed >= 256 {
			t.Fatal("lost partial results")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not cancel")
	}
}

func TestPingSilentHostWithTCPResponseIsOccupied(t *testing.T) {
	o := fixtureOptions()
	o.CIDR, o.Start, o.End = "", "192.168.10.40", "192.168.10.40"
	o.TCPPorts = []int{443}
	e := &Engine{
		Interfaces: func() ([]platform.Adapter, error) { return fixtureAdapters, nil },
		Neighbours: func() ([]Neighbour, error) { return nil, nil },
		Ping: func(context.Context, string, string, time.Duration) Evidence {
			return Evidence{Probe: "icmp", State: "no-response", At: stamp()}
		},
		TCP: func(context.Context, string, string, int, time.Duration) Evidence {
			return Evidence{Probe: "tcp:443", State: "responded", At: stamp()}
		},
	}
	r, err := e.Scan(context.Background(), o, nil, nil, nil)
	if err != nil || len(r.Hosts) != 1 || !r.Hosts[0].Responding || r.Hosts[0].Status != "responding" || len(r.Hosts[0].Evidence) != 2 {
		t.Fatalf("ping silence hid TCP evidence: %+v %v", r, err)
	}
}

func TestTCPProbeLoopbackAndCancellation(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	e := tcp(context.Background(), "127.0.0.1", "127.0.0.1", port, time.Second)
	if e.State != "responded" || e.LatencyMS == nil || e.At == "" {
		t.Fatalf("loopback listener not detected: %+v", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e = tcp(ctx, "127.0.0.1", "127.0.0.1", port, time.Second)
	if e.State == "responded" {
		t.Fatal("cancelled TCP connection reported a response")
	}
}

func TestPaginationRetainsEveryAddressAndOrdersTies(t *testing.T) {
	scope, err := Prepare(fixtureOptions(), fixtureAdapters)
	if err != nil {
		t.Fatal(err)
	}
	r := Result{Scope: scope}
	for _, ip := range Addresses(scope) {
		r.Hosts = append(r.Hosts, Host{IP: ip, Status: "known", Known: true})
	}
	var all []Host
	for offset := 0; offset < scope.Total; offset += 100 {
		p := Paginate(r, offset, 100, "known", "hostname", true)
		if p.Matching != 256 || p.Counts["known"] != 256 {
			t.Fatal("page totals changed", p.Matching, p.Counts)
		}
		all = append(all, p.Hosts...)
	}
	if len(all) != 256 {
		t.Fatal("pagination lost addresses", len(all))
	}
	for i, h := range all {
		if h.IP != r.Hosts[255-i].IP {
			t.Fatal("equal sort values were not ordered by numeric IP", i, h.IP)
		}
	}
	if r.Hosts[0].IP != scope.Start || len(Paginate(r, 500, 100, "", "ip", false).Hosts) != 0 {
		t.Fatal("pagination mutated the original or produced an invalid trailing page")
	}
}

func TestRejectsAmbiguousRangeAndNoActiveProbes(t *testing.T) {
	o := fixtureOptions()
	o.Start, o.End = "192.168.10.40", "192.168.10.41"
	if _, err := Prepare(o, fixtureAdapters); err == nil {
		t.Fatal("both CIDR and start/end were accepted")
	}
	o = fixtureOptions()
	o.ICMP = false
	if _, err := Prepare(o, fixtureAdapters); err == nil {
		t.Fatal("scan without an active probe was accepted")
	}
}
