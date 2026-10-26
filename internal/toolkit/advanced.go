package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/configbackup"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/isapi"
	"github.com/TerminalAddict/SADPTools/internal/nvr"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/remote"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
	"time"
)

type DeviceRead struct {
	SiteID   string `json:"siteID"`
	DeviceID string `json:"deviceID"`
	Pin      string `json:"pin"`
}
type EventAssessment struct {
	EventID    string `json:"eventID"`
	Result     string `json:"result"`
	Notes      string `json:"notes"`
	SnapshotID string `json:"snapshotID"`
}
type WalkTest struct {
	Assessments  []EventAssessment       `json:"assessments"`
	ID           string                  `json:"id"`
	SiteID       string                  `json:"siteID"`
	DeviceID     string                  `json:"deviceID"`
	Started      string                  `json:"started"`
	Completed    string                  `json:"completed"`
	Running      bool                    `json:"running"`
	Capabilities onvif.EventCapabilities `json:"capabilities"`
	Events       []onvif.CameraEvent     `json:"events"`
	Total        int                     `json:"total"`
	Result       string                  `json:"result"`
	Notes        string                  `json:"notes"`
	Error        string                  `json:"error"`
	Cleanup      string                  `json:"cleanup"`
	EvidenceIDs  []string                `json:"evidenceIDs"`
}

