package sites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"time"
)

type wrappedKey struct {
	Mode string
	Salt []byte
	Data []byte
}

func (s *Store) wrapped() (wrappedKey, error) {
	var b []byte
	var w wrappedKey
	e := s.db.QueryRow(`SELECT value FROM metadata WHERE key='vault'`).Scan(&b)
	if e != nil {
		return w, e
	}
	e = json.Unmarshal(b, &w)
	return w, e
}
func (s *Store) expire() {
	if len(s.key) > 0 && time.Since(s.used) > 15*time.Minute {
		clear(s.key)
		s.key = nil
	}
}
func (s *Store) VaultStatus() VaultStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	w, e := s.wrapped()
	mode := w.Mode
	if mode == "" {
		mode = vault.PlatformMode
	}
	return VaultStatus{Initialized: e == nil, Unlocked: len(s.key) == 32, Mode: mode, Automatic: vault.PlatformMode == "dpapi" && mode == "dpapi" && !s.manualLock}
}
func (s *Store) Unlock(passphrase string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unlock(passphrase)
}

// unlock is called with the store mutex held. Windows can unwrap its existing
// key for the current logon without weakening the encrypted on-disk format.
func (s *Store) unlock(passphrase string) error {
	if s.closed {
		return errors.New("credential storage is closed")
	}
	w, e := s.wrapped()
	var key []byte
	if errors.Is(e, sql.ErrNoRows) {
		w.Mode = vault.PlatformMode
		key = vault.Random(32)
		if w.Mode == "dpapi" {
			w.Data, e = vault.Protect(key)
		} else {
			if e = vault.CheckPassphrase(passphrase); e != nil {
				clear(key)
				return e
			}
			w.Salt = vault.Random(16)
			derived := vault.Derive(passphrase, w.Salt)
			w.Data, e = vault.Seal(derived, key, "sadptools-master-key-v1")
			clear(derived)
		}
		if e != nil {
			clear(key)
			return e
		}
		_, e = s.db.Exec(`INSERT INTO metadata VALUES('vault',?)`, []byte(encode(w)))
		if e != nil {
			clear(key)
			return e
		}
	} else if e != nil {
		return errors.New("vault metadata cannot be read; restore a backup")
	} else if w.Mode == "dpapi" {
		key, e = vault.Unprotect(w.Data)
	} else if w.Mode == "passphrase" && len(w.Salt) == 16 && len(passphrase) <= 1024 {
		derived := vault.Derive(passphrase, w.Salt)
		key, e = vault.Open(derived, w.Data, "sadptools-master-key-v1")
		clear(derived)
	} else {
		return errors.New("unsupported vault protection")
	}
	if e != nil || len(key) != 32 {
		clear(key)
		return errors.New("vault could not be unlocked; check the passphrase or Windows user account")
	}
	clear(s.key)
	s.key = key
	s.manualLock = false
	s.touch()
	return nil
}
func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.key)
	s.key = nil
	s.manualLock = true
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
}
func (s *Store) needKey() error {
	if s.closed {
		return errors.New("credential storage is closed")
	}
	s.expire()
	if len(s.key) != 32 && vault.PlatformMode == "dpapi" && !s.manualLock {
		w, e := s.wrapped()
		if errors.Is(e, sql.ErrNoRows) || e == nil && w.Mode == "dpapi" {
			if e = s.unlock(""); e != nil {
				return e
			}
		}
	}
	if len(s.key) != 32 {
		return errors.New("unlock the credential vault first")
	}
	s.touch()
	return nil
}

