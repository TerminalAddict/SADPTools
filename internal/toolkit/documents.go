package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/audit"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/remote"
	"github.com/TerminalAddict/SADPTools/internal/reports"
	"github.com/TerminalAddict/SADPTools/internal/retention"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"time"
)

type ChannelLinks struct {
	Links map[string]string `json:"links"`
}
type AdvancedView struct {
	Documents []sites.Document `json:"documents"`
	Assets    []sites.Asset    `json:"assets"`
	Checks    []string         `json:"checks"`
}

func (m *Manager) Advanced(site string) (AdvancedView, error) {
	out := AdvancedView{Checks: installation.Checks}
	var e error
	out.Documents, e = m.Store.Documents(site, "")
	if e != nil {
		return out, e
	}
	out.Assets, e = m.Store.Assets(site)
	return out, e
}
func (m *Manager) imageBelongs(site, id, kind string) error {
	items, e := m.Store.Assets(site)
	if e != nil {
		return e
	}
	for _, a := range items {
		if a.ID == id && !a.Encrypted && (kind == "" || a.Kind == kind) {
			return nil
		}
	}
	return errors.New("choose an image belonging to this site")
}
func (m *Manager) SaveAdvanced(d sites.Document) (sites.Document, error) {
	rows, e := m.Store.Records(d.SiteID)
	if e != nil {
		return d, e
	}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	var value any
	switch d.Kind {
	case "plan":
		var p installation.Plan
		if json.Unmarshal(d.Data, &p) != nil {
			return d, errors.New("invalid plan")
		}
		if e = p.Validate(ids); e != nil {
			return d, e
		}
		if e = m.imageBelongs(d.SiteID, p.ImageID, "plan"); e != nil {
			return d, e
		}
		for i := range p.Markers {
			p.Markers[i].Label = sanitize.Label(p.Markers[i].Label)
			p.Markers[i].Notes = sanitize.Text(p.Markers[i].Notes)
		}
		p.Notes = sanitize.Text(p.Notes)
		value = p
	case "retention":
		var s retention.Scenario
		if json.Unmarshal(d.Data, &s) != nil {
			return d, errors.New("invalid storage scenario")
		}
		if _, e = retention.Calculate(s); e != nil {
			return d, e
		}
		s.Name = sanitize.Label(s.Name)
		for i := range s.Channels {
			s.Channels[i].Label = sanitize.Label(s.Channels[i].Label)
		}
		value = s
	case "commissioning":
		var c installation.Commissioning
		if json.Unmarshal(d.Data, &c) != nil {
			return d, errors.New("invalid commissioning checklist")
		}
		if e = c.Validate(ids); e != nil {
			return d, e
		}
		for _, check := range c.Checks {
			for _, id := range check.EvidenceIDs {
				evidence, err := m.Store.Document(d.SiteID, id)
				if err != nil {
					if m.imageBelongs(d.SiteID, id, "snapshot") != nil {
						return d, errors.New("checklist evidence must belong to this site")
					}
				} else if evidence.DeviceID != "" && evidence.DeviceID != check.DeviceID {
					return d, errors.New("checklist evidence belongs to another device")
				}
			}
		}
		if c.BrandingID != "" {
			b, err := m.Store.Document(d.SiteID, c.BrandingID)
			if err != nil || b.Kind != "branding" {
				return d, errors.New("choose this site's branding template")
			}
		}
		value = c
	case "branding":
		var b installation.Branding
		if json.Unmarshal(d.Data, &b) != nil {
			return d, errors.New("invalid branding")
		}
		if b.LogoID != "" {
			if e = m.imageBelongs(d.SiteID, b.LogoID, "branding"); e != nil {
				return d, e
			}
		}
		b.Company = sanitize.Label(b.Company)
		b.Details = sanitize.Text(b.Details)
		value = b
	case "channel-links":
		var l ChannelLinks
		if json.Unmarshal(d.Data, &l) != nil || len(l.Links) > 1024 || !ids[d.DeviceID] {
			return d, errors.New("invalid channel links")
		}
		for channel, id := range l.Links {
			if channel == "" || len(channel) > 64 || id != "" && !ids[id] {
				return d, errors.New("channel link must reference a device in this site")
			}
		}
		value = l
	case "ssh-target":
		var t remote.Target
		if json.Unmarshal(d.Data, &t) != nil {
			return d, errors.New("invalid SSH target")
		}
		if e = t.Validate(); e != nil {
			return d, e
		}
		if t.CredentialID != "" {
			view, err := m.Store.View(d.SiteID)
			if err != nil {
				return d, err
			}
			found := false
			for _, p := range view.Profiles {
				if p.ID == t.CredentialID {
					found = true
				}
			}
			if !found {
				return d, errors.New("SSH credential must belong to this site")
			}
		}
		value = t
	default:
		return d, errors.New("this document kind cannot be edited directly")
	}
	d.Title = sanitize.Label(d.Title)
	d.Version = 1
	d.Data, e = json.Marshal(value)
	if e != nil {
		return d, e
	}
	return m.Store.SaveDocument(d)
}