func (m *Manager) readCredential(o DeviceRead) (devices.Device, vault.Secret, error) {
	d, e := m.previewDevice(o.SiteID, o.DeviceID)
	if e != nil {
		return d, vault.Secret{}, e
	}
	if o.SiteID == "" {
		return d, vault.Secret{}, errors.New("save this device to a site and assign its credential first")
	}
	s, source, e := m.Store.DeviceSecret(o.SiteID, o.DeviceID)
	if e == nil && source == "none" {
		e = errors.New("assign a device/site credential in Saved sites before this authenticated read")
	}
	return d, s, e
}
func (m *Manager) onvifClient(d devices.Device, pin string) (*onvif.Client, error) {
	target, e := targetForONVIF(d, pin)
	if e != nil {
		return nil, e
	}
	return onvif.NewClient(target)
}
func (m *Manager) document(site, device, kind, title string, value any) (sites.Document, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return sites.Document{}, e
	}
	return m.Store.SaveDocument(sites.Document{SiteID: site, DeviceID: device, Kind: kind, Title: title, Version: 1, Data: b})
}
func (m *Manager) StartNVR(o DeviceRead) error {
	return m.start("nvr-channels", o.SiteID, func(ctx context.Context) error {
		d, s, e := m.readCredential(o)
		if e != nil {
			return e
		}
		defer s.Clear()
		m.progress(0, 3, "Detecting recorder web interface")
		target := webinterface.Target{Address: d.IPv4}
		if d.HTTPPort != nil {
			target.Port = *d.HTTPPort
		}
		if d.HTTPSPort != nil {
			target.HTTPSPort = *d.HTTPSPort
		}
		endpoint, e := webinterface.New().Detect(ctx, target)
		if e != nil {
			return e
		}
		c, e := isapi.New(d.IPv4, endpoint.Endpoint.URL, o.Pin)
		if e != nil {
			return e
		}
		m.progress(1, 3, "Reading advertised NVR channels and stream mappings")
		inv, e := nvr.Read(ctx, c, s)
		if e != nil {
			return e
		}
		inv.NVRID = o.DeviceID
		rows, e := m.Store.Records(o.SiteID)
		if e != nil {
			return e
		}
		nvr.Match(inv.Channels, rows)
		links, e := m.Store.Documents(o.SiteID, "channel-links")
		if e != nil {
			return e
		}
		for _, doc := range links {
			if doc.DeviceID != o.DeviceID {
				continue
			}
			var link ChannelLinks
			if json.Unmarshal(doc.Data, &link) != nil {
				continue
			}
			for i := range inv.Channels {
				ch := &inv.Channels[i]
				if id, ok := link.Links[ch.ID]; ok {
					conflict := false
					for _, r := range rows {
						if r.ID == id && devices.ConflictingIdentity(r.Device, devices.Device{MAC: ch.MAC, Serial: ch.Serial}) {
							conflict = true
						}
					}
					if conflict {
						ch.LinkedDeviceID = ""
						ch.Confidence = "saved manual link conflicts with current identity — review required"
					} else {
						ch.LinkedDeviceID = id
						ch.Confidence = "manual link (retained across scans)"
					}
				}
			}
			break
		}
		_, e = m.document(o.SiteID, o.DeviceID, "nvr-channels", "NVR channel inventory", inv)
		m.progress(3, 3, "Channel inventory saved; connection/recording unknowns preserved")
		return e
	})
}
func (m *Manager) StartWalk(o DeviceRead, seconds int) error {
	if seconds < 10 || seconds > 300 {
		return errors.New("walk-test duration must be 10–300 seconds")
	}
	return m.start("walk-test", o.SiteID, func(parent context.Context) (result error) {
		ctx, cancel := context.WithTimeout(parent, time.Duration(seconds)*time.Second)
		defer cancel()
		w := WalkTest{ID: sites.ID(), SiteID: o.SiteID, DeviceID: o.DeviceID, Started: time.Now().UTC().Format(time.RFC3339Nano), Running: true, Events: []onvif.CameraEvent{}, EvidenceIDs: []string{}, Result: "not-tested"}
		m.mu.Lock()
		m.walk = w
		m.mu.Unlock()
		m.changed()
		defer func() {
			m.mu.Lock()
			m.walk.Running = false
			m.walk.Completed = time.Now().UTC().Format(time.RFC3339Nano)
			if result != nil && !errors.Is(result, context.Canceled) && !errors.Is(result, context.DeadlineExceeded) {
				m.walk.Error = sanitize.Text(result.Error())
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
		m.walk.Capabilities = caps
		m.mu.Unlock()
		sub, e := c.Subscribe(ctx, s)
		if e != nil {
			return e
		}
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			e := sub.Close(closeCtx)
			m.mu.Lock()
			m.walk.Cleanup = "subscription closed"
			if e != nil {
				m.walk.Cleanup = "unsubscribe unconfirmed; subscription expires on device (requested two minutes)"
			}
			m.mu.Unlock()
		}()
		start := time.Now()
		for ctx.Err() == nil {
			events, e := sub.Pull(ctx)
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return e
			}
			m.mu.Lock()
			for i := range events {
				events[i].EventID = fmt.Sprint(m.walk.Total + i + 1)
			}
			m.walk.Total += len(events)
			m.walk.Events = append(m.walk.Events, events...)
			if len(m.walk.Events) > 500 {
				m.walk.Events = m.walk.Events[len(m.walk.Events)-500:]
			}
			m.mu.Unlock()
			m.progress(int(time.Since(start).Seconds()), seconds, "Listening for events; latest 500 retained, no analytics settings changed")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(150 * time.Millisecond):
			}
		}
		return nil
	})
}
func (m *Manager) Walk() WalkTest {
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.walk
	w.Events = append([]onvif.CameraEvent{}, w.Events...)
	w.Capabilities.Topics = append([]string{}, w.Capabilities.Topics...)
	w.EvidenceIDs = append([]string{}, w.EvidenceIDs...)
	w.Assessments = append([]EventAssessment{}, w.Assessments...)
	return w
}
func (m *Manager) SaveWalk(id, result, notes string, evidence []string, assessments []EventAssessment) (sites.Document, error) {
	w := m.Walk()
	if w.ID != id || w.ID == "" || w.Running {
		return sites.Document{}, errors.New("finish the current walk-test before saving its summary")
	}
	if result != "pass" && result != "fail" && result != "not-tested" {
		return sites.Document{}, errors.New("choose a walk-test verdict")
	}
	if len(notes) > 8192 || len(evidence) > 20 {
		return sites.Document{}, errors.New("walk-test notes or evidence exceed limits")
	}
	for _, id := range evidence {
		if e := m.imageBelongs(w.SiteID, id, "snapshot"); e != nil {
			return sites.Document{}, e
		}
	}
	validEvents := map[string]bool{}
	for _, event := range w.Events {
		validEvents[event.EventID] = true
	}
	seen := map[string]bool{}
	if len(assessments) > 500 {
		return sites.Document{}, errors.New("too many event assessments")
	}
	for i := range assessments {
		a := &assessments[i]
		if !validEvents[a.EventID] || seen[a.EventID] || a.Result != "pass" && a.Result != "fail" && a.Result != "not-tested" || len(a.Notes) > 4096 {
			return sites.Document{}, errors.New("invalid or duplicate event assessment")
		}
		seen[a.EventID] = true
		a.Notes = sanitize.Text(a.Notes)
		if a.SnapshotID != "" {
			if e := m.imageBelongs(w.SiteID, a.SnapshotID, "snapshot"); e != nil {
				return sites.Document{}, e
			}
		}
	}
	w.Assessments = assessments
	w.Result = result
	w.Notes = sanitize.Text(notes)
	w.EvidenceIDs = evidence
	return m.document(w.SiteID, w.DeviceID, "walk-test", "Event walk-test", w)
}
func (m *Manager) StartConfiguration(o DeviceRead) error {
	return m.start("configuration-backup", o.SiteID, func(ctx context.Context) error {
		d, s, e := m.readCredential(o)
		if e != nil {
			return e
		}
		defer s.Clear()
		c, e := m.onvifClient(d, o.Pin)
		if e != nil {
			return e
		}
		m.progress(0, 1, "Reading documented settings; no device writes")
		b, e := configbackup.Capture(ctx, c, d, s)
		if e != nil {
			return e
		}
		raw, e := json.Marshal(b)
		if e != nil {
			return e
		}
		defer clear(raw)
		a, e := m.Store.SaveAsset(sites.Asset{SiteID: o.SiteID, DeviceID: o.DeviceID, Kind: "configuration", Title: "Configuration · " + b.At}, raw)
		if e != nil {
			return e
		}
		sections := append([]configbackup.Setting{}, b.Settings...)
		for i := range sections {
			sections[i].Value = ""
		}
		_, e = m.document(o.SiteID, o.DeviceID, "configuration", "Encrypted configuration backup", ConfigReference{AssetID: a.ID, Provider: b.Provider, At: b.At, Checksum: b.Checksum, Model: b.Device.Model, Firmware: b.Device.Firmware, Sections: sections})
		m.progress(1, 1, "Encrypted settings backup saved; unsupported sections marked")
		return e
	})
}

