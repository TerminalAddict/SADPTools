package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/remote"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"sort"

	"time"
)

func (m *Manager) SaveFirmware(site string, f diagnostics.Firmware) (sites.Document, error) {
	if e := f.Validate(); e != nil {
		return sites.Document{}, e
	}
	return m.document(site, "", "firmware", f.Model+" · "+f.Version, f)
}
func (m *Manager) MatchFirmware(site, document, device string) (diagnostics.FirmwareMatch, error) {
	doc, e := m.Store.Document(site, document)
	if e != nil || doc.Kind != "firmware" {
		return diagnostics.FirmwareMatch{}, errors.New("choose an imported firmware library entry")
	}
	var f diagnostics.Firmware
	if json.Unmarshal(doc.Data, &f) != nil {
		return diagnostics.FirmwareMatch{}, errors.New("invalid firmware metadata")
	}
	d, _, e := m.fieldDevice(DeviceRead{SiteID: site, DeviceID: device})
	if e != nil {
		return diagnostics.FirmwareMatch{}, e
	}
	identity := diagnostics.FirmwareIdentity{Vendor: d.Vendor, Model: d.Model, Version: d.Firmware, Source: "saved discovery observation; identifiers not reported by the device remain unknown"}
	evidence, _ := m.Store.Documents(site, "field-evidence")
	for _, doc := range evidence {
		if doc.DeviceID != device {
			continue
		}
		var f FieldEvidence
		if json.Unmarshal(doc.Data, &f) != nil || f.IdentityKey != d.Key() {
			continue
		}
		inspected := false
		for _, p := range f.Probes {
			if p.Step.Status != "verified" {
				continue
			}
			root, e := xmlsafe.Parse([]byte(p.Response))
			if e != nil {
				continue
			}
			if p.Capability.ID == "onvif-device" {
				inspected = true
				nodes := root.All("GetDeviceInformationResponse")
				if len(nodes) == 1 {
					n := nodes[0]
					if identity.Hardware == "" {
						identity.Hardware = n.Value("HardwareId")
					}
				}
			}
			if p.Capability.ID == "isapi-device" && root.Name == "DeviceInfo" {
				inspected = true
				if identity.Hardware == "" {
					identity.Hardware = root.Value("hardwareVersion")
				}
				if identity.Platform == "" {
					identity.Platform = root.Value("platform")
				}
				if identity.Region == "" {
					identity.Region = root.Value("region")
				}
			}
		}
		if !inspected {
			continue
		}
		identity.Source += "; last selected identity inspection " + doc.Updated
		break
	}
	return diagnostics.MatchFirmware(f, identity), nil
}
func (m *Manager) VerifyReplacement(site, id string) error {
	return m.start("replacement-preflight", site, func(ctx context.Context) error {
		doc, e := m.Store.Document(site, id)
		if e != nil || doc.Kind != "replacement" {
			return errors.New("replacement plan not found")
		}
		var p sites.Replacement
		if json.Unmarshal(doc.Data, &p) != nil || p.State == "completed" {
			return errors.New("replacement plan cannot be verified")
		}
		p.State = "needs-review"
		p.VerifiedAt = time.Time{}
		p.VerifiedIP = ""
		p.Verification = "not-tested"
		p.Checks = []string{}
		found := false
		var observed devices.Device
		for _, d := range m.Controller.Snapshot().Devices {
			if d.Key() == p.NewIdentity.Key() && !d.IdentityConflict && !devices.ConflictingIdentity(d, p.NewIdentity) && time.Since(d.LastSeen) >= 0 && time.Since(d.LastSeen) < 5*time.Minute {
				found = true
				observed = d
			}
		}
		if !found {
			p.Checks = append(p.Checks, "Replacement not rediscovered by MAC/serial within five minutes; run discovery and save/reconcile its current address")
		} else {
			conflict := false
			for _, d := range m.Controller.Snapshot().Devices {
				if d.IPv4 == observed.IPv4 && d.Key() != observed.Key() && time.Since(d.LastSeen) < 5*time.Minute {
					conflict = true
				}
			}
			if conflict {
				p.Checks = append(p.Checks, "Current address has conflicting discovered identities; stop and resolve the conflict")
			} else {
				run, err := diagnostics.NewNetwork().Run(ctx, diagnostics.Options{Target: observed.IPv4, TimeoutMS: 1200, Ports: []int{80, 443, 554}, HTTPPorts: []int{80}, HTTPSPorts: []int{443}, ICMP: true}, nil)
				if err != nil {
					return err
				}
				connected := false
				for _, s := range run.Steps {
					connected = connected || s.Protocol == "tcp" && s.Status == "verified"
				}
				if connected {
					p.State = "verified"
					p.VerifiedAt = time.Now().UTC()
					p.VerifiedIP = observed.IPv4
					p.Verification = "Recent stable-identity discovery and TCP connectivity verified; stream/NVR/recording still require operator checks"
				}
				p.Checks = append(p.Checks, run.Summary())
				rows, _ := m.Store.Records(site)
				for _, r := range rows {
					if r.ID == p.NewDeviceID && r.Device.IPv4 != observed.IPv4 {
						p.State = "needs-review"
						p.Checks = append(p.Checks, "Saved address differs from current discovery; reconcile it before completing replacement")
					}
				}
			}
		}
		p.Checks = append(p.Checks, "No unused-address conclusion from ping; destination address changes require a separate conflict review", "Verify Live View, NVR association and recording manually; no device settings have been transferred")
		raw, _ := json.Marshal(p)
		doc.Data = raw
		_, e = m.Store.SaveDocument(doc)
		return e
	})
}

