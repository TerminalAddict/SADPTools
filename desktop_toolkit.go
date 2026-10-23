//go:build desktop

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/audit"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/provision"
	"github.com/TerminalAddict/SADPTools/internal/reports"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/switches"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
	"github.com/wailsapp/wails/v3/pkg/application"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ToolkitService struct {
	manager   *toolkit.Manager
	app       *application.App
	loadError error
}

func (s *ToolkitService) ready() error {
	if s.loadError != nil || s.manager == nil {
		return errors.New("site storage could not open; check the user data folder and database version. Live discovery remains available.")
	}
	return nil
}
func (s *ToolkitService) ServiceShutdown() error {
	if s.manager != nil {
		s.manager.Close()
	}
	return nil
}
func (s *ToolkitService) ListSites() ([]sites.Site, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.Store.Sites()
}
func (s *ToolkitService) Site(id string) (sites.View, error) {
	if e := s.ready(); e != nil {
		return sites.View{}, e
	}
	return s.manager.Store.View(id)
}
func (s *ToolkitService) VisitSite(id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.Visit(id)
}
func (s *ToolkitService) SaveSite(site sites.Site) (sites.Site, error) {
	if e := s.ready(); e != nil {
		return site, e
	}
	return s.manager.Store.SaveSite(site)
}
func (s *ToolkitService) DeleteSite(id, confirmation string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.DeleteSite(id, confirmation)
}
func (s *ToolkitService) SaveLiveToSite(site string, ids []string) (sites.SaveResult, error) {
	if e := s.ready(); e != nil {
		return sites.SaveResult{}, e
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	rows := []devices.Device{}
	for _, d := range s.manager.Controller.Snapshot().Devices {
		if selected[d.ID] {
			rows = append(rows, d)
			delete(selected, d.ID)
		}
	}
	if len(selected) > 0 {
		return sites.SaveResult{}, errors.New("selected device is no longer in live discovery")
	}
	return s.manager.Store.SaveObserved(site, rows)
}
func (s *ToolkitService) SaveSiteDevice(r sites.Record) (sites.Record, error) {
	if e := s.ready(); e != nil {
		return r, e
	}
	return s.manager.Store.SaveRecord(r)
}
func (s *ToolkitService) MoveSiteDevice(site, id, destination string) (sites.Record, error) {
	if e := s.ready(); e != nil {
		return sites.Record{}, e
	}
	return s.manager.Store.MoveRecord(site, id, destination)
}
func (s *ToolkitService) MergeSiteDevices(site, keep, remove, confirm string) (sites.Record, error) {
	if e := s.ready(); e != nil {
		return sites.Record{}, e
	}
	return s.manager.Store.MergeRecords(site, keep, remove, confirm)
}
func (s *ToolkitService) SearchSites(query string) ([]sites.Record, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.Store.Search(query)
}
func (s *ToolkitService) SiteHistory(site, device string) ([]sites.Event, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.Store.History(site, device)
}
func (s *ToolkitService) SiteRun(site, id string) (sites.Run, error) {
	if e := s.ready(); e != nil {
		return sites.Run{}, e
	}
	return s.manager.Store.Run(site, id)
}
func (s *ToolkitService) ReserveIP(site, ip, note string, remove bool) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.Reserve(site, ip, note, remove)
}
func (s *ToolkitService) VaultStatus() (sites.VaultStatus, error) {
	if e := s.ready(); e != nil {
		return sites.VaultStatus{}, e
	}
	return s.manager.Store.VaultStatus(), nil
}
func (s *ToolkitService) UnlockVault(passphrase string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.Unlock(passphrase)
}
func (s *ToolkitService) LockVault() {
	if s.manager != nil {
		s.manager.Store.Lock()
	}
}
func (s *ToolkitService) SaveCredential(p sites.Profile, username, password, privacyPassword string) (sites.Profile, error) {
	if e := s.ready(); e != nil {
		return p, e
	}
	secret := vault.Secret{Username: username, Password: password, PrivacyPassword: privacyPassword}
	defer secret.Clear()
	return s.manager.Store.SaveProfile(p, secret)
}
func (s *ToolkitService) DeleteCredential(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.DeleteProfile(site, id)
}
func (s *ToolkitService) RevealPassword(site, id string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	secret, e := s.manager.Store.Secret(site, id)
	defer secret.Clear()
	if e != nil {
		return "", e
	}
	return secret.Password, nil
}
func (s *ToolkitService) CopyPassword(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	secret, e := s.manager.Store.Secret(site, id)
	defer secret.Clear()
	if e != nil {
		return e
	}
	if !s.app.Clipboard.SetText(secret.Password) {
		return errors.New("clipboard is unavailable")
	}
	return nil
}
func (s *ToolkitService) CopyText(value string) error {
	if len(value) > 32768 {
		return errors.New("clipboard text is too large")
	}
	if !s.app.Clipboard.SetText(value) {
		return errors.New("clipboard is unavailable")
	}
	return nil
}
func (s *ToolkitService) Job() (toolkit.Job, error) {
	if e := s.ready(); e != nil {
		return toolkit.Job{}, e
	}
	return s.manager.Job(), nil
}
func (s *ToolkitService) CancelJob() {
	if s.manager != nil {
		s.manager.Cancel()
	}
}
func (s *ToolkitService) saveFile(name string, data []byte) (string, error) {
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	path, e := s.app.Dialog.SaveFile().SetFilename(name).AddFilter("SADPTools file", "*."+ext).PromptForSingleSelection()
	if e != nil || path == "" {
		return "", e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".sadptools-*.tmp")
	if e != nil {
		return "", e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return "", e
	}
	return path, nil
}
func (s *ToolkitService) BackupSite(site, passphrase string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	data, e := s.manager.Store.Backup(site, passphrase)
	if e != nil {
		return "", e
	}
	defer clear(data)
	return s.saveFile("sadptools-site.sadpsite", data)
}
func (s *ToolkitService) RestoreSite(passphrase string) (sites.Site, error) {
	if e := s.ready(); e != nil {
		return sites.Site{}, e
	}
	path, e := s.app.Dialog.OpenFile().AddFilter("Encrypted site backup", "*.sadpsite").PromptForSingleSelection()
	if e != nil || path == "" {
		return sites.Site{}, e
	}
	f, e := os.Open(path)
	if e != nil {
		return sites.Site{}, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 2*vault.MaxBackup+1))
	if e != nil {
		return sites.Site{}, e
	}
	defer clear(data)
	return s.manager.Store.Restore(data, passphrase)
}
func (s *ToolkitService) ExportSite(site, format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	v, e := s.manager.Store.View(site)
	if e != nil {
		return "", e
	}
	switchReports, e := s.manager.Store.SwitchReports(site)
	if e != nil {
		return "", e
	}
	rows := [][]string{}
	port := func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	}
	status := func(v *bool) string {
		if v == nil {
			return ""
		}
		return strconv.FormatBool(*v)
	}
	for _, r := range v.Devices {
		d := r.Device
		lastSeen := ""
		if !d.LastSeen.IsZero() {
			lastSeen = d.LastSeen.UTC().Format(time.RFC3339)
		}
		mapping := []string{}
		for _, report := range switchReports {
			for _, m := range report.Mappings {
				if d.MAC != "" && m.MAC == d.MAC {
					mapping = append(mapping, m.SwitchName+" / "+m.PortName+" ("+m.Confidence+", "+m.At+")")
				}
			}
		}
		rows = append(rows, []string{v.Site.Name, d.IPv4, d.SubnetMask, d.Gateway, status(d.DHCP), d.Vendor, strings.Join(d.Protocols, ", "), d.Model, d.MAC, d.Serial, d.Firmware, status(d.Activated), port(d.HTTPPort), port(d.HTTPSPort), port(d.CommandPort), port(d.RTSPPort), d.Name, r.Location, r.Notes, strings.Join(r.Tags, ", "), lastSeen, strings.Join(mapping, "; ")})
	}

	data, e := s.manager.Store.Export(site)
	if e != nil {
		return "", e
	}
	if format != "json" {
		data, e = reports.Encode(format, "Site inventory — "+v.Site.Name, nil, []string{"Site", "IP", "Subnet mask", "Gateway", "DHCP", "Vendor", "Protocols", "Model", "MAC", "Serial", "Firmware", "Activated", "HTTP port", "HTTPS port", "SDK port", "RTSP port", "Name", "Location", "Notes", "Tags", "Last seen", "Switch observations"}, rows)
	}
	if e != nil {
		return "", e
	}
	return s.saveFile("sadptools-site."+format, data)
}
func (s *ToolkitService) ExportHistory(site, format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	history, e := s.manager.Store.History(site, "")
	if e != nil {
		return "", e
	}
	rows := [][]string{}
	for _, h := range history {
		rows = append(rows, []string{h.At, h.DeviceID, h.Source, h.Kind, h.Before, h.After})
	}
	data, e := reports.Encode(format, "Site change history", history, []string{"Time", "Device", "Source", "Change", "Before", "After"}, rows)
	if e != nil {
		return "", e
	}
	return s.saveFile("sadptools-history."+format, data)
}
func (s *ToolkitService) ReconcileSite(site, run string) ([]sites.Proposal, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	r, e := s.manager.Store.Run(site, run)
	if e != nil {
		return nil, e
	}
	if r.Kind != "discovery" && r.Kind != "onvif-information" {
		return nil, errors.New("select a saved discovery scan")
	}
	var d []devices.Device
	if e = json.Unmarshal([]byte(r.Data), &d); e != nil {
		return nil, fmt.Errorf("invalid scan data")
	}
	rows, e := s.manager.Store.Records(site)
	if e != nil {
		return nil, e
	}
	proposals := sites.Reconcile(rows, d)
	if r.Kind == "onvif-information" {
		filtered := []sites.Proposal{}
		for _, p := range proposals {
			if p.Kind != "not-seen" {
				filtered = append(filtered, p)
			}
		}
		proposals = filtered
	}
	return proposals, nil
}
func (s *ToolkitService) AcceptSiteChange(site, run, record, identity string, fields []string) (sites.Record, error) {
	if e := s.ready(); e != nil {
		return sites.Record{}, e
	}
	return s.manager.Store.ApplyObservation(site, run, record, identity, fields)
}
func (s *ToolkitService) IgnoreSiteChange(site, run, identity string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.IgnoreObservation(site, run, identity)
}

