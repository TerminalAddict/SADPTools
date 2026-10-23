package sites

import (
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/switches"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSchemaOneMigrationPreservesInventoryAndVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sites.sqlite")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	site := addSite(t, s, "Synthetic migration site")
	if e = s.Unlock("Synthetic-master-123"); e != nil {
		t.Fatal(e)
	}
	profile, e := s.SaveProfile(Profile{SiteID: site.ID, Label: "synthetic"}, vault.Secret{Username: "fixture", Password: "Synthetic-password"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`DROP TABLE switch_assets; PRAGMA user_version=1`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Unlock("Synthetic-master-123"); e != nil {
		t.Fatal(e)
	}
	secret, e := s.Secret(site.ID, profile.ID)
	if e != nil || secret.Password != "Synthetic-password" {
		t.Fatal("vault not preserved", e)
	}
	secret.Clear()
	var version int
	s.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != SchemaVersion {
		t.Fatal(version)
	}
	backups, _ := filepath.Glob(path + ".before-migration-*.sqlite")
	if len(backups) != 1 {
		t.Fatal("missing pre-migration backup")
	}
	stat, _ := os.Stat(backups[0])
	if stat.Size() == 0 {
		t.Fatal("empty backup")
	}
	if _, e = s.Switches(site.ID); e != nil {
		t.Fatal(e)
	}
}
func TestSwitchCredentialsBackupRestoreAndMissingReferenceAtomicity(t *testing.T) {
	s := testStore(t)
	site := addSite(t, s, "Synthetic switch site")
	s.Unlock("Synthetic-master-123")
	p, e := s.SaveProfile(Profile{SiteID: site.ID, Label: "SNMP fixture"}, vault.Secret{Username: "fixture", Password: "Synthetic-auth", PrivacyPassword: "Synthetic-privacy"})
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.SaveSwitch(switches.Asset{SiteID: site.ID, Name: "Synthetic switch", Host: "192.0.2.2", Version: "3", AuthProtocol: "sha256", Port: 161, CredentialID: p.ID})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteProfile(site.ID, p.ID); e == nil {
		t.Fatal("deleted assigned SNMP profile")
	}
	backup, e := s.Backup(site.ID, "Synthetic-backup-123")
	if e != nil {
		t.Fatal(e)
	}
	destination := testStore(t)
	destination.Unlock("Another-master-123")
	restored, e := destination.Restore(backup, "Synthetic-backup-123")
	if e != nil {
		t.Fatal(e)
	}
	assets, e := destination.Switches(restored.ID)
	if e != nil || len(assets) != 1 || assets[0].ID == a.ID || assets[0].CredentialID == p.ID {
		t.Fatal(assets, e)
	}
	secret, e := destination.Secret(restored.ID, assets[0].CredentialID)
	if e != nil || secret.PrivacyPassword != "Synthetic-privacy" {
		t.Fatal("lost portable credential", e)
	}
	secret.Clear()
	plain, _ := vault.DecryptBackup(backup, "Synthetic-backup-123")
	var b backupData
	json.Unmarshal(plain, &b)
	clear(plain)
	b.Site.DefaultCredential = "missing-synthetic-reference"
	plain, _ = json.Marshal(b)
	bad, _ := vault.EncryptBackup(plain, "Synthetic-backup-123")
	clear(plain)
	before, _ := destination.Sites()
	if _, e = destination.Restore(bad, "Synthetic-backup-123"); e == nil {
		t.Fatal("missing profile reference accepted")
	}
	after, _ := destination.Sites()
	if len(before) != len(after) {
		t.Fatal("failed restore changed destination")
	}
}
func TestIdleTimerClearsKeyWithoutFurtherAPICalls(t *testing.T) {
	s := testStore(t)
	s.Unlock("Synthetic-master-123")
	s.mu.Lock()
	s.used = time.Now().Add(-16 * time.Minute)
	s.idleTimer.Reset(time.Millisecond)
	s.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		locked := len(s.key) == 0
		s.mu.Unlock()
		if locked {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("idle key not cleared")
}