type ConfigReference struct {
	AssetID  string                 `json:"assetID"`
	Provider string                 `json:"provider"`
	At       string                 `json:"at"`
	Checksum string                 `json:"checksum"`
	Model    string                 `json:"model"`
	Firmware string                 `json:"firmware"`
	Sections []configbackup.Setting `json:"sections"`
}

func (m *Manager) ConfigBackup(site, id string) (configbackup.Backup, error) {
	doc, e := m.Store.Document(site, id)
	if e != nil || doc.Kind != "configuration" {
		return configbackup.Backup{}, errors.New("configuration backup not found")
	}
	var ref ConfigReference
	if e = json.Unmarshal(doc.Data, &ref); e != nil {
		return configbackup.Backup{}, e
	}
	asset, raw, e := m.Store.AssetData(site, ref.AssetID)
	if e != nil {
		return configbackup.Backup{}, e
	}
	defer clear(raw)
	if asset.Kind != "configuration" {
		return configbackup.Backup{}, errors.New("asset is not a configuration backup")
	}
	var b configbackup.Backup
	if e = json.Unmarshal(raw, &b); e != nil {
		return b, e
	}
	return b, b.Validate()
}
func (m *Manager) ImportConfiguration(site, device string, data []byte, passphrase string) (sites.Document, error) {
	b, e := configbackup.Import(data, passphrase)
	if e != nil {
		return sites.Document{}, e
	}
	if _, e = m.previewDevice(site, device); e != nil {
		return sites.Document{}, e
	}
	raw, _ := json.Marshal(b)
	defer clear(raw)
	a, e := m.Store.SaveAsset(sites.Asset{SiteID: site, DeviceID: device, Kind: "configuration", Title: "Imported configuration · " + b.At}, raw)
	if e != nil {
		return sites.Document{}, e
	}
	sections := append([]configbackup.Setting{}, b.Settings...)
	for i := range sections {
		sections[i].Value = ""
	}
	return m.document(site, device, "configuration", "Imported encrypted configuration", ConfigReference{a.ID, b.Provider, b.At, b.Checksum, b.Device.Model, b.Device.Firmware, sections})
}
func (m *Manager) ConfigurationPlan(site, backup, device string) (configbackup.Preview, error) {
	b, e := m.ConfigBackup(site, backup)
	if e != nil {
		return configbackup.Preview{}, e
	}
	d, e := m.previewDevice(site, device)
	if e != nil {
		return configbackup.Preview{}, e
	}
	return configbackup.Plan(b, d), nil
}
func (m *Manager) StartRemote(site, docID string) error {
	return m.start("remote-discovery", site, func(ctx context.Context) error {
		doc, e := m.Store.Document(site, docID)
		if e != nil || doc.Kind != "ssh-target" {
			return errors.New("choose a saved SSH target for this site")
		}
		var target remote.Target
		if json.Unmarshal(doc.Data, &target) != nil {
			return errors.New("invalid SSH target")
		}
		secret := vault.Secret{}
		if target.CredentialID != "" {
			secret, e = m.Store.Secret(site, target.CredentialID)
			if e != nil {
				return e
			}
		}
		defer secret.Clear()
		m.progress(0, 1, "Verifying SSH host and remote CLI, then discovering on the remote adapter")
		observation, e := remote.Run(ctx, target, secret)
		if e != nil {
			return e
		}
		observation.Target.CredentialID = ""
		observation.Target.KeyPath = ""
		observation.Target.KnownHosts = ""
		_, e = m.document(site, "", "remote-observation", "Remote discovery · "+target.Host, observation)
		m.progress(1, 1, "Remote observations saved separately; review before importing devices")
		return e
	})
}
func (m *Manager) ImportRemote(site, docID string, ids []string) (sites.SaveResult, error) {
	doc, e := m.Store.Document(site, docID)
	if e != nil || doc.Kind != "remote-observation" {
		return sites.SaveResult{}, errors.New("remote observations not found in selected site")
	}
	var ob remote.Observation
	if json.Unmarshal(doc.Data, &ob) != nil {
		return sites.SaveResult{}, errors.New("invalid remote observations")
	}
	chosen := map[string]bool{}
	for _, id := range ids {
		chosen[id] = true
	}
	rows := []devices.Device{}
	for _, d := range ob.Report.Devices {
		if chosen[d.ID] {
			rows = append(rows, d)
			delete(chosen, d.ID)
		}
	}
	if len(chosen) > 0 || len(rows) == 0 {
		return sites.SaveResult{}, errors.New("select devices from this remote observation")
	}
	result, e := m.Store.SaveObserved(site, rows)
	if e == nil {
		_, e = m.Store.SaveRun(sites.Run{SiteID: site, Kind: "remote-import", Scope: ob.Target.Host, Outcome: fmt.Sprintf("%d selected remote observations imported", len(rows)), Data: string(doc.Data)})
	}
	return result, e
}

func (m *Manager) ChannelStream(site, nvrID, channel, stream string) (string, error) {
	docs, e := m.Store.Documents(site, "nvr-channels")
	if e != nil {
		return "", e
	}
	for _, doc := range docs {
		if doc.DeviceID != nvrID {
			continue
		}
		var inv nvr.Inventory
		if json.Unmarshal(doc.Data, &inv) != nil {
			return "", errors.New("invalid channel inventory")
		}
		for _, ch := range inv.Channels {
			if channel != "" && ch.ID != channel {
				continue
			}
			for _, st := range ch.Streams {
				if st.ID == stream {
					return st.ID, nil
				}
			}
		}
		break
	}
	return "", errors.New("stream is not advertised in the latest saved NVR channel inventory; refresh channels")
}