func (s *ToolkitService) IPScope(o ipscan.Options) (ipscan.Scope, error) {
	if e := s.ready(); e != nil {
		return ipscan.Scope{}, e
	}
	return s.manager.IPScope(o)
}
func (s *ToolkitService) StartIPScan(site string, o ipscan.Options) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartIPScan(site, o)
}
func (s *ToolkitService) IPScanResult() (ipscan.Result, error) {
	if e := s.ready(); e != nil {
		return ipscan.Result{}, e
	}
	return s.manager.IPResult(), nil
}
func (s *ToolkitService) ResetIPScan() error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.ResetIPResult()
}
func (s *ToolkitService) SaveIPScan(site string) (sites.Run, error) {
	if e := s.ready(); e != nil {
		return sites.Run{}, e
	}
	return s.manager.SaveIPScan(site)
}
func (s *ToolkitService) ResolveIP(ip string) ([]string, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.ResolveIP(ip)
}
func (s *ToolkitService) OpenIP(ip string, port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	r, e := webinterface.New().Detect(ctx, webinterface.Target{Address: ip, Port: port})
	if e != nil {
		return e
	}
	if !r.Endpoint.ValidFor(ip) {
		return errors.New(r.Message)
	}
	return s.app.Browser.OpenURL(r.Endpoint.URL)
}
func (s *ToolkitService) ExportIPScan(format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	r := s.manager.IPResult()
	rows := [][]string{}
	for _, h := range r.Hosts {
		sources := []string{}
		latency := ""
		at := ""
		for _, e := range h.Evidence {
			sources = append(sources, e.Probe+": "+e.State)
			if e.LatencyMS != nil {
				latency = strconv.FormatFloat(*e.LatencyMS, 'f', 2, 64)
			}
			if e.At > at {
				at = e.At
			}
		}
		rows = append(rows, []string{h.IP, h.Status, strconv.FormatBool(h.Responding), strconv.FormatBool(h.Known), h.Reserved, strings.Join(h.MACs, ", "), strings.Join(h.Matches, ", "), h.Hostname, strings.Join(sources, "; "), latency, at})
	}
	data, e := reports.Encode(format, "IP investigation", r, []string{"IP", "Status", "Responding", "Known", "Reservation", "MAC evidence", "Device matches", "Hostname", "Probes", "Latency ms", "Observed at"}, rows)
	if e != nil {
		return "", e
	}
	return s.saveFile("sadptools-ip-scan."+format, data)
}
func (s *ToolkitService) IPScanPage(offset, limit int, filter, sort string, descending bool) (ipscan.Page, error) {
	if e := s.ready(); e != nil {
		return ipscan.Page{}, e
	}
	return s.manager.IPPage(offset, limit, filter, sort, descending), nil
}
func (s *ToolkitService) ReprobeIP(site, ip string, ports []int) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.ReprobeIP(site, ip, ports)
}

