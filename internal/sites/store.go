package sites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const SchemaVersion = 3

type Store struct {
	idleTimer  *time.Timer
	closed     bool
	mu         sync.Mutex
	db         *sql.DB
	path       string
	key        []byte
	used       time.Time
	manualLock bool
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	if _, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA secure_delete=ON;`); err != nil {
		return fail(err)
	}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fail(err)
	}
	if version > SchemaVersion {
		return fail(errors.New("site database is newer than this app; use the newer version"))
	}
	if version < SchemaVersion {
		if version > 0 {
			backup := path + ".before-migration-" + ID() + ".sqlite"
			bf, e := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return fail(e)
			}
			bf.Close()
			if _, e = db.Exec(`VACUUM INTO ?`, backup); e != nil {
				return fail(fmt.Errorf("pre-migration backup failed: %w", e))
			}
		}
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		_, e = tx.Exec(`
CREATE TABLE IF NOT EXISTS sites(id TEXT PRIMARY KEY,data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS devices(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,data TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS devices_site ON devices(site_id);
CREATE TABLE IF NOT EXISTS profiles(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,label TEXT NOT NULL,username TEXT NOT NULL,cipher BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS reservations(site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,ip TEXT NOT NULL,note TEXT NOT NULL,PRIMARY KEY(site_id,ip));
CREATE TABLE IF NOT EXISTS events(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,device_id TEXT NOT NULL,source TEXT NOT NULL,kind TEXT NOT NULL,before_data TEXT NOT NULL,after_data TEXT NOT NULL,at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS events_site ON events(site_id,at);
CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,kind TEXT NOT NULL,scope TEXT NOT NULL,at TEXT NOT NULL,outcome TEXT NOT NULL,data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS switch_assets(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,data TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS switch_assets_site ON switch_assets(site_id);
CREATE TABLE IF NOT EXISTS site_documents(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,kind TEXT NOT NULL,updated TEXT NOT NULL,data TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS site_documents_scope ON site_documents(site_id,kind,updated);
CREATE TABLE IF NOT EXISTS site_assets(id TEXT PRIMARY KEY,site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,metadata TEXT NOT NULL,payload BLOB NOT NULL);
CREATE INDEX IF NOT EXISTS site_assets_scope ON site_assets(site_id);
PRAGMA user_version=3;`)
		if e != nil {
			tx.Rollback()
			return fail(e)
		}
		if e = tx.Commit(); e != nil {
			return fail(e)
		}
	}
	return &Store{db: db, path: path}, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	clear(s.key)
	s.key = nil
	return s.db.Close()
}
func (s *Store) site(id string) (Site, error) {
	var b string
	var site Site
	e := s.db.QueryRow(`SELECT data FROM sites WHERE id=?`, id).Scan(&b)
	if e != nil {
		return site, errors.New("site not found")
	}
	e = json.Unmarshal([]byte(b), &site)
	return site, e
}
func (s *Store) Sites() ([]Site, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.sites() }
func (s *Store) sites() ([]Site, error) {
	rows, e := s.db.Query(`SELECT data FROM sites ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Site{}
	for rows.Next() {
		var b string
		var x Site
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &x); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) SaveSite(site Site) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateSite(site); err != nil {
		return site, err
	}
	before := ""
	kind := "site-created"
	if site.ID == "" {
		site.ID = ID()
		site.Created = now()
	} else {
		old, e := s.site(site.ID)
		if e != nil {
			return site, e
		}
		site.Created = old.Created
		site.LastVisited = old.LastVisited
		site.LastScan = old.LastScan
		before = encode(old)
		kind = "site-edited"
	}
	if site.DefaultCredential != "" && !s.profileExists(site.ID, site.DefaultCredential) {
		return site, errors.New("default credential must belong to this site")
	}
	tx, e := s.db.Begin()
	if e != nil {
		return site, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO sites(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, site.ID, encode(site)); e != nil {
		return site, e
	}
	if e = event(tx, site.ID, "", "manual", kind, before, encode(site)); e != nil {
		return site, e
	}
	return site, tx.Commit()
}
func event(tx *sql.Tx, site, device, source, kind, before, after string) error {
	_, e := tx.Exec(`INSERT INTO events VALUES(?,?,?,?,?,?,?,?)`, ID(), site, device, source, kind, before, after, now())
	return e
}
func (s *Store) DeleteSite(id, confirmation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	site, e := s.site(id)
	if e != nil {
		return e
	}
	if confirmation != site.Name {
		return errors.New("type the exact site name to delete it; make an encrypted backup first")
	}
	_, e = s.db.Exec(`DELETE FROM sites WHERE id=?`, id)
	return e
}
func (s *Store) records(id string) ([]Record, error) {
	rows, e := s.db.Query(`SELECT data FROM devices WHERE site_id=? ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var b string
		var r Record
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &r); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) record(site, id string) (Record, error) {
	var b string
	var r Record
	e := s.db.QueryRow(`SELECT data FROM devices WHERE site_id=? AND id=?`, site, id).Scan(&b)
	if e != nil {
		return r, errors.New("saved device not found")
	}
	e = json.Unmarshal([]byte(b), &r)
	return r, e
}
func (s *Store) Records(site string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records(site)
}
func (s *Store) profileExists(site, id string) bool {
	var n int
	return s.db.QueryRow(`SELECT count(*) FROM profiles WHERE site_id=? AND id=?`, site, id).Scan(&n) == nil && n == 1
}
func (s *Store) SaveRecord(r Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := validateRecord(r); e != nil {
		return r, e
	}
	if _, e := s.site(r.SiteID); e != nil {
		return r, e
	}
	before := ""
	kind := "device-added-manually"
	if r.ID == "" {
		r.ID = ID()
		r.FirstSaved = now()
	} else {
		old, e := s.record(r.SiteID, r.ID)
		if e != nil {
			return r, e
		}
		if r.Updated != old.Updated {
			return r, errors.New("saved device changed; reload before editing")
		}
		r.FirstSaved = old.FirstSaved
		before = encode(old)
		kind = "device-edited"
	}
	if r.CredentialMode == "profile" && !s.profileExists(r.SiteID, r.CredentialID) {
		return r, errors.New("choose a credential profile in this site")
	}
	if r.CredentialMode != "profile" {
		r.CredentialID = ""
	}
	r.Updated = now()
	tx, e := s.db.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO devices VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, r.ID, r.SiteID, encode(r)); e != nil {
		return r, e
	}
	if e = event(tx, r.SiteID, r.ID, "manual", kind, before, encode(r)); e != nil {
		return r, e
	}
	return r, tx.Commit()
}
func (s *Store) View(id string) (View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := View{}
	var e error
	if v.Site, e = s.site(id); e != nil {
		return v, e
	}
	if v.Devices, e = s.records(id); e != nil {
		return v, e
	}
	if v.Profiles, e = s.profiles(id); e != nil {
		return v, e
	}
	if v.Reservations, e = s.reservations(id); e != nil {
		return v, e
	}
	if v.Runs, e = s.runs(id); e != nil {
		return v, e
	}
	return v, nil
}
func (s *Store) Visit(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	site, e := s.site(id)
	if e != nil {
		return e
	}
	p, e := s.preferences()
	if e != nil {
		return e
	}
	p.SiteID = id
	if _, e = s.db.Exec(`INSERT INTO metadata VALUES('ui-preferences',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, []byte(encode(p))); e != nil {
		return e
	}
	site.LastVisited = now()
	_, e = s.db.Exec(`UPDATE sites SET data=? WHERE id=?`, encode(site), id)
	return e
}
func (s *Store) Reserve(site, ip, note string, remove bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, e := netip.ParseAddr(ip)
	if e != nil || !a.Is4() || len(note) > 4096 {
		return errors.New("invalid reservation")
	}
	if _, e = s.site(site); e != nil {
		return e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if remove {
		_, e = tx.Exec(`DELETE FROM reservations WHERE site_id=? AND ip=?`, site, ip)
	} else {
		_, e = tx.Exec(`INSERT INTO reservations VALUES(?,?,?) ON CONFLICT(site_id,ip) DO UPDATE SET note=excluded.note`, site, ip, note)
	}
	if e != nil {
		return e
	}
	if e = event(tx, site, "", "manual", "reservation", "", encode(Reservation{ip, note})); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) reservations(site string) ([]Reservation, error) {
	rows, e := s.db.Query(`SELECT ip,note FROM reservations WHERE site_id=? ORDER BY ip`, site)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	r := []Reservation{}
	for rows.Next() {
		var x Reservation
		if e = rows.Scan(&x.IP, &x.Note); e != nil {
			return nil, e
		}
		r = append(r, x)
	}
	return r, rows.Err()
}
func (s *Store) History(site, device string) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history(site, device)
}
func (s *Store) history(site, device string) ([]Event, error) {
	rows, e := s.db.Query(`SELECT id,site_id,device_id,source,kind,before_data,after_data,at FROM events WHERE site_id=? AND (?='' OR device_id=?) ORDER BY at DESC`, site, device, device)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var r Event
		if e = rows.Scan(&r.ID, &r.SiteID, &r.DeviceID, &r.Source, &r.Kind, &r.Before, &r.After, &r.At); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) SaveRun(r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(r.Data) > 32<<20 || len(r.Scope) > 8192 {
		return r, errors.New("scan/report too large")
	}
	site, e := s.site(r.SiteID)
	if e != nil {
		return r, e
	}
	r.ID = ID()
	r.At = now()
	tx, e := s.db.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO runs VALUES(?,?,?,?,?,?,?)`, r.ID, r.SiteID, r.Kind, r.Scope, r.At, r.Outcome, r.Data)
	if e != nil {
		return r, e
	}
	if r.Kind == "discovery" && r.Outcome == "completed" {
		site.LastScan = r.At
		_, e = tx.Exec(`UPDATE sites SET data=? WHERE id=?`, encode(site), site.ID)
		if e != nil {
			return r, e
		}
	}
	return r, tx.Commit()
}
func (s *Store) runs(site string) ([]Run, error) {
	rows, e := s.db.Query(`SELECT id,site_id,kind,scope,at,outcome,'' FROM runs WHERE site_id=? ORDER BY at DESC`, site)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		if e = rows.Scan(&r.ID, &r.SiteID, &r.Kind, &r.Scope, &r.At, &r.Outcome, &r.Data); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) Run(site, id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Run
	e := s.db.QueryRow(`SELECT id,site_id,kind,scope,at,outcome,data FROM runs WHERE site_id=? AND id=?`, site, id).Scan(&r.ID, &r.SiteID, &r.Kind, &r.Scope, &r.At, &r.Outcome, &r.Data)
	return r, e
}
func (s *Store) Search(query string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sites, e := s.sites()
	if e != nil {
		return nil, e
	}
	out := []Record{}
	for _, site := range sites {
		rows, e := s.records(site.ID)
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			if strings.Contains(strings.ToLower(encode(r)), strings.ToLower(query)) {
				out = append(out, r)
			}
		}
	}
	return out, nil
}