// CredentialSessionActive keeps an already unlocked key in use during an
// operator-started preview. Status polling does not extend its lifetime, and
// this method never unlocks an explicitly locked vault.
func (s *Store) CredentialSessionActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.manualLock || len(s.key) != 32 {
		return false
	}
	s.touch()
	return true
}
func (s *Store) profiles(site string) ([]Profile, error) {
	rows, e := s.db.Query(`SELECT id,site_id,label,username FROM profiles WHERE site_id=? ORDER BY label`, site)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		var p Profile
		if e = rows.Scan(&p.ID, &p.SiteID, &p.Label, &p.Username); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) SaveProfile(p Profile, secret vault.Secret) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer secret.Clear()
	if e := s.needKey(); e != nil {
		return p, e
	}
	if p.Label == "" || len(p.Label) > 200 || len(secret.Username) > 256 || len(secret.Password) > 4096 || len(secret.PrivacyPassword) > 4096 {
		return p, errors.New("invalid credential label or size")
	}
	if _, e := s.site(p.SiteID); e != nil {
		return p, e
	}
	if p.ID == "" {
		p.ID = ID()
	} else if !s.profileExists(p.SiteID, p.ID) {
		return p, errors.New("credential profile not found in this site")
	}
	p.Username = secret.Username
	plain, e := vault.EncodeSecret(secret)
	if e != nil {
		return p, e
	}
	defer clear(plain)
	cipher, e := vault.Seal(s.key, plain, "profile:"+p.ID)
	if e != nil {
		return p, e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return p, e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO profiles VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET label=excluded.label,username=excluded.username,cipher=excluded.cipher`, p.ID, p.SiteID, p.Label, p.Username, cipher)
	if e != nil {
		return p, e
	}
	if e = event(tx, p.SiteID, "", "manual", "credential-profile-saved", "", encode(p)); e != nil {
		return p, e
	}
	return p, tx.Commit()
}
func (s *Store) DeleteProfile(site, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.needKey(); e != nil {
		return e
	}
	current, e := s.site(site)
	if e != nil {
		return e
	}
	if current.DefaultCredential == id {
		return errors.New("remove this site's default assignment first")
	}
	rows, e := s.records(site)
	if e != nil {
		return e
	}
	for _, r := range rows {
		if r.CredentialMode == "profile" && r.CredentialID == id {
			return errors.New("remove device assignments before deleting this profile")
		}
	}
	assets, e := s.assets(site)
	if e != nil {
		return e
	}
	for _, a := range assets {
		if a.CredentialID == id {
			return errors.New("remove switch assignments before deleting this profile")
		}
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM profiles WHERE site_id=? AND id=?`, site, id); e != nil {
		return e
	}
	if e = event(tx, site, "", "manual", "credential-profile-deleted", id, ""); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) secret(site, id string) (vault.Secret, error) {
	if e := s.needKey(); e != nil {
		return vault.Secret{}, e
	}
	var b []byte
	if e := s.db.QueryRow(`SELECT cipher FROM profiles WHERE site_id=? AND id=?`, site, id).Scan(&b); e != nil {
		return vault.Secret{}, errors.New("credential profile not found")
	}
	plain, e := vault.Open(s.key, b, "profile:"+id)
	if e != nil {
		return vault.Secret{}, e
	}
	defer clear(plain)
	return vault.DecodeSecret(plain)
}

// Secret is only for explicit reveal/copy or backend authenticated operations.
// The return type cannot be JSON-serialized into snapshots or diagnostics.
func (s *Store) Secret(site, id string) (vault.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secret(site, id)
}
func (s *Store) DeviceSecret(site, id string) (vault.Secret, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.record(site, id)
	if e != nil {
		return vault.Secret{}, "", e
	}
	profile := r.CredentialID
	source := "device profile"
	if r.CredentialMode == "none" {
		return vault.Secret{}, "none", nil
	}
	if r.CredentialMode == "inherit" {
		current, e := s.site(site)
		if e != nil {
			return vault.Secret{}, "", e
		}
		profile = current.DefaultCredential
		source = "site default"
	}
	if profile == "" {
		return vault.Secret{}, "none", nil
	}
	secret, e := s.secret(site, profile)
	return secret, source, e
}

// touch is called with the store mutex held. The timer clears the in-memory key
// even when no further UI/API calls arrive after the idle deadline.
func (s *Store) touch() {
	s.used = time.Now()
	if s.idleTimer == nil {
		s.idleTimer = time.AfterFunc(15*time.Minute, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.closed {
				return
			}
			s.expire()
			if len(s.key) > 0 {
				s.idleTimer.Reset(15*time.Minute - time.Since(s.used))
			}
		})
	} else {
		s.idleTimer.Reset(15 * time.Minute)
	}
}