type ANPRSession struct {
	ID           string             `json:"id"`
	SiteID       string             `json:"siteID"`
	DeviceID     string             `json:"deviceID"`
	Started      string             `json:"started"`
	Completed    string             `json:"completed"`
	Running      bool               `json:"running"`
	Events       []onvif.PlateEvent `json:"events"`
	Total        int                `json:"total"`
	Topics       []string           `json:"topics"`
	Status       string             `json:"status"`
	Error        string             `json:"error"`
	Cleanup      string             `json:"cleanup"`
	RetainPlates bool               `json:"retainPlates"`
}

func (m *Manager) ANPR() ANPRSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.anpr.Running && m.anpr.Completed != "" {
		t, _ := time.Parse(time.RFC3339Nano, m.anpr.Completed)
		if time.Since(t) > 5*time.Minute {
			m.anpr.Events = []onvif.PlateEvent{}
			m.anpr.Status = "expired — plate evidence cleared after five minutes"
		}
	}
	s := m.anpr
	s.Events = append([]onvif.PlateEvent{}, s.Events...)
	s.Topics = append([]string{}, s.Topics...)
	return s
}
func (m *Manager) ClearANPR() {
	m.mu.Lock()
	m.anpr.Events = []onvif.PlateEvent{}
	m.anpr.RetainPlates = false
	m.mu.Unlock()
	m.changed()
}
func (m *Manager) StartANPR(o DeviceRead, seconds int, retain bool) error {
	if seconds < 10 || seconds > 300 {
		return errors.New("ANPR sessions must last 10–300 seconds")
	}
	return m.start("anpr-session", o.SiteID, func(parent context.Context) (result error) {
		ctx, cancel := context.WithTimeout(parent, time.Duration(seconds)*time.Second)
		defer cancel()
		m.mu.Lock()
		if m.anprExpiry != nil {
			m.anprExpiry.Stop()
		}
		m.anpr = ANPRSession{ID: sites.ID(), SiteID: o.SiteID, DeviceID: o.DeviceID, Started: time.Now().UTC().Format(time.RFC3339Nano), Running: true, Events: []onvif.PlateEvent{}, Topics: []string{}, Status: "connecting", RetainPlates: retain}
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			m.anpr.Running = false
			m.anpr.Completed = time.Now().UTC().Format(time.RFC3339Nano)
			m.expireANPRLocked(m.anpr.ID, 5*time.Minute)
			if result != nil {
				m.anpr.Status = statusOf(result)
				m.anpr.Error = sanitize.Text(result.Error())
			} else if m.anpr.Total == 0 {
				m.anpr.Status = "No plate event observed; detection and transport remain unverified"
			} else {
				m.anpr.Status = "Local events received; NVR/VMS delivery not verified"
			}
			m.mu.Unlock()
			m.changed()
		}()
		d, s, e := m.readCredential(o)
		if e != nil {
			return e
		}
		defer s.Clear()
		c, e := m.onvifClient(d, o.Pin)
		if e != nil {
			return e
		}
		caps, e := c.EventCapabilities(ctx, s)
		if e != nil {
			return e
		}
		m.mu.Lock()
		m.anpr.Topics = caps.Topics
		m.mu.Unlock()
		sub, e := c.Subscribe(ctx, s)
		if e != nil {
			return e
		}
		defer func() {
			clean, stop := context.WithTimeout(context.Background(), 4*time.Second)
			defer stop()
			err := sub.Close(clean)
			m.mu.Lock()
			m.anpr.Cleanup = "subscription closed"
			if err != nil {
				m.anpr.Cleanup = "unsubscribe unconfirmed; subscription expiry requested"
			}
			m.mu.Unlock()
		}()
		seen := map[string]bool{}
		start := time.Now()
		for ctx.Err() == nil {
			batch, e := sub.PullPlates(ctx)
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return e
			}
			m.mu.Lock()
			for _, event := range batch {
				if seen[event.ID] {
					continue
				}
				if len(seen) > 2000 {
					m.mu.Unlock()
					return errors.New("ANPR deduplication limit reached; session stopped")
				}
				seen[event.ID] = true
				m.anpr.Total++
				if !m.anpr.RetainPlates {
					event.Plate = "[redacted]"
				}
				m.anpr.Events = append(m.anpr.Events, event)
			}
			if len(m.anpr.Events) > 500 {
				m.anpr.Events = m.anpr.Events[len(m.anpr.Events)-500:]
			}
			sort.SliceStable(m.anpr.Events, func(i, j int) bool { return m.anpr.Events[i].DeviceTime < m.anpr.Events[j].DeviceTime })
			m.mu.Unlock()
			m.progress(int(time.Since(start).Seconds()), seconds, "ANPR listening; camera → local receipt; VMS remains unverified")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(150 * time.Millisecond):
			}
		}
		return nil
	})
}