type ReportOptions struct {
	SiteID      string   `json:"siteID"`
	ChecklistID string   `json:"checklistID"`
	DocumentIDs []string `json:"documentIDs"`
	ImageIDs    []string `json:"imageIDs"`
	Redact      bool     `json:"redact"`
}

func (m *Manager) CreateReport(o ReportOptions) (sites.Document, error) {
	view, e := m.Store.View(o.SiteID)
	if e != nil {
		return sites.Document{}, e
	}
	doc, e := m.Store.Document(o.SiteID, o.ChecklistID)
	if e != nil || doc.Kind != "commissioning" {
		return sites.Document{}, errors.New("save a commissioning checklist first")
	}
	var form installation.Commissioning
	if json.Unmarshal(doc.Data, &form) != nil {
		return sites.Document{}, errors.New("invalid checklist")
	}
	snap := reports.Snapshot{Version: 1, At: time.Now().UTC().Format(time.RFC3339Nano), Title: form.Title, Site: sanitize.Label(view.Site.Name), Customer: sanitize.Label(view.Site.Customer), Address: sanitize.Text(view.Site.Address), Technician: form.Technician, Date: form.Date, Project: form.Project, Summary: form.Summary, Signoff: form.Signoff, Redacted: o.Redact, Sections: []reports.Section{}, Figures: []reports.Figure{}}
	if o.Redact {
		snap.Address = "[redacted]"
		snap.Customer = "[redacted]"
	}
	names := map[string]string{}
	equipment := reports.Section{Title: "Equipment inventory", Headers: []string{"Name / location", "Model", "IP / MAC", "Serial / firmware", "Last observed"}, Rows: [][]string{}, Note: "Saved site inventory; last observation is not a current reachability test."}
	for _, r := range view.Devices {
		if r.Removed {
			continue
		}
		d := r.Device
		names[r.ID] = sanitize.Label(d.Name)
		if names[r.ID] == "" {
			names[r.ID] = d.Model
		}
		network := d.IPv4 + " / " + d.MAC
		serial := d.Serial
		if o.Redact {
			network = "[redacted]"
			serial = "[redacted]"
		}
		equipment.Rows = append(equipment.Rows, []string{sanitize.Text(d.Name + " · " + r.Location), sanitize.Label(d.Model), network, serial + " / " + sanitize.Label(d.Firmware), d.LastSeen.UTC().Format(time.RFC3339)})
	}
	snap.Sections = append(snap.Sections, equipment)
	checks := reports.Section{Title: "Commissioning matrix", Headers: []string{"Device", "Check", "Result", "Provenance / time", "Notes"}, Rows: [][]string{}, Note: "Manual results are technician attestations. Not tested remains distinct from failure."}
	issues := reports.Section{Title: "Outstanding issues / actions", Headers: []string{"Device", "Check", "Action / notes"}, Rows: [][]string{}}
	for _, c := range form.Checks {
		checks.Rows = append(checks.Rows, []string{names[c.DeviceID], c.Test, c.Result, c.Source + " · " + c.At, sanitize.Text(c.Notes)})
		if c.Result == "fail" || c.Result == "not-tested" {
			issues.Rows = append(issues.Rows, []string{names[c.DeviceID], c.Test, c.Result + " · " + sanitize.Text(c.Notes)})
		}
	}
	snap.Sections = append(snap.Sections, checks, issues)
	addImage := func(id, caption string, markers []installation.Marker) error {
		if e := m.imageBelongs(o.SiteID, id, ""); e != nil {
			return e
		}
		for _, f := range snap.Figures {
			if f.ImageID == id {
				return nil
			}
		}
		snap.Figures = append(snap.Figures, reports.Figure{ImageID: id, Caption: sanitize.Text(caption), Markers: markers})
		return nil
	}
	if form.BrandingID != "" {
		b, e := m.Store.Document(o.SiteID, form.BrandingID)
		if e != nil {
			return sites.Document{}, e
		}
		var branding installation.Branding
		if json.Unmarshal(b.Data, &branding) != nil {
			return sites.Document{}, errors.New("invalid branding template")
		}
		snap.Company = branding.Company
		snap.BusinessDetails = branding.Details
		if branding.LogoID != "" {
			if e = addImage(branding.LogoID, "Company branding", nil); e != nil {
				return sites.Document{}, e
			}
		}
	}
	if len(o.DocumentIDs) > 100 || len(o.ImageIDs) > 64 {
		return sites.Document{}, errors.New("too many report attachments")
	}
	for _, id := range o.DocumentIDs {
		d, e := m.Store.Document(o.SiteID, id)
		if e != nil {
			return sites.Document{}, e
		}
		switch d.Kind {
		case "field-evidence", "anpr-session", "image-change", "nvr-channels", "stream-diagnostic", "walk-test", "retention", "plan":
		default:
			return sites.Document{}, errors.New("choose diagnostic, channel, walk-test, retention or plan evidence")
		}
		if d.Kind == "plan" {
			var p installation.Plan
			if json.Unmarshal(d.Data, &p) != nil {
				return sites.Document{}, errors.New("invalid plan")
			}
			if e = addImage(p.ImageID, d.Title+" · manual approximate camera placements", p.Markers); e != nil {
				return sites.Document{}, e
			}
		}
		section, e := evidenceSection(d, o.Redact, names)
		if e != nil {
			return sites.Document{}, e
		}
		snap.Sections = append(snap.Sections, section)
	}
	// Audit runs are snapshots already; include only explicitly validated findings.
	for _, run := range view.Runs {
		if run.Kind != "audit" {
			continue
		}
		full, e := m.Store.Run(o.SiteID, run.ID)
		if e != nil {
			continue
		}
		var report audit.Report
		if json.Unmarshal([]byte(full.Data), &report) != nil {
			continue
		}
		rows := [][]string{}
		for _, f := range report.Findings {
			rows = append(rows, []string{names[f.RecordID], sanitize.Text(f.Check), f.Status, sanitize.Text(f.Evidence), sanitize.Text(f.Source) + " · " + f.At})
		}
		snap.Sections = append(snap.Sections, reports.Section{Title: "Latest saved audit", Note: run.At, Headers: []string{"Device", "Check", "Status", "Evidence", "Source / time"}, Rows: rows})
		break
	}
	for _, id := range o.ImageIDs {
		if e = addImage(id, "Selected site evidence", nil); e != nil {
			return sites.Document{}, e
		}
	}
	if o.Redact {
		values := []string{view.Site.Customer, view.Site.Address}
		for _, r := range view.Devices {
			d := r.Device
			values = append(values, d.IPv4, d.MAC, d.Serial, d.Gateway, d.SubnetMask)
		}
		snap = reports.RedactSnapshot(snap, values)
	}
	if e = snap.Validate(); e != nil {
		return sites.Document{}, e
	}
	return m.document(o.SiteID, "", "report", fmt.Sprintf("%s · %s", snap.Title, form.Date), snap)
}
func (m *Manager) ExportReport(site, id, format string) ([]byte, error) {
	doc, e := m.Store.Document(site, id)
	if e != nil || doc.Kind != "report" {
		return nil, errors.New("report snapshot not found")
	}
	var snap reports.Snapshot
	if json.Unmarshal(doc.Data, &snap) != nil {
		return nil, errors.New("report snapshot invalid")
	}
	return reports.Commissioning(format, snap, func(id string) ([]byte, error) {
		a, b, e := m.Store.AssetData(site, id)
		if e != nil {
			return nil, e
		}
		if a.Encrypted || a.MIME != "image/png" {
			return nil, errors.New("report references a non-image asset")
		}
		return b, nil
	})
}
