package toolkit

import (
	"bytes"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/remote"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
)

func advancedFixture(t *testing.T) (*Manager, sites.Site, sites.Record) {
	t.Helper()
	dir := t.TempDir()
	controller := app.New(filepath.Join(dir, "settings.json"), nil)
	m, e := New(filepath.Join(dir, "sites.sqlite"), controller, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Close(); controller.Close() })
	site, e := m.Store.SaveSite(sites.Site{Name: "SYNTHETIC site", Customer: "Synthetic customer"})
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.Store.SaveRecord(sites.Record{SiteID: site.ID, CredentialMode: "none", Device: devices.Device{ID: "fixture", Serial: "fixture-serial", IPv4: "192.0.2.5", Model: "Synthetic camera"}})
	if e != nil {
		t.Fatal(e)
	}
	return m, site, r
}
func TestAdvancedSiteIsolationAndImmutableReport(t *testing.T) {
	m, site, r := advancedFixture(t)
	other, _ := m.Store.SaveSite(sites.Site{Name: "Other synthetic site"})
	var imageData bytes.Buffer
	png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 40, 20)))
	a, e := m.Store.SaveAsset(sites.Asset{SiteID: site.ID, Kind: "plan", Title: "Synthetic plan"}, imageData.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(installation.Plan{ImageID: a.ID, Markers: []installation.Marker{{ID: "marker", DeviceID: r.ID, X: .5, Y: .5, Label: "Synthetic"}}})
	d, e := m.SaveAdvanced(sites.Document{SiteID: site.ID, Kind: "plan", Title: "Plan", Data: raw})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.SaveAdvanced(sites.Document{SiteID: other.ID, Kind: "plan", Title: "Bad reference", Data: raw}); e == nil {
		t.Fatal("cross-site image/device accepted")
	}
	form := installation.Commissioning{Title: "Synthetic handover", Technician: "Māori fixture", Date: "2026-01-01", Checks: []installation.Check{{DeviceID: r.ID, Test: "live-video", Result: "not-tested", Source: "manual"}}}
	raw, _ = json.Marshal(form)
	check, e := m.SaveAdvanced(sites.Document{SiteID: site.ID, Kind: "commissioning", Title: form.Title, Data: raw})
	if e != nil {
		t.Fatal(e)
	}
	snapshot, e := m.CreateReport(ReportOptions{SiteID: site.ID, ChecklistID: check.ID, DocumentIDs: []string{d.ID}, Redact: true})
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(snapshot.Data, []byte("192.0.2.5")) || bytes.Contains(snapshot.Data, []byte("fixture-serial")) {
		t.Fatal("identity not redacted")
	}
	if _, e = m.Store.SaveDocument(snapshot); e == nil {
		t.Fatal("report snapshot mutable")
	}
	pdf, e := m.ExportReport(site.ID, snapshot.ID, "pdf")
	if e != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal(e)
	}
	if e = m.Store.DeleteAsset(site.ID, a.ID); e == nil {
		t.Fatal("referenced image deleted")
	}
}
func TestRemoteObservationImportStaysInSelectedSite(t *testing.T) {
	m, site, _ := advancedFixture(t)
	other, _ := m.Store.SaveSite(sites.Site{Name: "Other synthetic site"})
	ob := remote.Observation{Target: remote.Target{Host: "synthetic-host"}, Report: remote.Report{Schema: 1, Version: "fixture", Devices: []devices.Device{{ID: "remote-fixture", Serial: "remote-fixture-serial", IPv4: "198.51.100.5", Model: "Synthetic remote camera"}}}}
	doc, e := m.document(site.ID, "", "remote-observation", "Synthetic remote scan", ob)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.ImportRemote(other.ID, doc.ID, []string{"remote-fixture"}); e == nil {
		t.Fatal("cross-site import accepted")
	}
	if _, e = m.ImportRemote(site.ID, doc.ID, []string{"remote-fixture"}); e != nil {
		t.Fatal(e)
	}
	rows, _ := m.Store.Records(site.ID)
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	rows, _ = m.Store.Records(other.ID)
	if len(rows) != 0 {
		t.Fatal("other site changed")
	}
	if len(m.Controller.Snapshot().Devices) != 0 {
		t.Fatal("remote devices merged into local broadcast inventory")
	}
}
func TestWalkEvidenceRequiresFinishedSessionAndRealEventID(t *testing.T) {
	m, site, r := advancedFixture(t)
	m.walk = WalkTest{ID: "synthetic-walk", SiteID: site.ID, DeviceID: r.ID, Running: true, Events: []onvif.CameraEvent{{EventID: "1", Type: "motion", State: "active"}}}
	if _, e := m.SaveWalk("synthetic-walk", "pass", "", nil, nil); e == nil {
		t.Fatal("active walk saved")
	}
	m.walk.Running = false
	if _, e := m.SaveWalk("synthetic-walk", "pass", "", nil, []EventAssessment{{EventID: "other", Result: "pass"}}); e == nil {
		t.Fatal("invented event graded")
	}
	d, e := m.SaveWalk("synthetic-walk", "pass", "password=SENTINEL", nil, []EventAssessment{{EventID: "1", Result: "pass", Notes: "Synthetic motion observed"}})
	if e != nil || strings.Contains(string(d.Data), "SENTINEL") {
		t.Fatal(e)
	}
}