type ANPRGrade struct {
	Expected            int    `json:"expected"`
	Matched             int    `json:"matched"`
	RecognitionFailures int    `json:"recognitionFailures"`
	TransportFailures   int    `json:"transportFailures"`
	Notes               string `json:"notes"`
	Conditions          string `json:"conditions"`
	Integration         string `json:"integration"`
}

func (m *Manager) SaveANPR(id string, g ANPRGrade) (sites.Document, error) {
	s := m.ANPR()
	if s.Running || s.ID != id || id == "" {
		return sites.Document{}, errors.New("finish the current ANPR session first")
	}
	if g.Expected < 1 || g.Expected > 10000 || g.Matched < 0 || g.Matched > g.Expected || g.RecognitionFailures < 0 || g.TransportFailures < 0 || g.Matched+g.RecognitionFailures+g.TransportFailures > g.Expected {
		return sites.Document{}, errors.New("enter valid, non-overlapping technician-approved test counts")
	}
	if len(g.Notes) > 4096 || len(g.Conditions) > 4096 {
		return sites.Document{}, errors.New("notes too long")
	}
	g.Notes = sanitize.Text(g.Notes)
	g.Conditions = sanitize.Text(g.Conditions)
	g.Integration = "not-verified — local events alone do not verify NVR/VMS integration"
	value := struct {
		Grade     ANPRGrade `json:"grade"`
		MatchRate float64   `json:"matchRate"`
		Source    string    `json:"source"`
		Received  int       `json:"received"`
		Started   string    `json:"started"`
		Completed string    `json:"completed"`
		Transport string    `json:"transport"`
	}{g, 100 * float64(g.Matched) / float64(g.Expected), "Technician-approved test counts; no plate numbers or images retained", s.Total, s.Started, s.Completed, s.Status}
	return m.document(s.SiteID, s.DeviceID, "anpr-session", fmt.Sprintf("ANPR test · %.1f%%", value.MatchRate), value)
}

func (m *Manager) RemoteTroubleshoot(o DeviceRead, targetID string) error {
	return m.start("remote-troubleshoot", o.SiteID, func(ctx context.Context) error {
		doc, e := m.Store.Document(o.SiteID, targetID)
		if e != nil || doc.Kind != "ssh-target" {
			return errors.New("choose an explicitly trusted SSH target in this site")
		}
		var t remote.Target
		if json.Unmarshal(doc.Data, &t) != nil {
			return errors.New("invalid remote probe")
		}
		d, _, e := m.fieldDevice(o)
		if e != nil {
			return e
		}
		secret := vault.Secret{}
		if t.CredentialID != "" {
			secret, e = m.Store.Secret(o.SiteID, t.CredentialID)
			if e != nil {
				return e
			}
		}
		defer secret.Clear()
		out, e := remote.Troubleshoot(ctx, t, secret, d.IPv4)
		if e != nil {
			return e
		}
		if out.Connectivity == nil {
			return errors.New("remote returned no diagnostic evidence")
		}
		f := FieldEvidence{SiteID: o.SiteID, DeviceID: o.DeviceID, IdentityKey: d.Key(), Run: out.Connectivity.Run}
		f.Run.ID = sites.ID()
		f.Run.Kind = "remote-connectivity"
		f.Run.Steps = append(f.Run.Steps, diagnostics.Step{Test: "Observation source", Status: "observed", Evidence: "Trusted SSH probe " + sanitize.Label(t.Host)})
		_, e = m.document(o.SiteID, o.DeviceID, "field-evidence", "Remote connectivity", f)
		return e
	})
}

// Caller holds m.mu. Cleanup occurs even if the page is closed and never polled.
func (m *Manager) expireANPRLocked(id string, after time.Duration) {
	if m.anprExpiry != nil {
		m.anprExpiry.Stop()
	}
	if m.closed {
		m.anpr.Events = nil
		return
	}
	m.anprExpiry = time.AfterFunc(after, func() {
		m.mu.Lock()
		if m.anpr.ID == id && !m.anpr.Running {
			m.anpr.Events = []onvif.PlateEvent{}
			m.anpr.Status = "expired — plate evidence cleared after five minutes"
			m.anpr.RetainPlates = false
		}
		m.mu.Unlock()
		m.changed()
	})
}
