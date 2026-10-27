package sites

import (
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"testing"
	"time"
)

func TestReplacementGuardsHistoryAndPortableReferences(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic replacement")
	old, e := s.SaveRecord(Record{SiteID: site.ID, CredentialMode: "none", Device: syntheticDevice()})
	if e != nil {
		t.Fatal(e)
	}
	fresh := syntheticDevice()
	fresh.MAC = "02:00:00:00:00:02"
	fresh.Serial = "SYNTHETIC-002"
	fresh.IPv4 = "192.0.2.41"
	replacement, e := s.SaveRecord(Record{SiteID: site.ID, CredentialMode: "none", Device: fresh})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateReplacement(site.ID, old.ID, old.ID, ""); e == nil {
		t.Fatal("self replacement")
	}
	other := addSite(t, s, "Other synthetic site")
	if _, e = s.CreateReplacement(other.ID, old.ID, replacement.ID, ""); e == nil {
		t.Fatal("cross-site replacement")
	}
	doc, e := s.CreateReplacement(site.ID, old.ID, replacement.ID, "Synthetic planning")
	if e != nil {
		t.Fatal(e)
	}
	confirm := "REPLACE " + fresh.Key()
	if _, e = s.CompleteReplacement(site.ID, doc.ID, doc.Updated, confirm, "Checks"); e == nil {
		t.Fatal("unverified plan retired a device")
	}
	var plan Replacement
	json.Unmarshal(doc.Data, &plan)
	plan.State = "verified"
	plan.VerifiedIP = fresh.IPv4
	plan.VerifiedAt = time.Now().Add(-time.Hour)
	doc.Data, _ = json.Marshal(plan)
	doc, e = s.SaveDocument(doc)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteReplacement(site.ID, doc.ID, doc.Updated, confirm, "Checks"); e == nil {
		t.Fatal("expired verification accepted")
	}
	plan.VerifiedAt = time.Now()
	doc.Data, _ = json.Marshal(plan)
	doc, e = s.SaveDocument(doc)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteReplacement(site.ID, doc.ID, doc.Updated, "wrong", "Checks"); e == nil {
		t.Fatal("wrong confirmation accepted")
	}
	old.Device.Serial = "DIFFERENT-SERIAL"
	old, e = s.SaveRecord(old)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteReplacement(site.ID, doc.ID, doc.Updated, confirm, "Checks"); e == nil {
		t.Fatal("old identity changed")
	}
	old.Device = plan.OldIdentity
	old, e = s.SaveRecord(old)
	if e != nil {
		t.Fatal(e)
	}
	doc, e = s.CompleteReplacement(site.ID, doc.ID, doc.Updated, confirm, "Synthetic checks: stream passed; NVR and recording require manual follow-up")
	if e != nil {
		t.Fatal(e)
	}
	rows, _ := s.Records(site.ID)
	retired := 0
	for _, r := range rows {
		if r.Removed {
			retired++
			if r.ID != old.ID {
				t.Fatal("wrong retirement")
			}
		}
	}
	if retired != 1 {
		t.Fatal(rows)
	}
	if _, e = s.CompleteReplacement(site.ID, doc.ID, doc.Updated, confirm, "Again"); e == nil {
		t.Fatal("reconciled twice")
	}
	if e = s.Unlock("Synthetic replacement key"); e != nil {
		t.Fatal(e)
	}
	backup, e := s.Backup(site.ID, "Synthetic backup passphrase")
	if e != nil {
		t.Fatal(e)
	}
	target := testStore(t)
	target.Unlock("Synthetic destination key")
	restored, e := target.Restore(backup, "Synthetic backup passphrase")
	if e != nil {
		t.Fatal(e)
	}
	docs, e := target.Documents(restored.ID, "replacement")
	if e != nil || len(docs) != 1 {
		t.Fatal(docs, e)
	}
	var restoredPlan Replacement
	json.Unmarshal(docs[0].Data, &restoredPlan)
	if restoredPlan.OldDeviceID == old.ID || restoredPlan.NewDeviceID == replacement.ID || restoredPlan.State != "completed" {
		t.Fatal("references/state lost", restoredPlan)
	}
	rows, _ = target.Records(restored.ID)
	for _, r := range rows {
		if r.ID == restoredPlan.NewDeviceID && devices.ConflictingIdentity(r.Device, fresh) {
			t.Fatal("identity changed on restore")
		}
	}
}
