package sites

import (
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/switches"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

type backupProfile struct {
	Profile Profile
	Secret  []byte
}
type backupData struct {
	Documents    []Document
	Assets       []backupAsset
	OriginalSite string `json:"-"`
	Switches     []switches.Asset
	Version      int
	Site         Site
	Devices      []Record
	Profiles     []backupProfile
	Reservations []Reservation
	Events       []Event
	Runs         []Run
}

func (s *Store) Backup(site, passphrase string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := backupData{Version: 2}
	var e error
	if b.Site, e = s.site(site); e != nil {
		return nil, e
	}
	if b.Devices, e = s.records(site); e != nil {
		return nil, e
	}
	if b.Reservations, e = s.reservations(site); e != nil {
		return nil, e
	}
	if b.Events, e = s.history(site, ""); e != nil {
		return nil, e
	}
	if b.Runs, e = s.runs(site); e != nil {
		return nil, e
	}
	if b.Switches, e = s.assets(site); e != nil {
		return nil, e
	}
	if b.Documents, e = s.documents(site, ""); e != nil {
		return nil, e
	}
	if b.Assets, e = s.backupAssets(site); e != nil {
		return nil, e
	}
	defer func() {
		for _, a := range b.Assets {
			clear(a.Payload)
		}
	}()
	profiles, e := s.profiles(site)
	if e != nil {
		return nil, e
	}
	defer func() {
		for i := range b.Profiles {
			clear(b.Profiles[i].Secret)
		}
	}()
	for _, p := range profiles {
		secret, e := s.secret(site, p.ID)
		if e != nil {
			return nil, e
		}
		plain, e := vault.EncodeSecret(secret)
		secret.Clear()
		if e != nil {
			return nil, e
		}
		b.Profiles = append(b.Profiles, backupProfile{p, plain})
	}
	for i := range b.Runs {
		if e = s.db.QueryRow(`SELECT data FROM runs WHERE id=?`, b.Runs[i].ID).Scan(&b.Runs[i].Data); e != nil {
			return nil, e
		}
	}
	plain, e := json.Marshal(b)
	if e != nil {
		return nil, e
	}
	defer clear(plain)
	return vault.EncryptBackup(plain, passphrase)
}
func (s *Store) Restore(encrypted []byte, passphrase string) (Site, error) {
	plain, e := vault.DecryptBackup(encrypted, passphrase)
	if e != nil {
		return Site{}, e
	}
	defer clear(plain)
	var b backupData
	if json.Unmarshal(plain, &b) != nil || (b.Version != 1 && b.Version != 2) || len(b.Documents) > 10000 || len(b.Assets) > 1000 || len(b.Devices) > 10000 || len(b.Profiles) > 10000 || len(b.Switches) > 10000 || len(b.Events) > 100000 || len(b.Runs) > 10000 {
		return Site{}, errors.New("invalid or oversized backup contents")
	}
	defer func() {
		for i := range b.Profiles {
			clear(b.Profiles[i].Secret)
		}
	}()
	if e = validateSite(b.Site); e != nil {
		return Site{}, e
	}
	for _, r := range b.Devices {
		if e = validateRecord(r); e != nil {
			return Site{}, e
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	needsKey := len(b.Profiles) > 0
	totalAssets := 0
	for _, a := range b.Assets {
		totalAssets += len(a.Payload)
		if totalAssets > 32<<20 {
			return Site{}, errors.New("site attachments exceed 32 MiB")
		}
		needsKey = needsKey || a.Meta.Encrypted
		if len(a.Payload) > 16<<20 {
			return Site{}, errors.New("oversized attachment")
		}
	}
	defer func() {
		for _, a := range b.Assets {
			clear(a.Payload)
		}
	}()
	if needsKey {
		if e = s.needKey(); e != nil {
			return Site{}, e
		}
	}
	ids := map[string]string{}
	addID := func(old string) error {
		if old == "" || ids[old] != "" {
			return errors.New("duplicate/missing backup identity")
		}
		ids[old] = ID()
		return nil
	}
	if e = addID(b.Site.ID); e != nil {
		return Site{}, e
	}
	for _, p := range b.Profiles {
		if e = addID(p.Profile.ID); e != nil {
			return Site{}, e
		}
	}
	for _, r := range b.Devices {
		if e = addID(r.ID); e != nil {
			return Site{}, e
		}
	}
	for _, a := range b.Switches {
		if e = a.Validate(); e != nil {
			return Site{}, e
		}
		if e = addID(a.ID); e != nil {
			return Site{}, e
		}
	}
	for _, d := range b.Documents {
		if e = addID(d.ID); e != nil {
			return Site{}, e
		}
	}
	for _, a := range b.Assets {
		if e = addID(a.Meta.ID); e != nil {
			return Site{}, e
		}
	}
	profileIDs := map[string]bool{}
	for _, p := range b.Profiles {
		profileIDs[p.Profile.ID] = true
	}
	if b.Site.DefaultCredential != "" && !profileIDs[b.Site.DefaultCredential] {
		return Site{}, errors.New("backup default credential reference is missing")
	}
	oldSite := b.Site.ID
	b.OriginalSite = oldSite
	b.Site.ID = ids[oldSite]
	b.Site.Name += " (restored)"
	b.Site.Created = now()
	b.Site.DefaultCredential = ids[b.Site.DefaultCredential]
	tx, e := s.db.Begin()
	if e != nil {
		return Site{}, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO sites VALUES(?,?)`, b.Site.ID, encode(b.Site)); e != nil {
		return Site{}, e
	}
	for _, p := range b.Profiles {
		secret, e := vault.DecodeSecret(p.Secret)
		if e != nil {
			return Site{}, e
		}
		if secret.Username != p.Profile.Username {
			secret.Clear()
			return Site{}, errors.New("backup credential metadata mismatch")
		}
		secret.Clear()
		id := ids[p.Profile.ID]
		cipher, e := vault.Seal(s.key, p.Secret, "profile:"+id)
		if e != nil {
			return Site{}, e
		}
		if _, e = tx.Exec(`INSERT INTO profiles VALUES(?,?,?,?,?)`, id, b.Site.ID, p.Profile.Label, p.Profile.Username, cipher); e != nil {
			return Site{}, e
		}
	}
	for _, r := range b.Devices {
		r.ID = ids[r.ID]
		r.SiteID = b.Site.ID
		if r.CredentialID != "" {
			id := ids[r.CredentialID]
			if id == "" || !profileIDs[r.CredentialID] {
				return Site{}, errors.New("backup contains a missing credential reference")
			}
			r.CredentialID = id
		}
		if _, e = tx.Exec(`INSERT INTO devices VALUES(?,?,?)`, r.ID, r.SiteID, encode(r)); e != nil {
			return Site{}, e
		}
	}
	for _, a := range b.Switches {
		a.ID = ids[a.ID]
		a.SiteID = b.Site.ID
		if a.CredentialID != "" {
			if !profileIDs[a.CredentialID] {
				return Site{}, errors.New("backup switch credential reference is missing")
			}
			a.CredentialID = ids[a.CredentialID]
		}
		a.Updated = now()
		if _, e = tx.Exec(`INSERT INTO switch_assets VALUES(?,?,?)`, a.ID, a.SiteID, encode(a)); e != nil {
			return Site{}, e
		}
	}
	for _, r := range b.Reservations {
		if _, e = tx.Exec(`INSERT INTO reservations VALUES(?,?,?)`, b.Site.ID, r.IP, r.Note); e != nil {
			return Site{}, e
		}
	}
	for _, v := range b.Events {
		if _, e = tx.Exec(`INSERT INTO events VALUES(?,?,?,?,?,?,?,?)`, ID(), b.Site.ID, ids[v.DeviceID], v.Source, v.Kind, v.Before, v.After, v.At); e != nil {
			return Site{}, e
		}
	}
	for _, r := range b.Runs {
		if _, e = tx.Exec(`INSERT INTO runs VALUES(?,?,?,?,?,?,?)`, ID(), b.Site.ID, r.Kind, r.Scope, r.At, r.Outcome, remapRunIDs(r.Data, ids)); e != nil {
			return Site{}, e
		}
	}
	if e = restoreDocuments(tx, &b, ids, s.key); e != nil {
		return Site{}, e
	}
	if e = event(tx, b.Site.ID, "", "restore", "encrypted-backup-restored", oldSite, b.Site.ID); e != nil {
		return Site{}, e
	}
	return b.Site, tx.Commit()
}

func remapRunIDs(raw string, ids map[string]string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return raw
	}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				switch k {
				case "oldDeviceID", "newDeviceID", "id", "siteID", "assetID", "switchID", "recordID", "deviceID", "credentialID", "planID", "linkedDeviceID", "nvrID", "imageID", "snapshotID", "logoID", "documentID", "brandingID":
					if old, ok := v.(string); ok && ids[old] != "" {
						x[k] = ids[old]
					}
				}
				if k == "evidenceIDs" || k == "documentIDs" || k == "imageIDs" {
					if a, ok := v.([]any); ok {
						for i, item := range a {
							if old, ok := item.(string); ok && ids[old] != "" {
								a[i] = ids[old]
							}
						}
					}
				}
				if k == "links" {
					if links, ok := v.(map[string]any); ok {
						for channel, item := range links {
							if old, ok := item.(string); ok && ids[old] != "" {
								links[channel] = ids[old]
							}
						}
					}
				}
				visit(v)
			}
		case []any:
			for _, v := range x {
				visit(v)
			}
		}
	}
	visit(value)
	return encode(value)
}
