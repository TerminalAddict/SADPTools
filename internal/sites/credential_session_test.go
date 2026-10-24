package sites

import (
	"testing"
	"time"
)

func TestActiveCredentialSessionAndManualLock(t *testing.T) {
	s := testStore(t)
	if s.CredentialSessionActive() {
		t.Fatal("session implicitly unlocked vault")
	}
	if e := s.Unlock("synthetic-test-master-passphrase"); e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	s.used = time.Now().Add(-14 * time.Minute)
	s.mu.Unlock()
	if !s.CredentialSessionActive() {
		t.Fatal("active session expired")
	}
	s.mu.Lock()
	refreshed := time.Since(s.used) < time.Second
	s.mu.Unlock()
	if !refreshed {
		t.Fatal("active credential use did not refresh deadline")
	}
	s.Lock()
	if s.CredentialSessionActive() || s.VaultStatus().Unlocked || s.VaultStatus().Automatic {
		t.Fatal("explicit lock not respected")
	}
}
