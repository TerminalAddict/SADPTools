package sites

import (
	"bytes"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAdvancedAssetsMigrationAndPortableBackup(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic installation")
	if e := s.Unlock("Synthetic master passphrase"); e != nil {
		t.Fatal(e)
	}
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 40, 20)))
	imageAsset, e := s.SaveAsset(Asset{SiteID: site.ID, Kind: "plan", Title: "Synthetic plan"}, pngData.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	config, e := s.SaveAsset(Asset{SiteID: site.ID, Kind: "configuration"}, []byte("synthetic-secret-backup"))
	if e != nil {
		t.Fatal(e)
	}
	d, e := s.SaveDocument(Document{SiteID: site.ID, Kind: "plan", Title: "Level 1", Data: json.RawMessage(`{"imageID":"` + imageAsset.ID + `","markers":[]}`)})
	if e != nil {
		t.Fatal(e)
	}
	another := addSite(t, s, "Other site")
	if _, _, e = s.AssetData(another.ID, imageAsset.ID); e == nil {
		t.Fatal("cross-site asset access")
	}
	if e = s.DeleteAsset(site.ID, imageAsset.ID); e == nil {
		t.Fatal("referenced plan deleted")
	}
	if _, e = s.SaveDocument(Document{ID: d.ID, SiteID: another.ID, Kind: "plan", Data: json.RawMessage(`{}`)}); e == nil {
		t.Fatal("cross-site update")
	}
	raw, e := s.Backup(site.ID, "Synthetic portable passphrase")
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte("synthetic-secret-backup")) {
		t.Fatal("plaintext backup")
	}
	target, e := Open(filepath.Join(t.TempDir(), "restore.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer target.Close()
	target.Unlock("Synthetic destination key")
	restored, e := target.Restore(raw, "Synthetic portable passphrase")
	if e != nil {
		t.Fatal(e)
	}
	docs, e := target.Documents(restored.ID, "plan")
	if e != nil || len(docs) != 1 {
		t.Fatal(e, docs)
	}
	var plan struct {
		ImageID string `json:"imageID"`
	}
	json.Unmarshal(docs[0].Data, &plan)
	if plan.ImageID == imageAsset.ID {
		t.Fatal("asset reference not remapped")
	}
	if _, b, e := target.AssetData(restored.ID, plan.ImageID); e != nil || len(b) == 0 {
		t.Fatal(e)
	}
	assets, _ := target.Assets(restored.ID)
	for _, a := range assets {
		if a.Kind == "configuration" {
			_, data, e := target.AssetData(restored.ID, a.ID)
			if e != nil || string(data) != "synthetic-secret-backup" {
				t.Fatal(e)
			}
		}
	}
	s.Lock()
	if _, _, e = s.AssetData(site.ID, config.ID); e == nil {
		t.Fatal("locked config exposed")
	}
	s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	data, _ := os.ReadFile(s.path)
	if bytes.Contains(data, []byte("synthetic-secret-backup")) {
		t.Fatal("plaintext configuration on disk")
	}
}

func TestEvidenceReferencesRemapAndRestoreIsAtomic(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic portable refs")
	r, e := s.SaveRecord(Record{SiteID: site.ID, CredentialMode: "none", Device: devices.Device{Serial: "synthetic-ref-device", IPv4: "192.0.2.10"}})
	if e != nil {
		t.Fatal(e)
	}
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 10, 10)))
	a, e := s.SaveAsset(Asset{SiteID: site.ID, DeviceID: r.ID, Kind: "snapshot"}, pic.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	evidence, e := s.SaveDocument(Document{SiteID: site.ID, DeviceID: r.ID, Kind: "stream-diagnostic", Data: json.RawMessage(`{"deviceID":"` + r.ID + `"}`)})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(installation.Commissioning{Title: "Synthetic checklist", Checks: []installation.Check{{DeviceID: r.ID, Test: "live-video", Result: "pass", Source: "saved stream-diagnostic", EvidenceIDs: []string{evidence.ID, a.ID}}}})
	_, e = s.SaveDocument(Document{SiteID: site.ID, Kind: "commissioning", Data: raw})
	if e != nil {
		t.Fatal(e)
	}
	links, _ := json.Marshal(map[string]any{"links": map[string]string{"4": r.ID}})
	_, e = s.SaveDocument(Document{SiteID: site.ID, DeviceID: r.ID, Kind: "channel-links", Data: links})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteDocument(site.ID, evidence.ID); e == nil {
		t.Fatal("referenced evidence deleted")
	}
	backup, e := s.Backup(site.ID, "synthetic portable passphrase")
	if e != nil {
		t.Fatal(e)
	}
	restored, e := s.Restore(backup, "synthetic portable passphrase")
	if e != nil {
		t.Fatal(e)
	}
	docs, _ := s.Documents(restored.ID, "")
	rows, _ := s.Records(restored.ID)
	for _, d := range docs {
		if bytes.Contains(d.Data, []byte(r.ID)) || bytes.Contains(d.Data, []byte(evidence.ID)) || bytes.Contains(d.Data, []byte(a.ID)) {
			t.Fatal("old reference survived restore", d.Kind)
		}
		if d.Kind == "channel-links" && !bytes.Contains(d.Data, []byte(rows[0].ID)) {
			t.Fatal("link lost")
		}
	}
	plain, e := vault.DecryptBackup(backup, "synthetic portable passphrase")
	if e != nil {
		t.Fatal(e)
	}
	var b backupData
	json.Unmarshal(plain, &b)
	clear(plain)
	b.Documents[0].DeviceID = "another-site-device"
	tampered, _ := json.Marshal(b)
	bad, _ := vault.EncryptBackup(tampered, "synthetic portable passphrase")
	before, _ := s.Sites()
	if _, e = s.Restore(bad, "synthetic portable passphrase"); e == nil {
		t.Fatal("invalid reference restored")
	}
	after, _ := s.Sites()
	if len(before) != len(after) {
		t.Fatal("failed restore partially committed")
	}
}