func (s *ToolkitService) SiteScanScope(site string) (toolkit.SiteScope, error) {
	if e := s.ready(); e != nil {
		return toolkit.SiteScope{}, e
	}
	return s.manager.SiteScope(site)
}
func (s *ToolkitService) StartSiteRescan(site, scopeToken string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartSiteRescan(site, scopeToken)
}
func (s *ToolkitService) ONVIFEndpoint(site, id string) (onvif.Endpoint, error) {
	if e := s.ready(); e != nil {
		return onvif.Endpoint{}, e
	}
	return s.manager.ONVIFEndpoint(site, id)
}
func (s *ToolkitService) QueryONVIF(site, id, profile, pin string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.QueryONVIF(site, id, profile, pin)
}

func (s *ToolkitService) PreviewProvision(site string, o provision.Options) (provision.Plan, error) {
	if e := s.ready(); e != nil {
		return provision.Plan{}, e
	}
	return s.manager.PreviewProvision(site, o)
}
func (s *ToolkitService) ProvisionStatus() (provision.Plan, error) {
	if e := s.ready(); e != nil {
		return provision.Plan{}, e
	}
	return s.manager.ProvisionStatus(), nil
}
func (s *ToolkitService) StartProvision(planID, confirmation, currentPassword, newPassword string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartProvision(planID, confirmation, currentPassword, newPassword)
}

