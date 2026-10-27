package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/isapi"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/recordings"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
	"strings"
	"time"
)

type FieldOptions struct {
	RTSP bool `json:"rtsp"`
	DeviceRead
	Kind       string              `json:"kind"`
	Network    diagnostics.Options `json:"network"`
	Operations []string            `json:"operations"`
	Window     recordings.Window   `json:"window"`
	Channel    string              `json:"channel"`
	Provider   string              `json:"provider"`
	ProfileID  string              `json:"profileID"`
}
type Finding struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	Evidence       string `json:"evidence"`
	Recommendation string `json:"recommendation"`
	CouldBreak     string `json:"couldBreak"`
}
type FieldEvidence struct {
	SiteID      string               `json:"siteID"`
	DeviceID    string               `json:"deviceID"`
	IdentityKey string               `json:"identityKey"`
	Run         diagnostics.Run      `json:"run"`
	Probes      []diagnostics.Probe  `json:"probes"`
	Findings    []Finding            `json:"findings"`
	Recording   *recordings.Result   `json:"recording"`
	Imaging     *onvif.Imaging       `json:"imaging"`
	Profiles    []onvif.MediaProfile `json:"profiles"`
}

func (m *Manager) Field() FieldEvidence {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, _ := json.Marshal(m.field)
	var out FieldEvidence
	_ = json.Unmarshal(b, &out)
	return out
}
func (m *Manager) setField(v FieldEvidence) { m.mu.Lock(); m.field = v; m.mu.Unlock(); m.changed() }
func (m *Manager) fieldDevice(o DeviceRead) (devices.Device, vault.Secret, error) {
	var d devices.Device
	var e error
	if o.SiteID != "" {
		rows, err := m.Store.Records(o.SiteID)
		if err != nil {
			return d, vault.Secret{}, err
		}
		for _, r := range rows {
			if r.ID == o.DeviceID && !r.Removed {
				d = r.Device
			}
		}
	} else {
		for _, row := range m.Controller.Snapshot().Devices {
			if row.ID == o.DeviceID {
				d = row
			}
		}
	}
	if d.IPv4 == "" || d.IdentityConflict {
		return d, vault.Secret{}, errors.New("choose an unambiguous saved or discovered device")
	}
	return d, vault.Secret{}, e
}
func (m *Manager) fieldISAPI(ctx context.Context, d devices.Device, pin string) (*isapi.Client, error) {
	target := webinterface.Target{Address: d.IPv4}
	if d.HTTPPort != nil {
		target.Port = *d.HTTPPort
	}
	if d.HTTPSPort != nil {
		target.HTTPSPort = *d.HTTPSPort
	}
	v, e := webinterface.New().Detect(ctx, target)
	if e != nil {
		return nil, e
	}
	return isapi.New(d.IPv4, v.Endpoint.URL, pin)
}
func statusOf(e error) string {
	switch {
	case e == nil:
		return "verified"
	case errors.Is(e, isapi.ErrAuthentication) || errors.Is(e, onvif.ErrAuthentication) || errors.Is(e, streaming.ErrAuthentication):
		return "permission-denied"
	case errors.Is(e, isapi.ErrUnsupported) || errors.Is(e, onvif.ErrUnsupported):
		return "unsupported"
	case errors.Is(e, context.Canceled):
		return "cancelled"
	default:
		return "failed"
	}
}
func (m *Manager) StartField(o FieldOptions) error {
	if o.Kind != "connectivity" && o.Kind != "protocol" && o.Kind != "security" && o.Kind != "recordings" && o.Kind != "imaging" {
		return errors.New("unknown diagnostic tool")
	}
	if o.Kind == "connectivity" || o.Kind == "security" {
		// Target is supplied from inventory later; validate only the requested options here.
		network := o.Network
		network.Target = "127.0.0.1"
		if e := network.Validate(); e != nil {
			return e
		}
	}
	if len(o.Operations) > 16 {
		return errors.New("select at most 16 read operations")
	}
	for _, id := range o.Operations {
		if _, ok := diagnostics.FindCapability(id); !ok {
			return errors.New("operation not in read-only catalogue")
		}
	}
	if o.Kind == "recordings" {
		if e := o.Window.Validate(); e != nil {
			return e
		}
	}
	return m.start("field-"+o.Kind, o.SiteID, func(parent context.Context) (result error) {
		ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
		defer cancel()
		d, s, e := m.fieldDevice(o.DeviceRead)
		if e != nil {
			return e
		}
		defer s.Clear()
		if o.SiteID != "" && (o.Kind != "connectivity" || len(o.Operations) > 0 || o.RTSP) {
			s, _, e = m.Store.DeviceSecret(o.SiteID, o.DeviceID)
			if e != nil {
				return e
			}
		}
		f := FieldEvidence{SiteID: o.SiteID, DeviceID: o.DeviceID, IdentityKey: d.Key(), Run: diagnostics.NewRun(o.Kind, d.IPv4), Probes: []diagnostics.Probe{}, Findings: []Finding{}, Profiles: []onvif.MediaProfile{}}
		f.Run.ID = sites.ID()
		m.setField(f)
		defer func() {
			f.Run.Completed = time.Now().UTC().Format(time.RFC3339Nano)
			f.Run.State = "completed"
			if result != nil {
				f.Run.State = "failed"
			}
			if ctx.Err() != nil {
				f.Run.State = "cancelled"
			}
			m.setField(f)
			if o.SiteID != "" {
				_, err := m.document(o.SiteID, o.DeviceID, "field-evidence", o.Kind+" diagnostics", f)
				if result == nil {
					result = err
				}
			}
		}()
		if o.Kind == "connectivity" || o.Kind == "security" {
			o.Network.Target = d.IPv4
			if o.SiteID != "" {
				v, err := m.Store.View(o.SiteID)
				if err == nil {
					o.Network.ExpectedNetworks = v.Site.Networks
				}
			}
			run, err := diagnostics.NewNetwork().Run(ctx, o.Network, func(step diagnostics.Step) {
				f.Run.Steps = append(f.Run.Steps, step)
				m.setField(f)
				m.progress(len(f.Run.Steps), 0, step.Test+": "+step.Status)
			})
			run.ID = f.Run.ID
			f.Run = run
			f.Run.Kind = o.Kind
			m.progress(len(run.Steps), len(run.Steps), "Network checks completed")
			if err != nil {
				return err
			}
			if o.Kind == "security" {
				for _, step := range run.Steps {
					if step.Protocol == "http" && step.Code > 0 {
						f.Findings = append(f.Findings, Finding{"Unencrypted web service", "risky", step.Evidence, "Prefer verified HTTPS where supported; review HTTP dependencies first", "NVR/VMS management, snapshots or legacy integrations may require HTTP"})
					}
					if step.Protocol == "https" && step.Status == "failed" {
						f.Findings = append(f.Findings, Finding{"HTTPS trust/transport", "unknown", step.Evidence, step.Next, "Changing certificates can break pinned VMS integrations"})
					}
					if step.Protocol == "tcp" && step.Status == "verified" {
						f.Findings = append(f.Findings, Finding{step.Test, "observed", "Open port only; service identity and policy not proven", "Review the device's service configuration", "Disabling RTSP, ONVIF or SDK ports can stop recording or management"})
					}
				}
				o.Operations = []string{"onvif-protocols", "onvif-capabilities", "onvif-users"}
				f.Findings = append(f.Findings, Finding{"Account roles and hardening controls", "unknown", "No complete, verified account-policy adapter for this firmware", "Review user roles, legacy services and NVR dependencies in the camera UI", "Account/service changes may disconnect recorders"})
			}
		}
		if o.Kind == "protocol" || o.Kind == "security" || o.Kind == "connectivity" && len(o.Operations) > 0 {
			for i, id := range o.Operations {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p := m.inspectField(ctx, d, s, o.Pin, o.Kind, id)
				f.Probes = append(f.Probes, p)
				f.Run.Steps = append(f.Run.Steps, p.Step)
				m.setField(f)
				m.progress(i+1, len(o.Operations), p.Capability.Purpose+": "+p.Step.Status)
			}
		}
		if o.Kind == "connectivity" && o.RTSP {
			step := diagnostics.Step{Test: "ONVIF profile → RTSP DESCRIBE", Protocol: "rtsp", At: time.Now().UTC().Format(time.RFC3339Nano)}
			start := time.Now()
			err := func() error {
				c, e := m.onvifClient(d, o.Pin)
				if e != nil {
					return e
				}
				cat, e := c.MediaProfiles(ctx, s, 0)
				if e != nil {
					return e
				}
				id := o.ProfileID
				if id == "" && len(cat.Profiles) > 0 {
					id = cat.Profiles[0].ID
				}
				raw, e := c.StreamURI(ctx, cat, id, false, s)
				if e != nil {
					return e
				}
				source, e := streaming.NewSource(d.IPv4, raw, "rtsp", o.Pin, s)
				if e != nil {
					return e
				}
				desc, e := streaming.Describe(ctx, source)
				step.Code = desc.Code
				if e == nil {
					step.Evidence = "SDP received; advertised codecs: " + strings.Join(desc.Codecs, ", ") + ". No playback/recording verified."
				}
				return e
			}()
			step.DurationMS = time.Since(start).Milliseconds()
			step.Status = statusOf(err)
			if err != nil {
				step.Evidence = sanitize.Text(err.Error())
				step.Next = "Check selected ONVIF profile, RTSP permissions and transport; try Live View for media decoding evidence"
			}
			f.Run.Steps = append(f.Run.Steps, step)
			m.setField(f)
		}

		if o.Kind == "recordings" {
			var r recordings.Result
			if o.Provider == "isapi" {
				c, err := m.fieldISAPI(ctx, d, o.Pin)
				if err != nil {
					return err
				}
				r, e = c.SearchRecordings(ctx, o.Channel, o.Window, s)
			} else if o.Provider == "onvif" {
				c, err := m.onvifClient(d, o.Pin)
				if err != nil {
					return err
				}
				r, e = c.RecordingMetadata(ctx, o.Window, s)
			} else {
				return errors.New("choose ONVIF or ISAPI recording metadata")
			}
			if e != nil {
				r.State = statusOf(e)
				r.Replay = "not-verified"
				r.Notes = append(r.Notes, sanitize.Text(e.Error()), "Use the recorder's playback interface to verify this period manually")
			}
			f.Recording = &r
			c, err := m.onvifClient(d, o.Pin)
			if err == nil {
				clock, err := c.Read(ctx, "GetSystemDateAndTime", s)
				if err == nil {
					r.Clock, _ = diagnostics.ScrubXML(clock, s.Username, s.Password)
				}
				clear(clock)
			}
			if o.Provider == "isapi" {
				for _, id := range []string{"isapi-storage", "isapi-clock"} {
					p := m.inspectField(ctx, d, s, o.Pin, "recordings", id)
					f.Probes = append(f.Probes, p)
					f.Run.Steps = append(f.Run.Steps, p.Step)
					if p.Step.Status != "verified" {
						r.Notes = append(r.Notes, p.Capability.Purpose+": "+p.Step.Status+"; inspect storage/schedule health in the recorder UI")
					}
					if id == "isapi-clock" && p.Step.Status == "verified" {
						r.Clock = p.Response
					}
				}
			}
			r.Notes = append(r.Notes, "Storage/schedule/connection health is unknown unless explicitly exposed in the evidence below; no live-stream or recording-schedule assumptions are made")
			f.Recording = &r
			return nil
		}
		if o.Kind == "imaging" {
			c, err := m.onvifClient(d, o.Pin)
			if err != nil {
				return err
			}
			catalog, err := c.MediaProfiles(ctx, s, 0)
			if err != nil {
				return err
			}
			f.Profiles = catalog.Profiles
			profile := o.ProfileID
			if profile == "" && len(catalog.Profiles) > 0 {
				profile = catalog.Profiles[0].ID
			}
			v, err := c.ReadImaging(ctx, catalog, profile, s)
			f.Imaging = &v
			return err
		}
		return nil
	})
}

