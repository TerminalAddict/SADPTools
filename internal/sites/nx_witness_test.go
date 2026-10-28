package sites

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/devices"
)

// Synthetic full-report JSON; no customer data or functional credentials.
const nxWitnessFixture = `{"summary":{},"cameras":[{"name":"Synthetic camera","ip":"192.0.2.77"}],
"system":{"name":"Synthetic campus","local_id":"fixture-system","cloud_account":"fixture@example.invalid","properties":{"auditTrailEnabled":"true","cloudToken":"DO-NOT-IMPORT-TOKEN"}},
"servers":[{"id":"fixture-server","name":"Synthetic recorder","url":"http://192.0.2.80:7001","full_version":"6.1.2-synthetic","memory_bytes":8589934592,"redundancy":false,"network_addresses":["192.0.2.80"],"discovery":[{"url":"http://192.0.2.80:7001","ignored":false}],"properties":{"cpuModelName":"Synthetic CPU","password":"DO-NOT-IMPORT-PASSWORD"},"auth_key":"DO-NOT-IMPORT-AUTH"}],
"users":[{"id":"fixture-user","account":"synthetic-viewer","enabled":false,"direct_permissions":9223372036854775807,"groups":["Synthetic operators"],"password_hash":"DO-NOT-IMPORT-HASH"}],
"storage":[{"id":"fixture-storage","name":"Synthetic archive","server_id":"fixture-server","server_name":"Synthetic recorder","path":"/synthetic/archive","stored_space_bytes":0,"space_limit_bytes":null,"recording_enabled":false,"backup":true}],
"licenses":[{"id":"fixture-license","key":"SYNTHETIC-LICENSE-KEY","name":"Synthetic four-channel licence","channels":4,"activations_remaining":null,"deactivations_remaining":0,"deactivation_eligible":false,"activation_information":"Unknown from this export","fields":{"COUNT":"4","PASSWORD":"DO-NOT-IMPORT-FIELD"},"license_block":"SYNTHETIC-LICENSE-BLOCK"}]}`

