package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"time"
)

func (m *Manager) IPScope(o ipscan.Options) (ipscan.Scope, error) {
	adapters, e := platform.Interfaces()
	if e != nil {
		return ipscan.Scope{}, e
	}
	return ipscan.Prepare(o, adapters)
}
func (m *Manager) known(site string) ([]ipscan.Known, map[string]string, error) {
	known := []ipscan.Known{}
	reserved := map[string]string{}
	for _, d := range m.Controller.Snapshot().Devices {
		known = append(known, ipscan.Known{IP: d.IPv4, MAC: d.MAC, ID: d.ID, Source: "live-discovery", At: d.LastSeen.Format(time.RFC3339)})
	}
	if site != "" {
		v, e := m.Store.View(site)
		if e != nil {
			return nil, nil, e
		}
		for _, r := range v.Devices {
			if !r.Removed {
				known = append(known, ipscan.Known{IP: r.Device.IPv4, MAC: r.Device.MAC, ID: r.ID, Source: "saved-site", At: r.Updated})
			}
		}
		for _, r := range v.Reservations {
			reserved[r.IP] = r.Note
		}
	}
	return known, reserved, nil
}
func (m *Manager) StartIPScan(site string, o ipscan.Options) error {
	scope, e := m.IPScope(o)
	if e != nil {
		return e
	}
	if scope.NeedsConfirmation && !o.ConfirmScope {
		return errors.New("confirm the displayed scan scope")
	}
	known, reserved, e := m.known(site)
	if e != nil {
		return e
	}
	if e = m.Store.SaveScannerPreferences(o); e != nil {
		return e
	}
	o = pinScanScope(o, scope)
	return m.start("IP scan", site, func(ctx context.Context) error {
		initial := ipscan.Result{Scope: scope, Started: time.Now().UTC().Format(time.RFC3339), Hosts: []ipscan.Host{}, Warnings: []string{}}
		indices := map[string]int{}
		for _, ip := range ipscan.Addresses(scope) {
			indices[ip] = len(initial.Hosts)
			initial.Hosts = append(initial.Hosts, ipscan.Host{IP: ip, Status: "unscanned", Evidence: []ipscan.Evidence{}, Matches: []string{}, MACs: []string{}})
		}
		m.mu.Lock()
		m.ipResult = initial
		m.mu.Unlock()
		result, e := ipscan.New().Scan(ctx, o, known, reserved, func(h ipscan.Host, done, total int) {
			m.mu.Lock()
			if index, ok := indices[h.IP]; ok {
				m.ipResult.Hosts[index] = h
			}
			m.ipResult.Completed = done
			m.mu.Unlock()
			m.progress(done, total, "Investigating IPv4 addresses")
		})
		m.mu.Lock()
		m.ipResult = result
		m.mu.Unlock()
		return e
	})
}

// Keep the approved range fixed if adapter addresses change before the worker
// starts. Preserve an explicit CIDR's network/broadcast semantics, including
// narrower subnets such as a /25 on a host configured with a /24.
func pinScanScope(o ipscan.Options, scope ipscan.Scope) ipscan.Options {
	if o.CIDR != "" || o.Start == "" && o.End == "" {
		o.CIDR = scope.CIDR
		o.Start, o.End = "", ""
	} else {
		o.Start, o.End = scope.Start, scope.End
	}
	return o
}
func (m *Manager) IPResult() ipscan.Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.ipResult
	r.Hosts = append([]ipscan.Host{}, r.Hosts...)
	r.Warnings = append([]string{}, r.Warnings...)
	return r
}
func (m *Manager) ResetIPResult() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job.Running {
		return errors.New("stop the active operation before resetting")
	}
	m.ipResult = ipscan.Result{Hosts: []ipscan.Host{}, Warnings: []string{}}
	return nil
}
func (m *Manager) SaveIPScan(site string) (sites.Run, error) {
	m.mu.Lock()
	r := m.ipResult
	r.Hosts = append([]ipscan.Host{}, r.Hosts...)
	r.Warnings = append([]string{}, r.Warnings...)
	busy := m.job.Running
	m.mu.Unlock()
	if busy || r.Started == "" {
		return sites.Run{}, errors.New("finish or cancel a scan before saving it")
	}
	data, e := json.Marshal(r)
	if e != nil {
		return sites.Run{}, e
	}
	outcome := "completed"
	if r.Cancelled {
		outcome = "cancelled"
	}
	return m.Store.SaveRun(sites.Run{SiteID: site, Kind: "ip-scan", Scope: r.Scope.Start + "–" + r.Scope.End + " via " + r.Scope.Interface, Outcome: outcome, Data: string(data)})
}
func (m *Manager) IPPage(offset, limit int, filter, sortBy string, descending bool) ipscan.Page {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ipscan.Paginate(m.ipResult, offset, limit, filter, sortBy, descending)
}
func (m *Manager) ReprobeIP(site, ip string, ports []int) error {
	m.mu.Lock()
	scope := m.ipResult.Scope
	found := false
	for _, h := range m.ipResult.Hosts {
		if h.IP == ip {
			found = true
			break
		}
	}
	m.mu.Unlock()
	if !found {
		return errors.New("select an address from the approved scan")
	}
	known, reserved, e := m.known(site)
	if e != nil {
		return e
	}
	o := ipscan.Options{Interface: scope.Interface, Start: ip, End: ip, ICMP: true, TCPPorts: ports, Workers: 1, Rate: 10, TimeoutMS: 700, ConfirmScope: true}
	return m.start("IP recheck", site, func(ctx context.Context) error {
		r, e := ipscan.New().Scan(ctx, o, known, reserved, nil)
		if e != nil {
			return e
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, h := range m.ipResult.Hosts {
			if h.IP == ip && len(r.Hosts) == 1 {
				m.ipResult.Hosts[i] = r.Hosts[0]
				break
			}
		}
		return nil
	})
}
func (m *Manager) ResolveIP(ip string) ([]string, error) {
	names, e := ipscan.Resolve(context.Background(), ip)
	if e != nil {
		return nil, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, h := range m.ipResult.Hosts {
		if h.IP == ip && len(names) > 0 {
			m.ipResult.Hosts[i].Hostname = names[0]
		}
	}
	return names, nil
}