type ImageChange struct {
	DeviceRead
	ProfileID    string `json:"profileID"`
	Field        string `json:"field"`
	Value        string `json:"value"`
	Previous     string `json:"previous"`
	Serial       string `json:"serial"`
	Confirmation string `json:"confirmation"`
}

func (m *Manager) ApplyImage(o ImageChange) error {
	if o.Serial == "" || o.Confirmation != "APPLY "+o.Serial {
		return errors.New("confirm APPLY followed by the exact device serial")
	}
	return m.start("image-setting", o.SiteID, func(ctx context.Context) error {
		d, s, e := m.readCredential(o.DeviceRead)
		if e != nil {
			return e
		}
		defer s.Clear()
		if d.Serial != o.Serial {
			return errors.New("selected device identity changed")
		}
		c, e := m.onvifClient(d, o.Pin)
		if e != nil {
			return e
		}
		cat, e := c.MediaProfiles(ctx, s, 0)
		if e != nil {
			return e
		}
		state, e := c.SetImageSetting(ctx, cat, o.ProfileID, o.Serial, o.Field, o.Value, o.Previous, s)
		_, saveErr := m.document(o.SiteID, o.DeviceID, "image-change", "Image setting · "+state, map[string]string{"field": o.Field, "previous": o.Previous, "proposed": o.Value, "state": state, "at": time.Now().UTC().Format(time.RFC3339Nano), "profileID": o.ProfileID})
		m.progress(1, 1, "Image change "+state+"; verify Live View. Restore requires a separate confirmed change.")
		if e != nil {
			return e
		}
		return saveErr
	})
}

