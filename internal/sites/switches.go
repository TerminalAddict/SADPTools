package sites

import (
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/switches"
)

func (s *Store) assets(site string) ([]switches.Asset, error) {
	rows, e := s.db.Query(`SELECT data FROM switch_assets WHERE site_id=? ORDER BY id`, site)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []switches.Asset{}
	for rows.Next() {
		var b string
		var a switches.Asset
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &a); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) Switches(site string) ([]switches.Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.assets(site)
}
func (s *Store) SaveSwitch(a switches.Asset) (switches.Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := a.Validate(); e != nil {
		return a, e
	}
	if _, e := s.site(a.SiteID); e != nil {
		return a, e
	}
	if a.CredentialID != "" {
		var n int
		if e := s.db.QueryRow(`SELECT count(*) FROM profiles WHERE id=? AND site_id=?`, a.CredentialID, a.SiteID).Scan(&n); e != nil || n != 1 {
			return a, errors.New("select a credential profile belonging to this site")
		}
	}
	before := ""
	if a.ID == "" {
		a.ID = ID()
	} else {
		if e := s.db.QueryRow(`SELECT data FROM switch_assets WHERE id=? AND site_id=?`, a.ID, a.SiteID).Scan(&before); e != nil {
			return a, errors.New("switch asset not found")
		}
		var old switches.Asset
		json.Unmarshal([]byte(before), &old)
		if old.Updated != a.Updated {
			return a, errors.New("switch changed; reopen it before saving")
		}
	}
	a.Updated = now()
	tx, e := s.db.Begin()
	if e != nil {
		return a, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO switch_assets VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, a.ID, a.SiteID, encode(a)); e != nil {
		return a, e
	}
	if e = event(tx, a.SiteID, "", "manual", "switch-saved", before, encode(a)); e != nil {
		return a, e
	}
	return a, tx.Commit()
}
func (s *Store) DeleteSwitch(site, id, confirmation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b string
	if e := s.db.QueryRow(`SELECT data FROM switch_assets WHERE id=? AND site_id=?`, id, site).Scan(&b); e != nil {
		return errors.New("switch asset not found")
	}
	var a switches.Asset
	if json.Unmarshal([]byte(b), &a) != nil {
		return errors.New("invalid switch asset")
	}
	if confirmation != a.Name {
		return errors.New("type the switch name to confirm deletion")
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM switch_assets WHERE id=? AND site_id=?`, id, site); e != nil {
		return e
	}
	if e = event(tx, site, "", "manual", "switch-deleted", b, ""); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) SwitchReports(site string) ([]switches.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.switchReports(site)
}
func (s *Store) switchReports(site string) ([]switches.Report, error) {
	rows, e := s.db.Query(`SELECT data FROM runs WHERE site_id=? AND kind='switch' ORDER BY at DESC`, site)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []switches.Report{}
	seen := map[string]bool{}
	for rows.Next() {
		var b string
		var r switches.Report
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if json.Unmarshal([]byte(b), &r) != nil {
			continue
		}
		if !seen[r.AssetID] {
			seen[r.AssetID] = true
			out = append(out, r)
		}
	}
	return out, rows.Err()
}
