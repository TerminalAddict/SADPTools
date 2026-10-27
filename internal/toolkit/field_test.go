package toolkit

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/switches"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"
)

func TestLabelsRemainStableAcrossIPChangesAndIncludeSwitches(t *testing.T) {
	m, site, r := advancedFixture(t)
	code, e := diagnostics.AssetCode(site.ID, r.ID)
	if e != nil {
		t.Fatal(e)
	}
	r.Device.IPv4 = "198.51.100.7"
	if _, e = m.Store.SaveRecord(r); e != nil {
		t.Fatal(e)
	}
	a, e := m.LocateAsset(code)
	if e != nil || a.IP != r.Device.IPv4 || a.ID != r.ID {
		t.Fatal(a, e)
	}
	sw, e := m.Store.SaveSwitch(switches.Asset{SiteID: site.ID, Name: "SYNTHETIC switch", Host: "192.0.2.1", Port: 161, Version: "3", AuthProtocol: "sha256"})
	if e != nil {
		t.Fatal(e)
	}
	opts := LabelOptions{SiteID: site.ID, SwitchID: sw.ID, WidthMM: 75, HeightMM: 35, Copies: 2, ShowIP: true}
	data, e := m.Label(opts)
	if e != nil || !bytes.Contains(data, []byte(sw.Name)) || !bytes.Contains(data, []byte(sw.Host)) {
		t.Fatal(e)
	}
	if _, e = m.SaveLabel(opts); e != nil {
		t.Fatal(e)
	}
	code, _ = diagnostics.AssetCode(site.ID, sw.ID)
	a, e = m.LocateAsset(code)
	if e != nil || a.Kind != "switch" {
		t.Fatal(a, e)
	}
	other, _ := m.Store.SaveSite(sites.Site{Name: "Other synthetic site"})
	opts.SiteID = other.ID
	if _, e = m.Label(opts); e == nil {
		t.Fatal("cross-site label allowed")
	}
}
func TestSupportDefaultRedactionPreviewImageConsentAndIsolation(t *testing.T) {
	m, site, r := advancedFixture(t)
	doc, e := m.document(site.ID, r.ID, "field-evidence", "Synthetic evidence", map[string]any{"target": r.Device.IPv4, "password": "SYNTHETIC-SECRET", "nested": map[string]string{"plate": "SYNTHETIC-PLATE"}})
	if e != nil {
		t.Fatal(e)
	}
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 10, 10)))
	a, e := m.Store.SaveAsset(sites.Asset{SiteID: site.ID, DeviceID: r.ID, Kind: "snapshot"}, pic.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	opts := BundleOptions{SiteID: site.ID, DeviceID: r.ID, Documents: []string{doc.ID}, Anonymise: true}
	b, e := m.PrepareSupport(opts)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(b)
	for _, secret := range []string{r.Device.IPv4, "SYNTHETIC-SECRET", "SYNTHETIC-PLATE"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("bundle leaked identifier/secret", secret)
		}
	}
	if len(b.Images) != 0 {
		t.Fatal("default includes images")
	}
	if _, e = m.ExportSupport("stale", ""); e == nil {
		t.Fatal("stale preview exported")
	}
	opts.Screenshots = []string{a.ID}
	if _, e = m.PrepareSupport(opts); e == nil {
		t.Fatal("no image consent")
	}
	opts.ApproveImages = true
	b, e = m.PrepareSupport(opts)
	if e != nil || len(b.Images) != 1 {
		t.Fatal(e)
	}
	exported, e := m.ExportSupport(b.Digest, "")
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(exported), int64(len(exported)))
	if e != nil {
		t.Fatal(e)
	}
	if len(z.File) != len(b.Manifest.Files)+2 {
		t.Fatal("files absent from manifest")
	}
	for _, f := range z.File {
		if strings.Contains(f.Name, "/") || f.Mode().Perm() != 0600 {
			t.Fatal("unsafe ZIP entry")
		}
	}
	other, _ := m.Store.SaveSite(sites.Site{Name: "Other"})
	foreign, _ := m.document(other.ID, "", "field-evidence", "Other", map[string]int{"x": 1})
	opts.Documents = []string{foreign.ID}
	if _, e = m.PrepareSupport(opts); e == nil {
		t.Fatal("cross-site evidence allowed")
	}
}
func TestANPRRetentionClearsWithoutPollingAndGradesStayPrivate(t *testing.T) {
	m, site, r := advancedFixture(t)
	m.mu.Lock()
	m.anpr = ANPRSession{ID: "synthetic", SiteID: site.ID, DeviceID: r.ID, Total: 1, Events: []onvif.PlateEvent{{Plate: "SYNTHETIC-PLATE"}}, Completed: time.Now().UTC().Format(time.RFC3339Nano)}
	m.expireANPRLocked("synthetic", 10*time.Millisecond)
	m.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		n := len(m.anpr.Events)
		m.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("retention depends on polling")
		}
		time.Sleep(time.Millisecond)
	}
	doc, e := m.SaveANPR("synthetic", ANPRGrade{Expected: 4, Matched: 2, RecognitionFailures: 1, TransportFailures: 1})
	if e != nil || bytes.Contains(doc.Data, []byte("SYNTHETIC-PLATE")) {
		t.Fatal(e)
	}
	if _, e = m.SaveANPR("synthetic", ANPRGrade{Expected: 1, Matched: 2}); e == nil {
		t.Fatal("invented match rate")
	}
	if e = m.ApplyImage(ImageChange{Serial: "synthetic", Confirmation: "wrong"}); e == nil {
		t.Fatal("unconfirmed image write")
	}
}