type LabelOptions struct {
	SiteID   string `json:"siteID"`
	DeviceID string `json:"deviceID"`
	SwitchID string `json:"switchID"`
	WidthMM  int    `json:"widthMM"`
	HeightMM int    `json:"heightMM"`
	Copies   int    `json:"copies"`
	ShowIP   bool   `json:"showIP"`
	ShowMAC  bool   `json:"showMAC"`
	SiteText string `json:"siteText"`
	Location string `json:"location"`
	Channel  string `json:"channel"`
}
type LabelAsset struct {
	ID       string `json:"id"`
	SiteID   string `json:"siteID"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Location string `json:"location"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Removed  bool   `json:"removed"`
}

func (m *Manager) LabelAssets(site string) ([]LabelAsset, error) {
	rows, e := m.Store.Records(site)
	if e != nil {
		return nil, e
	}
	out := []LabelAsset{}
	for _, r := range rows {
		name := r.Device.Name
		if name == "" {
			name = r.Device.Model
		}
		out = append(out, LabelAsset{ID: r.ID, SiteID: site, Kind: "device", Name: name, Location: r.Location, IP: r.Device.IPv4, MAC: r.Device.MAC, Removed: r.Removed})
	}
	switches, e := m.Store.Switches(site)
	if e != nil {
		return nil, e
	}
	for _, a := range switches {
		out = append(out, LabelAsset{ID: a.ID, SiteID: site, Kind: "switch", Name: a.Name, Location: a.Location, IP: a.Host})
	}
	return out, nil
}
func (m *Manager) Label(o LabelOptions) ([]byte, error) {
	v, e := m.Store.View(o.SiteID)
	if e != nil {
		return nil, e
	}
	assets, e := m.LabelAssets(o.SiteID)
	if e != nil {
		return nil, e
	}
	if (o.DeviceID == "") == (o.SwitchID == "") {
		return nil, errors.New("choose one saved camera/NVR or switch")
	}
	var a *LabelAsset
	for i := range assets {
		if assets[i].ID == o.DeviceID && assets[i].Kind == "device" || assets[i].ID == o.SwitchID && assets[i].Kind == "switch" {
			a = &assets[i]
		}
	}
	if a == nil {
		return nil, errors.New("saved asset not found")
	}
	siteText := o.SiteText
	if siteText == "" {
		siteText = v.Site.Name
	}
	location := o.Location
	if location == "" {
		location = a.Location
	}
	if location == "" {
		location = a.Name
	}
	lines := []string{siteText, location}
	if o.ShowIP {
		lines = append(lines, a.IP)
	}
	if o.ShowMAC && a.MAC != "" {
		lines = append(lines, a.MAC)
	}
	if o.Channel != "" {
		lines = append(lines, "NVR/ch "+o.Channel)
	}
	return diagnostics.LabelSVG(diagnostics.Label{SiteID: o.SiteID, AssetID: a.ID, Lines: lines, WidthMM: o.WidthMM, HeightMM: o.HeightMM, Copies: o.Copies})
}
func (m *Manager) SaveLabel(o LabelOptions) (sites.Document, error) {
	if _, e := m.Label(o); e != nil {
		return sites.Document{}, e
	}
	return m.document(o.SiteID, o.DeviceID, "asset-label", "Equipment label", o)
}
func (m *Manager) LocateAsset(code string) (LabelAsset, error) {
	site, id, e := diagnostics.ParseAssetCode(code)
	if e != nil {
		return LabelAsset{}, e
	}
	rows, e := m.LabelAssets(site)
	if e != nil {
		return LabelAsset{}, e
	}
	for _, r := range rows {
		if r.ID == id {
			return r, nil
		}
	}
	return LabelAsset{}, errors.New("asset is not in this local inventory; import its site backup first")
}

