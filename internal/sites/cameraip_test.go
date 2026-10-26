package sites

import (
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"testing"
)

func TestAddCameraIPCreatesHistoryAndPreservesStableDuplicate(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "SYNTHETIC routed site")
	d := syntheticDevice()
	d.Protocols = []string{"direct-ip", "isapi"}
	d.Interfaces = []devices.Interface{}
	r := Record{SiteID: site.ID, Device: d, Notes: "Curated synthetic note", CredentialMode: "none", ManualFields: []string{"ipv4"}}
	added, e := s.AddCameraIP(r, `{"identified":true,"evidence":[]}`)
	if e != nil || added.Existing || added.Record.ID == "" {
		t.Fatal(added, e)
	}
	h, e := s.History(site.ID, added.Record.ID)
	if e != nil || len(h) != 2 {
		t.Fatal(h, e)
	}
	r.Device.IPv4 = "198.51.100.10"
	r.Notes = "Must not replace saved note"
	same, e := s.AddCameraIP(r, `{}`)
	if e != nil || !same.Existing || encode(same.Record) != encode(added.Record) {
		t.Fatal("stable duplicate changed", same, e)
	}
	h, _ = s.History(site.ID, added.Record.ID)
	if len(h) != 2 {
		t.Fatal("duplicate wrote history")
	}
	r.Device = syntheticDevice()
	r.Device.Serial = "OTHER-SYNTHETIC"
	r.Device.MAC = "02:00:00:00:00:02"
	if _, e = s.AddCameraIP(r, `{}`); e == nil {
		t.Fatal("IP treated as identity")
	}
	r.Device = devices.Device{IPv4: d.IPv4, Protocols: []string{"direct-ip"}}
	if _, e = s.AddCameraIP(r, `{}`); e == nil {
		t.Fatal("unknown identity merged by IP")
	}
	other := addSite(t, s, "Other SYNTHETIC site")
	r.SiteID = other.ID
	if _, e = s.AddCameraIP(r, `{}`); e != nil {
		t.Fatal("site isolation", e)
	}
	rows, _ := s.Records(site.ID)
	if len(rows) != 1 || rows[0].Notes != added.Record.Notes {
		t.Fatal("original inventory changed")
	}
}
func TestAddCameraIPUnknownIdentityAndCredentialScope(t *testing.T) {
	s := testStore(t)
	a := addSite(t, s, "SYNTHETIC A")
	b := addSite(t, s, "SYNTHETIC B")
	if e := s.Unlock("Synthetic test-only passphrase"); e != nil {
		t.Fatal(e)
	}
	p, e := s.SaveProfile(Profile{SiteID: a.ID, Label: "Synthetic"}, vault.Secret{Username: "fixture", Password: "SYNTHETIC-PASSWORD"})
	if e != nil {
		t.Fatal(e)
	}
	r := Record{SiteID: b.ID, CredentialMode: "profile", CredentialID: p.ID, Device: devices.Device{IPv4: "198.51.100.10", Protocols: []string{"direct-ip"}}}
	if _, e = s.AddCameraIP(r, `{}`); e == nil {
		t.Fatal("cross-site profile accepted")
	}
	rows, _ := s.Records(b.ID)
	if len(rows) != 0 {
		t.Fatal("failed transaction left a record")
	}
	r.SiteID = a.ID
	added, e := s.AddCameraIP(r, `{"identified":false}`)
	if e != nil {
		t.Fatal(e)
	}
	if added.Record.Device.ID != "" || added.Record.Device.MAC != "" || added.Record.Device.SADPCompatible() {
		t.Fatal("fabricated identity", added)
	}
	secret, _, e := s.DeviceSecret(a.ID, added.Record.ID)
	if e != nil || secret.Password != "SYNTHETIC-PASSWORD" {
		t.Fatal("explicit profile not assigned", e)
	}
	a.Archived = true
	if _, e = s.SaveSite(a); e != nil {
		t.Fatal(e)
	}
	r.Device.IPv4 = "198.51.100.11"
	if _, e = s.AddCameraIP(r, `{}`); e == nil {
		t.Fatal("archived site accepted")
	}
}
