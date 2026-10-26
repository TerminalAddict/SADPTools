package sites

import (
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type AddedCamera struct {
	Record   Record `json:"record"`
	Existing bool   `json:"existing"`
}

// AddCameraIP is a create-only, site-scoped transaction. Stable duplicate
// identities are returned unchanged. An address collision never implies identity.
func (s *Store) AddCameraIP(r Record, evidence string) (AddedCamera, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID != "" || !webinterface.UsableAddress(r.Device.IPv4) || len(evidence) > 32<<10 {
		return AddedCamera{}, errors.New("invalid camera addition")
	}
	if e := validateRecord(r); e != nil {
		return AddedCamera{}, e
	}
	site, e := s.site(r.SiteID)
	if e != nil {
		return AddedCamera{}, e
	}
	if site.Archived {
		return AddedCamera{}, errors.New("choose an active site")
	}
	rows, e := s.records(r.SiteID)
	if e != nil {
		return AddedCamera{}, e
	}
	id, conflict := Match(rows, r.Device)
	if conflict {
		return AddedCamera{}, errors.New("camera identity conflicts with saved inventory; review the existing records")
	}
	for _, old := range rows {
		if old.Device.IPv4 == r.Device.IPv4 && old.ID != id {
			return AddedCamera{}, errors.New("this IP is already saved with a different or unverified identity; review that record instead of adding a duplicate")
		}
	}
	if id != "" {
		for _, old := range rows {
			if old.ID == id {
				return AddedCamera{Record: old, Existing: true}, nil
			}
		}
	}
	if r.CredentialMode == "profile" && !s.profileExists(r.SiteID, r.CredentialID) {
		return AddedCamera{}, errors.New("choose a credential profile in this site")
	}
	if r.CredentialMode != "profile" {
		r.CredentialID = ""
	}
	r.ID = ID()
	r.FirstSaved = now()
	r.Updated = r.FirstSaved
	tx, e := s.db.Begin()
	if e != nil {
		return AddedCamera{}, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO devices VALUES(?,?,?)`, r.ID, r.SiteID, encode(r)); e != nil {
		return AddedCamera{}, e
	}
	if e = event(tx, r.SiteID, r.ID, "direct-ip", "device-added-by-ip", "", encode(r)); e != nil {
		return AddedCamera{}, e
	}
	if e = event(tx, r.SiteID, r.ID, "direct-ip", "read-only-service-check", "", evidence); e != nil {
		return AddedCamera{}, e
	}
	return AddedCamera{Record: r}, tx.Commit()
}