type BundleOptions struct {
	SiteID        string   `json:"siteID"`
	DeviceID      string   `json:"deviceID"`
	Documents     []string `json:"documents"`
	Anonymise     bool     `json:"anonymise"`
	Screenshots   []string `json:"screenshots"`
	ApproveImages bool     `json:"approveImages"`
}

func (m *Manager) PrepareSupport(o BundleOptions) (diagnostics.Bundle, error) {
	d, _, e := m.fieldDevice(DeviceRead{SiteID: o.SiteID, DeviceID: o.DeviceID})
	if e != nil {
		return diagnostics.Bundle{}, e
	}
	files := map[string][]byte{}
	adapters, _ := platform.Interfaces()
	if o.Anonymise {
		for i := range adapters {
			adapters[i].Name = fmt.Sprintf("adapter-%d", i+1)
			adapters[i].Addresses = []string{"[anonymised]"}
			adapters[i].Gateway = "[anonymised]"
		}
	}
	files["adapters.json"] = diagnostics.SafeJSON(adapters)
	logs := m.Controller.Snapshot().Logs
	if len(logs) > 100 {
		logs = logs[len(logs)-100:]
	}
	files["logs.json"] = diagnostics.SafeJSON(logs)
	if o.Anonymise {
		for _, value := range []string{d.IPv4, d.MAC, d.Serial, d.Name} {
			if value != "" {
				files["logs.json"] = []byte(strings.ReplaceAll(string(files["logs.json"]), value, "[anonymised]"))
			}
		}
	}

	identity := map[string]string{"model": d.Model, "firmware": d.Firmware, "vendor": d.Vendor}
	if !o.Anonymise {
		identity["ip"] = d.IPv4
		identity["mac"] = d.MAC
		identity["serial"] = d.Serial
	}
	files["device.json"] = diagnostics.SafeJSON(identity)
	for i, id := range o.Documents {
		doc, e := m.Store.Document(o.SiteID, id)
		if e != nil {
			return diagnostics.Bundle{}, e
		}
		if doc.DeviceID != "" && doc.DeviceID != o.DeviceID {
			return diagnostics.Bundle{}, errors.New("bundle evidence belongs to another device")
		}
		if doc.Kind != "field-evidence" && doc.Kind != "stream-diagnostic" && doc.Kind != "walk-test" && doc.Kind != "anpr-session" {
			return diagnostics.Bundle{}, errors.New("only diagnostic metadata may be included; event text, footage and backups are excluded")
		}
		raw := diagnostics.SafeJSON(doc.Data)
		if doc.Kind == "walk-test" {
			var walk WalkTest
			if json.Unmarshal(doc.Data, &walk) != nil {
				return diagnostics.Bundle{}, errors.New("invalid event assessment")
			}
			counts := map[string]int{}
			for _, a := range walk.Assessments {
				counts[a.Result]++
			}
			raw = diagnostics.SafeJSON(map[string]any{"kind": "event-test-summary", "started": walk.Started, "completed": walk.Completed, "received": walk.Total, "result": walk.Result, "assessments": counts, "note": "Event payloads, notes, images and identifiers omitted"})
		}
		if o.Anonymise {
			for _, secret := range []string{d.IPv4, d.MAC, d.Serial, d.Name, o.SiteID, o.DeviceID} {
				if secret != "" {
					raw = []byte(strings.ReplaceAll(string(raw), secret, "[anonymised]"))
				}
			}
		}
		files[fmt.Sprintf("evidence-%02d.json", i+1)] = raw
	}
	if len(o.Screenshots) > 4 || len(o.Screenshots) > 0 && !o.ApproveImages {
		return diagnostics.Bundle{}, errors.New("explicitly approve up to four screenshots; pixel content is not anonymised")
	}
	pictures := map[string][]byte{}
	for i, id := range o.Screenshots {
		a, raw, err := m.Store.AssetData(o.SiteID, id)
		if err != nil {
			return diagnostics.Bundle{}, err
		}
		if a.Kind != "snapshot" || a.Encrypted || a.DeviceID != o.DeviceID {
			clear(raw)
			return diagnostics.Bundle{}, errors.New("choose a screenshot of this selected device")
		}
		pictures[fmt.Sprintf("approved-screenshot-%02d.png", i+1)] = raw
	}
	defer func() {
		for _, v := range pictures {
			clear(v)
		}
	}()
	b, e := diagnostics.PrepareBundleImages(files, pictures)
	if e != nil {
		return b, e
	}
	m.mu.Lock()
	m.support = b
	m.mu.Unlock()
	return b, nil
}
func (m *Manager) ExportSupport(digest, passphrase string) ([]byte, error) {
	m.mu.Lock()
	b := m.support
	m.mu.Unlock()
	return b.Export(digest, passphrase)
}