func TestNXWitnessEncryptedPersistenceAndPortableTransfer(t *testing.T) {
	p, err := PrepareImport([]byte(nxWitnessFixture), "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Clear()
	v := p.Preview()
	if !v.NeedsVault || v.NXWitness == nil || *v.NXWitness != (NXWitnessSummary{1, 1, 1, 1}) {
		t.Fatal("missing NX import preview")
	}
	path := filepath.Join(t.TempDir(), "sites.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = s.ImportSite(p, "Locked"); err == nil {
		t.Fatal("protected inventory imported without vault")
	}
	if err = s.Unlock("Synthetic source laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	site, err := s.ImportSite(p, "Imported NX site")
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.NXWitness(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.Users[0]["direct_permissions"] != "9223372036854775807" || n.Users[0]["enabled"] != false || n.Storage[0]["stored_space_bytes"] != "0" || n.Licenses[0]["activations_remaining"] != nil || n.Licenses[0]["deactivations_remaining"] != "0" {
		t.Fatal("unknown/zero/false/int64 values changed")
	}
	private, _ := json.Marshal(n)
	if bytes.Contains(private, []byte("DO-NOT-IMPORT")) {
		t.Fatal("arbitrary auth fields imported")
	}
	if !bytes.Contains(private, []byte("SYNTHETIC-LICENSE-KEY")) || !bytes.Contains(private, []byte("SYNTHETIC-LICENSE-BLOCK")) {
		t.Fatal("licence details lost")
	}
	view, err := s.View(site.ID)
	if err != nil || !view.HasNXWitness {
		t.Fatal("NX site not identified", err)
	}
	metadata, _ := s.Assets(site.ID)
	report, _ := s.Export(site.ID)
	history, _ := s.History(site.ID, "")
	for _, value := range []any{p, v, view, metadata, history, string(report)} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "SYNTHETIC-LICENSE") || strings.Contains(string(raw), "synthetic-viewer") {
			t.Fatal("private NX information entered metadata/report")
		}
	}
	if strings.Contains(fmt.Sprintf("%#v", n), "SYNTHETIC-LICENSE") {
		t.Fatal("NX data leaked to formatted log")
	}
	var stored []byte
	if err = s.db.QueryRow(`SELECT payload FROM site_assets WHERE site_id=?`, site.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("SYNTHETIC-LICENSE")) || bytes.Contains(stored, []byte("Synthetic recorder")) {
		t.Fatal("plaintext NX data in database")
	}
	backup, err := s.Backup(site.ID, "Synthetic transfer passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backup, []byte("SYNTHETIC-LICENSE")) {
		t.Fatal("plaintext licence in backup")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.View(site.ID)
	if err != nil || !view.HasNXWitness {
		t.Fatal("NX metadata lost after reopening")
	}
	if _, err = s.NXWitness(site.ID); err == nil {
		t.Fatal("locked snapshot read")
	}
	if err = s.Unlock("Synthetic source laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	persisted, err := s.NXWitness(site.ID)
	if err != nil || !reflect.DeepEqual(n, persisted) {
		t.Fatal("NX snapshot changed on disk")
	}
	destination := testStore(t)
	if err = destination.Unlock("Synthetic other laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareImport(backup, "Synthetic transfer passphrase")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Clear()
	if prepared.Preview().NXWitness == nil {
		t.Fatal("portable import lost NX preview")
	}
	restored, err := destination.ImportSite(prepared, "Transferred NX site")
	if err != nil {
		t.Fatal(err)
	}
	got, err := destination.NXWitness(restored.ID)
	if err != nil || !reflect.DeepEqual(n, got) {
		t.Fatal("NX information changed during transfer")
	}
	if _, err = destination.NXWitness(site.ID); err == nil {
		t.Fatal("snapshot references source site")
	}
}

func TestNXWitnessAbsentForDiscoveredAndOlderImportedSites(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic discovered site")
	if _, err := s.SaveObserved(site.ID, []devices.Device{syntheticDevice()}); err != nil {
		t.Fatal(err)
	}
	view, _ := s.View(site.ID)
	if view.HasNXWitness {
		t.Fatal("NX area shown on discovered site")
	}
	if _, err := s.NXWitness(site.ID); err == nil {
		t.Fatal("invented NX information")
	}
	for _, data := range []string{`{"summary":{},"cameras":[]}`, `{"summary":{},"cameras":[],"system":{"name":"","properties":{}},"servers":[],"users":[],"storage":[],"licenses":[]}`} {
		p, err := PrepareImport([]byte(data), "")
		if err != nil {
			t.Fatal(err)
		}
		if p.Preview().NXWitness != nil || p.Preview().NeedsVault {
			t.Fatal("empty inventory created NX snapshot")
		}
		site, err := s.ImportSite(p, "Camera-only import")
		p.Clear()
		if err != nil {
			t.Fatal(err)
		}
		view, _ := s.View(site.ID)
		if view.HasNXWitness {
			t.Fatal("camera-only import shows NX area")
		}
	}
}

func TestNXWitnessRejectsMalformedOrUnprotectedImports(t *testing.T) {
	for _, invalid := range []string{`"servers":{}`, `"users":[{"enabled":"yes"}]`, `"licenses":[{"channels":1.5}]`, `"system":{"properties":[]}`, `"servers":[{"memory_bytes":9223372036854775808}]`} {
		if _, err := PrepareImport([]byte(`{"summary":{},"cameras":[],`+invalid+`}`), ""); err == nil {
			t.Fatal("malformed NX section accepted")
		}
	}
	for _, mutate := range []func(*backupData){
		func(b *backupData) { b.Assets[0].Meta.Encrypted = false },
		func(b *backupData) { b.Assets[0].Meta.DeviceID = b.Devices[0].ID },
		func(b *backupData) { b.Assets[0].Meta.SiteID = "another-site" },
		func(b *backupData) { b.Assets[0].Payload = []byte(`{"version":999}`) },
		func(b *backupData) { a := b.Assets[0]; a.Meta.ID = ID(); b.Assets = append(b.Assets, a) },
	} {
		b, _, err := decodeNX([]byte(nxWitnessFixture))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&b)
		s := testStore(t)
		if err = s.Unlock("Synthetic validation passphrase"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.restoreBackup(b, "Invalid NX"); err == nil {
			t.Fatal("invalid snapshot restored")
		}
		if list, _ := s.Sites(); len(list) != 0 {
			t.Fatal("failed import partially committed")
		}
	}
}
