package sites

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/switches"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

// Deliberately synthetic cameras and passwords; never contact these addresses.
const transferFixture = `{"format":"sadptools-site","version":1,"site":{"name":"Synthetic workshop","customer":"Fixture customer","networks":["192.0.2.0/24"],"defaultCredentials":{"username":"fixture-shared","password":"SYNTHETIC-shared-secret"}},"cameras":[
{"name":"Inherited","ipv4":"192.0.2.10","mac":"02:00:00:00:00:10","httpsPort":8443},
{"name":"Override","ipv4":"192.0.2.11","serial":"synthetic-11","credentials":{"username":"fixture-override","password":"SYNTHETIC-override-secret"}},
{"name":"Unknown","ipv4":"192.0.2.12","inheritCredentials":false},
{"name":"Known empty","ipv4":"192.0.2.13","credentials":{"username":"fixture-empty","password":""}}
]}`

func TestDocumentedTransferExampleAndLegacyBackup(t *testing.T) {
	raw, err := os.ReadFile("../../docs/examples/site-transfer-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := PrepareImport(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Clear()
	if p.Preview().Devices != 3 || p.Preview().DevicesWithCredentials != 2 {
		t.Fatal("documented example drifted")
	}
	legacy, _ := json.Marshal(backupData{Version: 1, Site: Site{ID: "synthetic-legacy", Name: "Legacy fixture"}})
	encrypted, err := vault.EncryptBackup(legacy, "Synthetic legacy passphrase")
	if err != nil {
		t.Fatal(err)
	}
	old, err := PrepareImport(encrypted, "Synthetic legacy passphrase")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Clear()
	if _, err = testStore(t).ImportSite(old, "Legacy import"); err != nil {
		t.Fatal(err)
	}
}

func TestEntireSiteTransferBetweenIndependentVaults(t *testing.T) {
	p, err := PrepareImport([]byte(transferFixture), "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Clear()
	if p.Preview().Devices != 4 || p.Preview().Credentials != 3 || p.Preview().DevicesWithCredentials != 3 {
		t.Fatal("incorrect preview counts")
	}
	for _, value := range []any{p, p.Preview()} {
		raw, _ := json.Marshal(value)
		if bytes.Contains(raw, []byte("SYNTHETIC-shared-secret")) || strings.Contains(fmt.Sprintf("%#v", value), "SYNTHETIC-shared-secret") {
			t.Fatal("preview disclosed credentials")
		}
	}
	source := testStore(t)
	if err = source.Unlock("Synthetic source laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	site, err := source.ImportSite(p, "Original laptop")
	if err != nil {
		t.Fatal(err)
	}
	view, _ := source.View(site.ID)
	for _, r := range view.Devices {
		secret, _, err := source.DeviceSecret(site.ID, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"Inherited": "SYNTHETIC-shared-secret", "Override": "SYNTHETIC-override-secret", "Unknown": "", "Known empty": ""}[r.Device.Name]
		if secret.Password != want {
			t.Fatal("credential assignment was not retained")
		}
		secret.Clear()
		if r.Device.SADPCompatible() || !r.Device.LastSeen.IsZero() {
			t.Fatal("import invented discovery evidence")
		}
	}
	site.Interface = "Source laptop Ethernet"
	if site, err = source.SaveSite(site); err != nil {
		t.Fatal(err)
	}
	asset, err := source.SaveAsset(Asset{SiteID: site.ID, Kind: "configuration", Title: "Synthetic configuration"}, []byte("SYNTHETIC-configuration-secret"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := source.SaveDocument(Document{SiteID: site.ID, Kind: "configuration", Title: "Camera backup", Data: json.RawMessage(`{"assetID":"` + asset.ID + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.SaveSwitch(switches.Asset{SiteID: site.ID, Name: "Synthetic switch", Host: "192.0.2.20", Port: 161, Version: "3", AuthProtocol: "sha256", CredentialID: site.DefaultCredential})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.SaveRun(Run{SiteID: site.ID, Kind: "synthetic-test", Data: `{"deviceID":"` + view.Devices[0].ID + `"}`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.db.Exec(`INSERT INTO reservations VALUES(?,?,?)`, site.ID, "192.0.2.100", "Synthetic spare"); err != nil {
		t.Fatal(err)
	}
	encrypted, err := source.Backup(site.ID, "Synthetic transfer password")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SYNTHETIC-shared-secret", "SYNTHETIC-override-secret", "SYNTHETIC-configuration-secret"} {
		if bytes.Contains(encrypted, []byte(secret)) {
			t.Fatal("plaintext in site file")
		}
	}
	if _, err = PrepareImport(encrypted, "wrong password"); err == nil {
		t.Fatal("wrong file password accepted")
	}
	prepared, err := PrepareImport(encrypted, "Synthetic transfer password")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Clear()
	preview := prepared.Preview()
	if preview.Documents != 1 || preview.Attachments != 1 || preview.Switches != 1 || preview.Runs != 1 || preview.History == 0 {
		t.Fatal("whole site was not included")
	}
	destination := testStore(t)
	existing := addSite(t, destination, "Existing destination site")
	if err = destination.Unlock("Different destination laptop passphrase"); err != nil {
		t.Fatal(err)
	}
	imported, err := destination.ImportSite(prepared, "Transferred workshop")
	if err != nil {
		t.Fatal(err)
	}
	got, err := destination.View(imported.ID)
	if err != nil || got.Site.Interface != "" || len(got.Devices) != 4 || len(got.Reservations) != 1 || imported.ID == site.ID || imported.DefaultCredential == site.DefaultCredential {
		t.Fatal("site contents or identity remapping failed", err)
	}
	all, _ := destination.Sites()
	if len(all) != 2 {
		t.Fatal("existing site overwritten")
	}
	if _, err = destination.View(existing.ID); err != nil {
		t.Fatal("existing site removed")
	}
	for _, r := range got.Devices {
		if r.Device.Name == "Override" {
			secret, _, err := destination.DeviceSecret(imported.ID, r.ID)
			if err != nil || secret.Password != "SYNTHETIC-override-secret" {
				t.Fatal("destination credential unavailable")
			}
			secret.Clear()
		}
	}
	docs, _ := destination.Documents(imported.ID, "")
	if docs[0].ID == doc.ID || bytes.Contains(docs[0].Data, []byte(asset.ID)) {
		t.Fatal("document references not remapped")
	}
	assets, _ := destination.Assets(imported.ID)
	_, payload, err := destination.AssetData(imported.ID, assets[0].ID)
	if err != nil || string(payload) != "SYNTHETIC-configuration-secret" {
		t.Fatal("encrypted attachment was not rekeyed")
	}
	clear(payload)
	sw, _ := destination.Switches(imported.ID)
	if sw[0].CredentialID != imported.DefaultCredential {
		t.Fatal("switch credential lost")
	}
	if err := destination.db.QueryRow(`SELECT count(*) FROM runs WHERE site_id=?`, imported.ID).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	destination.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	plain, _ := os.ReadFile(destination.path)
	if bytes.Contains(plain, []byte("SYNTHETIC-shared-secret")) || bytes.Contains(plain, []byte("SYNTHETIC-configuration-secret")) {
		t.Fatal("plaintext persisted in destination")
	}
	prepared.Clear()
	if _, err = destination.ImportSite(prepared, "Expired"); err == nil {
		t.Fatal("cleared import reused")
	}
}

func TestNXImportRevealedMaskedAndUnknown(t *testing.T) {
	for _, reveal := range []bool{true, false} {
		t.Run(fmt.Sprint(reveal), func(t *testing.T) {
			password := "SYNTHETIC-NX-secret"
			if !reveal {
				password = "[redacted]"
			}
			raw := fmt.Sprintf(`{"summary":{"passwords_revealed":%t},"cameras":[
{"id":"nx-synthetic-id","name":"Fixture","ip":"192.0.2.50","url":"https://fixture:SYNTHETIC-URL-secret@192.0.2.50:8443/path?token=SYNTHETIC-token","username":"fixture","password":%q,"has_password":true,"credential_status":"recovered","media_port":"8000","mac":"02:00:00:00:00:50"},
{"name":"Missing","host":"camera.example.invalid","credential_status":"decode_error","credential_error":"SYNTHETIC-error-secret","password":"SYNTHETIC-untrusted-secret"},
{"name":"Blank","ip":"192.0.2.52","username":"fixture-empty","password":"","has_password":false,"credential_status":"recovered"},
{"name":"Virtual","virtual":true}
]}`, reveal, password)
			p, err := PrepareImport([]byte(raw), "")
			if err != nil {
				t.Fatal(err)
			}
			defer p.Clear()
			count := 1
			if reveal {
				count = 2
			}
			if p.Preview().Devices != 3 || p.Preview().Credentials != count || len(p.Preview().Warnings) < 3 {
				t.Fatal("NX counts/warnings incorrect")
			}
			s := testStore(t)
			s.Unlock("Synthetic NX destination passphrase")
			site, err := s.ImportSite(p, "NX fixture")
			if err != nil {
				t.Fatal(err)
			}
			view, _ := s.View(site.ID)
			for _, r := range view.Devices {
				if r.Device.Name == "Fixture" {
					if r.Device.HTTPSPort == nil || *r.Device.HTTPSPort != 8443 || r.Device.RTSPPort != nil || r.Device.UUID != "" {
						t.Fatal("invented or lost camera metadata")
					}
					secret, _, err := s.DeviceSecret(site.ID, r.ID)
					if err != nil || reveal && secret.Password != password || !reveal && secret.Password != "" {
						t.Fatal("NX password selection failed")
					}
					secret.Clear()
				}
			}
			exported, _ := s.Export(site.ID)
			for _, secret := range []string{"SYNTHETIC-NX-secret", "SYNTHETIC-URL-secret", "SYNTHETIC-token", "SYNTHETIC-error-secret", "SYNTHETIC-untrusted-secret", "[redacted]"} {
				if bytes.Contains(exported, []byte(secret)) {
					t.Fatal("secret leaked into inventory")
				}
			}
		})
	}
}

func TestSiteImportValidationAndRetry(t *testing.T) {
	for _, bad := range []string{
		`{`, `null`, `{}`, `{"format":"unknown"}`, strings.Replace(transferFixture, `"version":1`, `"version":99`, 1),
		strings.Replace(transferFixture, `"httpsPort":8443`, `"httpsPort":70000`, 1),
		strings.Replace(transferFixture, `"ipv4":"192.0.2.10"`, `"ipv4":"not an IP"`, 1),
		strings.Replace(transferFixture, `"password":"SYNTHETIC-shared-secret"`, `"password":null`, 1),
		strings.Replace(transferFixture, `"firmware"`, `"unknownField"`, 1) + ` {}`,
		strings.Replace(transferFixture, `"name":"Inherited"`, `"name":"Inherited","unexpected":true`, 1),
	} {
		if p, err := PrepareImport([]byte(bad), ""); err == nil {
			p.Clear()
			t.Fatal("invalid import accepted")
		}
	}
	p, err := PrepareImport([]byte(transferFixture), "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Clear()
	s := testStore(t)
	s.Unlock("Synthetic retry master passphrase")
	s.Lock()
	if _, err = s.ImportSite(p, "Locked attempt"); err == nil {
		t.Fatal("locked vault import accepted")
	}
	list, _ := s.Sites()
	if len(list) != 0 {
		t.Fatal("failed import partially committed")
	}
	s.Unlock("Synthetic retry master passphrase")
	if _, err = s.ImportSite(p, "Retry"); err != nil {
		t.Fatal("immutable import could not retry", err)
	}
	if _, err = s.ImportSite(p, strings.Repeat("n", 201)); err == nil {
		t.Fatal("oversized site name accepted")
	}
}

func TestExportNeedsVaultWithEncryptedAssetsOnly(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Fixture")
	if needs, err := s.ExportNeedsVault(site.ID); err != nil || needs {
		t.Fatal("empty site needs credentials")
	}
	s.Unlock("Synthetic assets master passphrase")
	if _, err := s.SaveAsset(Asset{SiteID: site.ID, Kind: "configuration"}, []byte("Synthetic config")); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if needs, err := s.ExportNeedsVault(site.ID); err != nil || !needs {
		t.Fatal("encrypted attachment missing from export credential requirement")
	}
}
