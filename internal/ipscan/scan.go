// Package ipscan performs bounded, evidence-based IPv4 investigations.
package ipscan

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Evidence struct {
	Probe     string   `json:"probe"`
	State     string   `json:"state"`
	Detail    string   `json:"detail"`
	At        string   `json:"at"`
	LatencyMS *float64 `json:"latencyMS"`
}
type Host struct {
	IP         string     `json:"ip"`
	Status     string     `json:"status"`
	Responding bool       `json:"responding"`
	Known      bool       `json:"known"`
	Conflict   bool       `json:"conflict"`
	Reserved   string     `json:"reserved"`
	Scanned    bool       `json:"scanned"`
	MACs       []string   `json:"macs"`
	Matches    []string   `json:"matches"`
	Hostname   string     `json:"hostname"`
	Evidence   []Evidence `json:"evidence"`
}
type Result struct {
	Scope     Scope    `json:"scope"`
	Hosts     []Host   `json:"hosts"`
	Started   string   `json:"started"`
	Finished  string   `json:"finished"`
	Completed int      `json:"completed"`
	Warnings  []string `json:"warnings"`
	Cancelled bool     `json:"cancelled"`
}
type Known struct {
	IP     string
	MAC    string
	ID     string
	Source string
	At     string
}
type Neighbour struct {
	IP        string
	MAC       string
	Interface string
	Reachable bool
	Source    string
}
type ProbeFunc func(context.Context, string, string, time.Duration) Evidence
type Engine struct {
	Interfaces func() ([]platform.Adapter, error)
	Neighbours func() ([]Neighbour, error)
	Ping       ProbeFunc
	TCP        func(context.Context, string, string, int, time.Duration) Evidence
}

