package sites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
)

type Preferences struct {
	SiteID  string         `json:"siteID"`
	Scanner ipscan.Options `json:"scanner"`
}

func (s *Store) preferences() (Preferences, error) {
	p := Preferences{Scanner: ipscan.Options{ICMP: true, Workers: 24, Rate: 60, TimeoutMS: 500, TCPPorts: []int{}}}
	var b []byte
	e := s.db.QueryRow(`SELECT value FROM metadata WHERE key='ui-preferences'`).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return p, nil
	}
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(b, &p)
	p.Scanner.ConfirmScope = false
	return p, e
}
func (s *Store) Preferences() (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preferences()
}
func (s *Store) SaveScannerPreferences(o ipscan.Options) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.preferences()
	if e != nil {
		return e
	}
	o.ConfirmScope = false
	p.Scanner = o
	_, e = s.db.Exec(`INSERT INTO metadata VALUES('ui-preferences',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, []byte(encode(p)))
	return e
}