func (m *Manager) StartTrace(o diagnostics.TraceOptions, path string) error {
	return m.start("raw-trace", "", func(ctx context.Context) error {
		m.progress(0, o.Seconds, "Capturing sensitive raw packets for one approved host; no bundle inclusion or upload")
		err := diagnostics.CaptureTrace(ctx, o, path)
		if err == nil {
			m.progress(o.Seconds, o.Seconds, "Raw trace saved separately; review before sharing and remove when no longer needed")
		}
		return err
	})
}

func (m *Manager) inspectField(ctx context.Context, d devices.Device, s vault.Secret, pin, kind, id string) diagnostics.Probe {
	cap, _ := diagnostics.FindCapability(id)
	p := diagnostics.Probe{Capability: cap, Step: diagnostics.Step{Category: kind, Test: cap.Operation, Protocol: cap.Protocol, At: time.Now().UTC().Format(time.RFC3339Nano)}}
	start := time.Now()
	var data []byte
	var err error
	if cap.Protocol == "onvif" {
		c, e := m.onvifClient(d, pin)
		err = e
		if e == nil {
			var meta onvif.ProbeMetadata
			data, meta, err = c.Inspect(ctx, cap.Operation, s)
			p.Step.Code = meta.Code
			p.ContentType = sanitize.Label(meta.ContentType)
		}
	} else {
		c, e := m.fieldISAPI(ctx, d, pin)
		err = e
		if e == nil {
			var meta isapi.ReadMetadata
			data, meta, err = c.Inspect(ctx, cap.Operation, s)
			p.Step.Code = meta.Code
			p.ContentType = sanitize.Label(meta.ContentType)
		}
	}
	p.Step.Status = statusOf(err)
	if err != nil {
		p.Step.Evidence = sanitize.Text(err.Error())
	} else {
		p.Endpoints = diagnostics.ServiceEndpoints(data, d.IPv4, s.Username, s.Password, s.PrivacyPassword)
		p.Response, err = diagnostics.ScrubXML(data, s.Username, s.Password, s.PrivacyPassword)
		if err != nil {
			p.Step.Status = "invalid-payload"
			p.Step.Evidence = err.Error()
		} else {
			p.Step.Evidence = "Read succeeded; XML scrubbed. Advertisement is not proof of every advertised feature."
		}
	}
	clear(data)
	p.Step.DurationMS = time.Since(start).Milliseconds()

	return p
}