func (s *ToolkitService) ExportProvision(format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	p := s.manager.ProvisionStatus()
	rows := [][]string{}
	for _, r := range p.Items {
		rows = append(rows, []string{r.Before.Model, r.Before.MAC, r.Before.IPv4, r.Proposed.IPv4, r.State, r.Message})
	}
	b, e := reports.Encode(format, "Provisioning plan / result", p, []string{"Model", "MAC", "Before IP", "Proposed IP", "State", "Evidence"}, rows)
	if e != nil {
		return "", e
	}
	return s.saveFile("sadptools-provision."+format, b)
}

func (s *ToolkitService) StartAudit(site string, ids []string, pins map[string]string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartAudit(site, ids, pins)
}
func (s *ToolkitService) AuditStatus() (audit.Report, error) {
	if e := s.ready(); e != nil {
		return audit.Report{}, e
	}
	return s.manager.AuditStatus(), nil
}
func (s *ToolkitService) ExportAudit(site, runID, format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	report := s.manager.AuditStatus()
	if runID != "" {
		run, e := s.manager.Store.Run(site, runID)
		if e != nil {
			return "", e
		}
		if run.Kind != "audit" {
			return "", errors.New("select an audit report")
		}
		if e = json.Unmarshal([]byte(run.Data), &report); e != nil {
			return "", errors.New("invalid saved report")
		}
	}
	rows := [][]string{}
	for _, f := range report.Findings {
		rows = append(rows, []string{f.IP, f.Model, f.MAC, f.Check, f.Status, f.Evidence, f.Source, f.At})
	}
	data, e := reports.Encode(format, "Health audit — "+report.SiteName, report, []string{"IP", "Model", "MAC", "Check", "Status", "Evidence", "Source", "Time"}, rows)
	if e != nil {
		return "", e
	}
	return s.saveFile("sadptools-audit."+format, data)
}

func (s *ToolkitService) SwitchAssets(site string) ([]switches.Asset, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.Store.Switches(site)
}
func (s *ToolkitService) SaveSwitch(a switches.Asset) (switches.Asset, error) {
	if e := s.ready(); e != nil {
		return a, e
	}
	return s.manager.Store.SaveSwitch(a)
}
func (s *ToolkitService) DeleteSwitch(site, id, confirmation string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.DeleteSwitch(site, id, confirmation)
}
func (s *ToolkitService) PollSwitch(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.PollSwitch(site, id)
}
func (s *ToolkitService) SwitchReports(site string) ([]switches.Report, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.Store.SwitchReports(site)
}
func (s *ToolkitService) ExportSwitches(site, format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	reportsList, e := s.manager.Store.SwitchReports(site)
	if e != nil {
		return "", e
	}
	rows := [][]string{}
	for _, report := range reportsList {
		for _, m := range report.Mappings {
			power := "Unknown/unsupported"
			if m.PowerWatts != nil {
				power = fmt.Sprintf("%.3f W", *m.PowerWatts)
			}
			rows = append(rows, []string{m.IP, m.MAC, m.SwitchName, m.PortName, strconv.Itoa(m.Port), m.Confidence, fmt.Sprint(m.VLANs), m.Link, power, m.At, strings.Join(m.Evidence, "; ")})
		}
	}
	data, e := reports.Encode(format, "Switch / camera mapping observations", reportsList, []string{"Camera IP", "MAC", "Switch", "Candidate port", "ifIndex", "Confidence", "VLANs", "Link", "Power", "Polled", "Evidence"}, rows)
	if e != nil {
		return "", e
	}
	return s.saveFile("sadptools-switch-mapping."+format, data)
}

func (s *ToolkitService) Preferences() (sites.Preferences, error) {
	if e := s.ready(); e != nil {
		return sites.Preferences{}, e
	}
	return s.manager.Store.Preferences()
}
