//go:build windows

package sites

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/vault"
)

// Runs on Windows CI: exercises real current-user DPAPI, including reopening
// the encrypted database. Cross-compilation is not runtime validation.
func TestWindowsAutomaticCredentialsRespectExplicitLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sites.sqlite")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	site := addSite(t, s, "Synthetic automatic Windows credentials")
	if !s.VaultStatus().Automatic {
		t.Fatal("Windows account protection should be automatic")
	}
	p, e := s.SaveProfile(Profile{SiteID: site.ID, Label: "synthetic"}, vault.Secret{Username: "fixture", Password: "synthetic-password"})
	if e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	s.used = time.Now().Add(-16 * time.Minute)
	s.expire()
	s.mu.Unlock()
	if s.VaultStatus().Unlocked {
		t.Fatal("idle key retained")
	}
	secret, e := s.Secret(site.ID, p.ID)
	if e != nil || secret.Password != "synthetic-password" {
		t.Fatal("automatic reuse failed", e)
	}
	secret.Clear()
	s.Lock()
	if s.VaultStatus().Automatic {
		t.Fatal("explicit lock bypassed")
	}
	if _, e = s.Secret(site.ID, p.ID); e == nil {
		t.Fatal("locked credential returned")
	}
	if e = s.Unlock(""); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	secret, e = s.Secret(site.ID, p.ID)
	if e != nil || secret.Password != "synthetic-password" {
		t.Fatal("reopened DPAPI vault", e)
	}
	secret.Clear()
}
