package sites

import (
	"bytes"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "sites.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func addSite(t *testing.T, s *Store, name string) Site {
	t.Helper()
	site, e := s.SaveSite(Site{Name: name, Networks: []string{"192.0.2.0/24"}, Protocols: []string{"sadp"}})
	if e != nil {
		t.Fatal(e)
	}
	return site
}
func syntheticDevice() devices.Device {
	return devices.Device{MAC: "02:00:00:00:00:01", Serial: "SYNTHETIC-001", IPv4: "192.0.2.40", Model: "SYNTHETIC", LastSeen: time.Now()}
}
func TestSitePersistenceDedupAndManualFields(t *testing.T) {
	s := testStore(t)
	a := addSite(t, s, "Synthetic site A")
	b := addSite(t, s, "Synthetic site B")
	d := syntheticDevice()
	for range 2 {
		if _, e := s.SaveObserved(a.ID, []devices.Device{d, d}); e != nil {
			t.Fatal(e)
		}
	}
	v, e := s.View(a.ID)
	if e != nil || len(v.Devices) != 1 {
		t.Fatal(v, e)
	}
	r := v.Devices[0]
	r.Notes = "Manual synthetic note"
	r.Device.Name = "Manual name"
	r.ManualFields = []string{"name"}
	r.CredentialMode = "none"
	if _, e = s.SaveRecord(r); e != nil {
		t.Fatal(e)
	}
	d.Name = "Observed name"
	d.IPv4 = "192.0.2.41"
	if _, e = s.SaveObserved(a.ID, []devices.Device{d}); e != nil {
		t.Fatal(e)
	}
	v, _ = s.View(a.ID)
	if v.Devices[0].Notes != r.Notes || v.Devices[0].Device.Name != r.Device.Name || v.Devices[0].Device.IPv4 == d.IPv4 {
		t.Fatal("scan overwrote curated inventory")
	}
	other, _ := s.View(b.ID)
	if len(other.Devices) != 0 {
		t.Fatal("site leak")
	}
	reopened, e := Open(s.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	v, e = reopened.View(a.ID)
	if e != nil || len(v.Devices) != 1 || v.Devices[0].Notes != r.Notes {
		t.Fatal(v, e)
	}
	if e = s.DeleteSite(a.ID, "wrong"); e == nil {
		t.Fatal("delete without confirmation")
	}
}
func TestCredentialsProtectedInheritanceAndPortableRestore(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic secret site")
	if e := s.Unlock("Synthetic master passphrase 123"); e != nil {
		t.Fatal(e)
	}
	secret := vault.Secret{Username: "synthetic-user", Password: "SYNTHETIC-PASSWORD-DO-NOT-EXPORT", PrivacyPassword: "SYNTHETIC-PRIVACY-PASSWORD"}
	p, e := s.SaveProfile(Profile{SiteID: site.ID, Label: "Synthetic shared credential"}, secret)
	if e != nil {
		t.Fatal(e)
	}
	site.DefaultCredential = p.ID
	site, e = s.SaveSite(site)
	if e != nil {
		t.Fatal(e)
	}
	s.SaveObserved(site.ID, []devices.Device{syntheticDevice()})
	v, _ := s.View(site.ID)
	r := v.Devices[0]
	got, source, e := s.DeviceSecret(site.ID, r.ID)
	if e != nil || source != "site default" || got.Password != secret.Password {
		t.Fatal("inheritance failed", e)
	}
	r.CredentialMode = "none"
	r, e = s.SaveRecord(r)
	if e != nil {
		t.Fatal(e)
	}
	got, source, e = s.DeviceSecret(site.ID, r.ID)
	if e != nil || source != "none" || got.Password != "" {
		t.Fatal("credential opt-out failed")
	}
	r.CredentialMode = "profile"
	r.CredentialID = p.ID
	r, e = s.SaveRecord(r)
	if e != nil {
		t.Fatal(e)
	}
	got, source, e = s.DeviceSecret(site.ID, r.ID)
	if e != nil || source != "device profile" || got.Password != secret.Password {
		t.Fatal("override failed")
	}
	exported, e := s.Export(site.ID)
	if e != nil {
		t.Fatal(e)
	}
	snapshot, _ := json.Marshal(v)
	secretJSON, _ := json.Marshal(secret)
	for _, data := range [][]byte{exported, snapshot, secretJSON} {
		if bytes.Contains(data, []byte(secret.Password)) {
			t.Fatal("plaintext leaked")
		}
	}
	s.db.Exec(`PRAGMA wal_checkpoint(FULL)`)
	for _, path := range []string{s.path, s.path + "-wal"} {
		data, _ := os.ReadFile(path)
		if bytes.Contains(data, []byte(secret.Password)) {
			t.Fatal("plaintext in SQLite")
		}
	}
	encrypted, e := s.Backup(site.ID, "Synthetic backup passphrase 123")
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(encrypted, []byte(secret.Password)) {
		t.Fatal("plaintext backup")
	}
	destination := testStore(t)
	if e = destination.Unlock("Different synthetic destination key"); e != nil {
		t.Fatal(e)
	}
	if _, e = destination.Restore(encrypted, "wrong password"); e == nil {
		t.Fatal("wrong backup password accepted")
	}
	restored, e := destination.Restore(encrypted, "Synthetic backup passphrase 123")
	if e != nil {
		t.Fatal(e)
	}
	rv, _ := destination.View(restored.ID)
	if restored.ID == site.ID || len(rv.Devices) != 1 || rv.Devices[0].ID == r.ID {
		t.Fatal("restore did not isolate identities")
	}
	got, _, e = destination.DeviceSecret(restored.ID, rv.Devices[0].ID)
	if e != nil || got.Password != secret.Password {
		t.Fatal("portable credential restore failed", e)
	}
	s.Lock()
	if _, e = s.Secret(site.ID, p.ID); e == nil {
		t.Fatal("locked reveal")
	}
	if _, e = s.Backup(site.ID, "Synthetic backup passphrase 123"); e == nil {
		t.Fatal("locked credential backup")
	}
	if _, e = s.Export(site.ID); e != nil {
		t.Fatal("offline metadata blocked by locked vault")
	}
}
func TestReconcileIPChangeAndIdentityAmbiguity(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic reconciliation")
	d := syntheticDevice()
	s.SaveObserved(site.ID, []devices.Device{d})
	rows, _ := s.Records(site.ID)
	d.IPv4 = "192.0.2.41"
	changes := Reconcile(rows, []devices.Device{d})
	if len(changes) != 1 || changes[0].Kind != "changed" || changes[0].RecordID != rows[0].ID {
		t.Fatal(changes)
	}
	run, e := s.SaveRun(Run{SiteID: site.ID, Kind: "discovery", Scope: "synthetic /24", Outcome: "completed", Data: encode([]devices.Device{d})})
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.ApplyObservation(site.ID, run.ID, rows[0].ID, d.Key(), []string{"ipv4"})
	if e != nil || r.Device.IPv4 != d.IPv4 {
		t.Fatal(r, e)
	}
	d.MAC = "02:00:00:00:00:02"
	changes = Reconcile(rows, []devices.Device{d})
	if changes[0].Kind != "identity-review" {
		t.Fatal("contradictory identity merged")
	}
	d.Serial = "OTHER-SYNTHETIC"
	changes = Reconcile(rows, []devices.Device{d})
	if changes[0].Kind != "new" {
		t.Fatal("IP used as identity")
	}
	history, _ := s.History(site.ID, r.ID)
	if len(history) < 2 {
		t.Fatal("history missing")
	}
}
func TestFutureSchemaFailsWithoutChangingData(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic newer schema")
	s.db.Exec(`PRAGMA user_version=999`)
	if _, e := Open(s.path); e == nil {
		t.Fatal("future schema accepted")
	}
	if _, e := s.View(site.ID); e != nil {
		t.Fatal("data destroyed")
	}
}
