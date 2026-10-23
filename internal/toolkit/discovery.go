package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/multidiscovery"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"log/slog"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SiteScope struct {
	SiteID         string   `json:"siteID"`
	SiteName       string   `json:"siteName"`
	Interface      string   `json:"interface"`
	Addresses      []string `json:"addresses"`
	Networks       []string `json:"networks"`
	Protocols      []string `json:"protocols"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	Warning        string   `json:"warning"`
	Token          string   `json:"token"`
}

func PrepareSiteScope(site sites.Site, all []platform.Adapter, seconds int) (SiteScope, error) {
	out := SiteScope{}
	if site.Archived {
		return out, errors.New("unarchive this site before scanning")
	}
	if site.Interface == "" || len(site.Networks) == 0 || len(site.Protocols) == 0 {
		return out, errors.New("edit this site to select an adapter, approved IPv4 networks and discovery protocols")
	}
	if e := multidiscovery.Validate(site.Protocols); e != nil {
		return out, e
	}
	selected, e := platform.Select(all, site.Interface)
	if e != nil {
		return out, e
	}
	if seconds < 1 || seconds > 300 {
		return out, errors.New("timeout must be 1–300 seconds")
	}
	for _, raw := range site.Networks {
		p, e := netip.ParsePrefix(raw)
		if e != nil || !p.Addr().Is4() || p.Addr().IsMulticast() || p.Addr().IsLoopback() {
			return out, errors.New("invalid approved site network")
		}
	}
	out = SiteScope{SiteID: site.ID, SiteName: site.Name, Interface: site.Interface, Addresses: selected[0].Addresses, Networks: site.Networks, Protocols: site.Protocols, TimeoutSeconds: seconds, Warning: "Multicast discovery reaches the selected adapter's local Layer 2 network, including other subnets on that link. Only replies in the approved site networks enter this scan's review. Routed cameras may not answer multicast; not seen does not mean removed."}
	b, _ := json.Marshal(out)
	hash := sha256.Sum256(b)
	out.Token = hex.EncodeToString(hash[:])
	return out, nil
}
func (m *Manager) SiteScope(site string) (SiteScope, error) {
	v, e := m.Store.View(site)
	if e != nil {
		return SiteScope{}, e
	}
	adapters, e := platform.Interfaces()
	if e != nil {
		return SiteScope{}, e
	}
	return PrepareSiteScope(v.Site, adapters, m.Controller.Settings().TimeoutSeconds)
}
func inNetworks(ip string, networks []string) bool {
	a, e := netip.ParseAddr(ip)
	if e != nil {
		return false
	}
	for _, n := range networks {
		p, e := netip.ParsePrefix(n)
		if e == nil && p.Contains(a) {
			return true
		}
	}
	return false
}
func (m *Manager) setRun(id string) { m.mu.Lock(); m.job.RunID = id; m.mu.Unlock() }
func (m *Manager) StartSiteRescan(site, scopeToken string) error {
	scope, e := m.SiteScope(site)
	if e != nil {
		return e
	}
	if scopeToken == "" || scope.Token != scopeToken {
		return errors.New("site or adapter scope changed; review and approve the current scope")
	}
	return m.start("Site discovery", site, func(ctx context.Context) error {
		inventory := devices.NewInventory()
		var countMu sync.Mutex
		seen := map[string]bool{}
		excluded := 0
		engine := multidiscovery.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
		result, scanErr := engine.Scan(ctx, discovery.Options{Interface: scope.Interface, Timeout: time.Duration(scope.TimeoutSeconds) * time.Second, Protocols: scope.Protocols}, func(d devices.Device) {
			if !inNetworks(d.IPv4, scope.Networks) {
				countMu.Lock()
				excluded++
				countMu.Unlock()
				return
			}
			inventory.Upsert(d, time.Now())
			m.Controller.Observe(d)
			countMu.Lock()
			seen[d.ID] = true
			count := len(seen)
			countMu.Unlock()
			m.progress(count, 0, strconv.Itoa(count)+" device(s) observed; saved inventory is unchanged")
		})
		data, _ := json.Marshal(inventory.Snapshot())
		outcome := "completed"
		if ctx.Err() != nil {
			outcome = "cancelled"
		} else if scanErr != nil {
			outcome = "incomplete"
		}
		description := strings.Join(scope.Networks, ", ") + " via " + scope.Interface + " [" + strings.Join(scope.Protocols, ", ") + "]; " + strconv.Itoa(excluded) + " out-of-scope replies ignored"
		if len(result.Warnings) > 0 {
			description += "; " + strings.Join(result.Warnings, "; ")
		}
		run, e := m.Store.SaveRun(sites.Run{SiteID: site, Kind: "discovery", Scope: description, Outcome: outcome, Data: string(data)})
		if e != nil {
			return e
		}
		m.setRun(run.ID)
		m.progress(len(seen), len(seen), "Scan saved. Review differences before accepting inventory changes.")
		return scanErr
	})
}
func (m *Manager) onvifDevice(site, id string) (devices.Device, error) {
	for _, d := range m.Controller.Snapshot().Devices {
		if d.ID == id && !d.IdentityConflict && len(d.XAddrs) > 0 {
			return d, nil
		}
	}
	if site != "" {
		rows, e := m.Store.Records(site)
		if e != nil {
			return devices.Device{}, e
		}
		for _, r := range rows {
			if r.ID == id && !r.Device.IdentityConflict && len(r.Device.XAddrs) > 0 {
				return r.Device, nil
			}
		}
	}
	return devices.Device{}, errors.New("select an unambiguous device with a discovered ONVIF service URL")
}
func targetForONVIF(d devices.Device, pin string) (onvif.Target, error) {
	chosen := ""
	for _, raw := range d.XAddrs {
		if onvif.ValidXAddr(raw, d.IPv4) {
			if chosen == "" {
				chosen = raw
			}
			if strings.HasPrefix(raw, "https://") {
				chosen = raw
				break
			}
		}
	}
	if chosen == "" {
		return onvif.Target{}, errors.New("no usable same-device ONVIF service address")
	}
	u, _ := url.Parse(chosen)
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if u.Port() != "" {
		port, _ = strconv.Atoi(u.Port())
	}
	source, e := platform.IPv4RouteSource(d.IPv4, port)
	if e != nil {
		return onvif.Target{}, errors.New("no IPv4 route to the selected ONVIF device")
	}
	return onvif.Target{Address: d.IPv4, URL: chosen, Source: source, CertificateSHA256: pin}, nil
}
func (m *Manager) ONVIFEndpoint(site, id string) (onvif.Endpoint, error) {
	d, e := m.onvifDevice(site, id)
	if e != nil {
		return onvif.Endpoint{}, e
	}
	target, e := targetForONVIF(d, "")
	if e != nil {
		return onvif.Endpoint{}, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return onvif.InspectEndpoint(ctx, target)
}
func (m *Manager) QueryONVIF(site, id, profile, pin string) error {
	d, e := m.onvifDevice(site, id)
	if e != nil {
		return e
	}
	target, e := targetForONVIF(d, pin)
	if e != nil {
		return e
	}
	client, e := onvif.NewClient(target)
	if e != nil {
		return e
	}
	var secret vault.Secret
	if profile != "" {
		secret, e = m.Store.Secret(site, profile)
		if e != nil {
			return e
		}
	}
	err := m.start("ONVIF information", site, func(ctx context.Context) error {
		defer secret.Clear()
		info, e := client.Information(ctx, d, secret)
		if e != nil {
			return e
		}
		m.Controller.Observe(info.Device)
		if site != "" {
			data, _ := json.Marshal([]devices.Device{info.Device})
			r, e := m.Store.SaveRun(sites.Run{SiteID: site, Kind: "onvif-information", Scope: "Read-only ONVIF information for " + d.IPv4 + "; " + strings.Join(info.Warnings, "; "), Outcome: "partial", Data: string(data)})
			if e != nil {
				return e
			}
			m.setRun(r.ID)
		}
		m.progress(1, 1, "Device information received. "+strings.Join(info.Warnings, "; "))
		return nil
	})
	if err != nil {
		secret.Clear()
	}
	return err
}