func New() *Engine {
	return &Engine{Interfaces: platform.Interfaces, Neighbours: neighbours, Ping: ping, TCP: tcp}
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (e *Engine) Scan(ctx context.Context, o Options, known []Known, reserved map[string]string, progress func(Host, int, int)) (Result, error) {
	r := Result{Started: stamp(), Hosts: []Host{}, Warnings: []string{}}
	adapters, err := e.Interfaces()
	if err != nil {
		return r, errors.New("could not enumerate adapters")
	}
	r.Scope, err = Prepare(o, adapters)
	if err != nil {
		return r, err
	}
	if r.Scope.NeedsConfirmation && !o.ConfirmScope {
		return r, errors.New("confirm the exact public/routed/large scan scope before starting")
	}
	neighbours, neighbourErr := e.Neighbours()
	if neighbourErr != nil {
		r.Warnings = append(r.Warnings, "Neighbour table unavailable; ICMP/TCP and saved evidence remain usable.")
	}
	for _, ip := range Addresses(r.Scope) {
		h := Host{IP: ip, Status: "unscanned", MACs: []string{}, Matches: []string{}, Evidence: []Evidence{}, Reserved: protected(ip, r.Scope, reserved)}
		for _, k := range known {
			if k.IP != ip {
				continue
			}
			h.Known = true
			h.Matches = append(h.Matches, k.ID)
			addMAC(&h, k.MAC)
			h.Evidence = append(h.Evidence, Evidence{Probe: k.Source, State: "known", Detail: "Recorded assignment/observation; does not prove current reachability", At: k.At})
		}
		for _, n := range neighbours {
			if n.IP == ip && n.Interface == o.Interface {
				addMAC(&h, n.MAC)
				h.Known = true
				h.Evidence = append(h.Evidence, Evidence{Probe: n.Source, State: "cached", Detail: "Neighbour cache; not a new probe response", At: stamp()})
			}
		}
		classify(&h)
		r.Hosts = append(r.Hosts, h)
	}
	jobs := make(chan int)
	type answer struct {
		index int
		host  Host
	}
	answers := make(chan answer, o.Workers)
	var wg sync.WaitGroup
	for range o.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				h := r.Hosts[i]
				if h.Reserved == "" {
					if o.ICMP {
						h.Evidence = append(h.Evidence, e.Ping(ctx, h.IP, r.Scope.Source, time.Duration(o.TimeoutMS)*time.Millisecond))
					}
					for _, p := range o.TCPPorts {
						if ctx.Err() != nil {
							break
						}
						h.Evidence = append(h.Evidence, e.TCP(ctx, h.IP, r.Scope.Source, p, time.Duration(o.TimeoutMS)*time.Millisecond))
					}
				}
				h.Scanned = ctx.Err() == nil
				classify(&h)
				answers <- answer{i, h}
			}
		}()
	}
	go func() {
		defer close(jobs)
		tick := time.NewTicker(time.Second / time.Duration(o.Rate))
		defer tick.Stop()
		for i := range r.Hosts {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(answers) }()
	warnings := map[string]bool{}
	for a := range answers {
		r.Hosts[a.index] = a.host
		if a.host.Scanned {
			r.Completed++
		}
		for _, ev := range a.host.Evidence {
			if ev.State == "unsupported" {
				warnings[ev.Detail] = true
			}
		}
		if progress != nil {
			progress(a.host, r.Completed, r.Scope.Total)
		}
	}
	for w := range warnings {
		r.Warnings = append(r.Warnings, w)
	}
	sort.Strings(r.Warnings)
	r.Finished = stamp()
	r.Cancelled = ctx.Err() != nil
	return r, ctx.Err()
}
func addMAC(h *Host, mac string) {
	mac = devices.NormalizeMAC(mac)
	if mac == "" {
		return
	}
	for _, m := range h.MACs {
		if m == mac {
			return
		}
	}
	h.MACs = append(h.MACs, mac)
}
func classify(h *Host) {
	for _, e := range h.Evidence {
		if e.State == "responded" {
			h.Responding = true
		}
	}
	h.Conflict = len(h.MACs) > 1
	switch {
	case h.Conflict:
		h.Status = "conflict"
	case h.Reserved != "":
		h.Status = "reserved"
	case h.Responding:
		h.Status = "responding"
	case h.Known:
		h.Status = "known"
	case !h.Scanned:
		h.Status = "unscanned"
	default:
		h.Status = "no-response"
	}
}
func tcp(ctx context.Context, ip, source string, port int, timeout time.Duration) Evidence {
	ev := Evidence{Probe: "tcp:" + strconv.Itoa(port), At: stamp(), State: "no-response"}
	start := time.Now()
	dialer := net.Dialer{Timeout: timeout, LocalAddr: &net.TCPAddr{IP: net.ParseIP(source)}}
	c, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err == nil {
		c.Close()
		ms := float64(time.Since(start).Microseconds()) / 1000
		ev.State = "responded"
		ev.Detail = "TCP connection accepted"
		ev.LatencyMS = &ms
	} else {
		ev.Detail = "No accepted TCP connection; closed or filtered port is inconclusive"
	}
	return ev
}
func Resolve(ctx context.Context, ip string) ([]string, error) {
	a, e := netip.ParseAddr(ip)
	if e != nil || !usable(a) {
		return nil, errors.New("invalid IPv4 address")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, e := net.DefaultResolver.LookupAddr(ctx, ip)
	if e != nil {
		return nil, errors.New("reverse DNS unavailable")
	}
	return names, nil
}

type Page struct {
	Scope     Scope          `json:"scope"`
	Hosts     []Host         `json:"hosts"`
	Started   string         `json:"started"`
	Finished  string         `json:"finished"`
	Completed int            `json:"completed"`
	Warnings  []string       `json:"warnings"`
	Cancelled bool           `json:"cancelled"`
	Matching  int            `json:"matching"`
	Counts    map[string]int `json:"counts"`
}

func Paginate(r Result, offset, limit int, filter, sortBy string, descending bool) Page {
	if offset < 0 {
		offset = 0
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	hosts := []Host{}
	counts := map[string]int{}
	for _, h := range r.Hosts {
		counts[h.Status]++
		match := filter == "" || filter == h.Status
		if filter == "responding" {
			match = h.Responding
		}
		if filter == "known" {
			match = h.Known
		}
		if match {
			hosts = append(hosts, h)
		}
	}
	sort.SliceStable(hosts, func(i, j int) bool {
		a, b := hosts[i], hosts[j]
		ai, _ := netip.ParseAddr(a.IP)
		bi, _ := netip.ParseAddr(b.IP)
		order := ai.Compare(bi)
		switch sortBy {
		case "status":
			if a.Status != b.Status {
				order = strings.Compare(a.Status, b.Status)
			}
		case "hostname":
			if a.Hostname != b.Hostname {
				order = strings.Compare(a.Hostname, b.Hostname)
			}
		}
		if descending {
			return order > 0
		}
		return order < 0
	})
	matching := len(hosts)
	if offset > matching {
		offset = matching
	}
	end := offset + limit
	if end > matching {
		end = matching
	}
	return Page{r.Scope, hosts[offset:end], r.Started, r.Finished, r.Completed, append([]string{}, r.Warnings...), r.Cancelled, matching, counts}
}
