package sites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/mediaimage"
	"github.com/TerminalAddict/SADPTools/internal/reports"
	"github.com/TerminalAddict/SADPTools/internal/retention"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"strings"
)

// Document is site-scoped versioned non-secret data. Sensitive vendor backup
// bytes belong in encrypted Asset payloads, never Data or ordinary exports.
type Document struct {
	ID       string          `json:"id"`
	SiteID   string          `json:"siteID"`
	DeviceID string          `json:"deviceID"`
	Kind     string          `json:"kind"`
	Title    string          `json:"title"`
	Version  int             `json:"version"`
	Data     json.RawMessage `json:"data"`
	Updated  string          `json:"updated"`
}
type Asset struct {
	ID        string `json:"id"`
	SiteID    string `json:"siteID"`
	DeviceID  string `json:"deviceID"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	MIME      string `json:"mime"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Encrypted bool   `json:"encrypted"`
	Created   string `json:"created"`
	Bytes     int    `json:"bytes"`
}

var documentKinds = map[string]bool{"nvr-channels": true, "channel-links": true, "stream-diagnostic": true, "walk-test": true, "configuration": true, "retention": true, "plan": true, "commissioning": true, "branding": true, "ssh-target": true, "remote-observation": true, "report": true}

func validateDocument(d Document) error {
	if !documentKinds[d.Kind] || len(d.Title) > 300 || len(d.Data) > 16<<20 || !json.Valid(d.Data) || d.Version != 1 {
		return errors.New("invalid or unsupported site document")
	}
	return nil
}
func (s *Store) document(site, id string) (Document, error) {
	var d Document
	var raw string
	e := s.db.QueryRow(`SELECT data FROM site_documents WHERE site_id=? AND id=?`, site, id).Scan(&raw)
	if e != nil {
		return d, errors.New("site document not found")
	}
	e = json.Unmarshal([]byte(raw), &d)
	return d, e
}
func (s *Store) Document(site, id string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.document(site, id)
}
func (s *Store) documents(site, kind string) ([]Document, error) {
	rows, e := s.db.Query(`SELECT data FROM site_documents WHERE site_id=? AND (?='' OR kind=?) ORDER BY updated DESC,id`, site, kind, kind)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var raw string
		var d Document
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(raw), &d); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) Documents(site, kind string) ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.site(site); e != nil {
		return nil, e
	}
	return s.documents(site, kind)
}
func (s *Store) SaveDocument(d Document) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Version == 0 {
		d.Version = 1
	}
	if e := validateDocument(d); e != nil {
		return d, e
	}
	if _, e := s.site(d.SiteID); e != nil {
		return d, e
	}
	if d.DeviceID != "" {
		if _, e := s.record(d.SiteID, d.DeviceID); e != nil {
			return d, e
		}
	}
	if d.ID == "" {
		d.ID = ID()
	} else {
		old, e := s.document(d.SiteID, d.ID)
		if e != nil {
			return d, e
		}
		if old.Kind != d.Kind || old.Kind == "report" || old.Updated != d.Updated {
			return d, errors.New("document changed or is an immutable report; reload before editing")
		}
	}
	d.Updated = now()
	tx, e := s.db.Begin()
	if e != nil {
		return d, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO site_documents VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,updated=excluded.updated`, d.ID, d.SiteID, d.Kind, d.Updated, encode(d)); e != nil {
		return d, e
	}
	if e = event(tx, d.SiteID, d.DeviceID, "manual", "document-saved", "", encode(map[string]string{"id": d.ID, "kind": d.Kind})); e != nil {
		return d, e
	}
	return d, tx.Commit()
}
func (s *Store) DeleteDocument(site, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.document(site, id)
	if e != nil {
		return e
	}
	referenced, e := s.documents(site, "")
	if e != nil {
		return e
	}
	for _, other := range referenced {
		if other.ID != id && strings.Contains(string(other.Data), id) {
			return errors.New("document is referenced by saved evidence; remove that reference first")
		}
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM site_documents WHERE site_id=? AND id=?`, site, id); e != nil {
		return e
	}
	if e = event(tx, site, d.DeviceID, "manual", "document-deleted", d.Kind, ""); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) assetsMetadata(site string) ([]Asset, error) {
	rows, e := s.db.Query(`SELECT metadata FROM site_assets WHERE site_id=? ORDER BY id`, site)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		var b string
		var a Asset
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
func (s *Store) Assets(site string) ([]Asset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.site(site); e != nil {
		return nil, e
	}
	return s.assetsMetadata(site)
}
func (s *Store) asset(site, id string) (Asset, []byte, error) {
	var a Asset
	var meta string
	var data []byte
	e := s.db.QueryRow(`SELECT metadata,payload FROM site_assets WHERE site_id=? AND id=?`, site, id).Scan(&meta, &data)
	if e != nil {
		return a, nil, errors.New("site asset not found")
	}
	if e = json.Unmarshal([]byte(meta), &a); e != nil {
		return a, nil, e
	}
	if a.Encrypted {
		if e = s.needKey(); e != nil {
			return a, nil, e
		}
		data, e = vault.Open(s.key, data, "asset:"+a.ID)
	}
	return a, data, e
}

// AssetData is Go-only. Desktop methods expose image assets only.
func (s *Store) AssetData(site, id string) (Asset, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asset(site, id)
}
func (s *Store) SaveAsset(a Asset, data []byte) (Asset, error) {
	if a.ID != "" || len(a.Title) > 300 || len(data) == 0 || len(data) > 16<<20 {
		return a, errors.New("invalid asset or size")
	}
	if a.Kind == "configuration" {
		a.Encrypted = true
		a.MIME = "application/octet-stream"
	} else if a.Kind == "plan" || a.Kind == "snapshot" || a.Kind == "branding" {
		b, w, h, e := mediaimage.Normalise(data)
		if e != nil {
			return a, e
		}
		data = b
		a.Width = w
		a.Height = h
		a.MIME = "image/png"
		a.Encrypted = false
	} else {
		return a, errors.New("unsupported asset type")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.site(a.SiteID); e != nil {
		return a, e
	}
	if a.DeviceID != "" {
		if _, e := s.record(a.SiteID, a.DeviceID); e != nil {
			return a, e
		}
	}
	var total int
	if e := s.db.QueryRow(`SELECT coalesce(sum(length(payload)),0) FROM site_assets WHERE site_id=?`, a.SiteID).Scan(&total); e != nil {
		return a, e
	}
	if total+len(data) > 32<<20 {
		return a, errors.New("site attachments exceed 32 MiB; remove unused assets or create a separate site")
	}
	a.ID = ID()
	a.Created = now()
	a.Bytes = len(data)
	if a.Encrypted {
		if e := s.needKey(); e != nil {
			return a, e
		}
		cipher, e := vault.Seal(s.key, data, "asset:"+a.ID)
		if e != nil {
			return a, e
		}
		data = cipher
	}
	_, e := s.db.Exec(`INSERT INTO site_assets VALUES(?,?,?,?)`, a.ID, a.SiteID, encode(a), data)
	return a, e
}
func (s *Store) DeleteAsset(site, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	docs, e := s.documents(site, "")
	if e != nil {
		return e
	}
	for _, d := range docs {
		if strings.Contains(string(d.Data), id) {
			return errors.New("asset is referenced by a plan or evidence document; remove that reference first")
		}
	}
	_, e = s.db.Exec(`DELETE FROM site_assets WHERE site_id=? AND id=?`, site, id)
	return e
}

type backupAsset struct {
	Meta    Asset
	Payload []byte
}

func (s *Store) backupAssets(site string) ([]backupAsset, error) {
	items, e := s.assetsMetadata(site)
	if e != nil {
		return nil, e
	}
	out := []backupAsset{}
	for _, a := range items {
		_, data, e := s.asset(site, a.ID)
		if e != nil {
			for _, item := range out {
				clear(item.Payload)
			}
			return nil, e
		}
		out = append(out, backupAsset{a, data})
	}
	return out, nil
}
func restoreDocuments(tx *sql.Tx, b *backupData, ids map[string]string, key []byte) error {
	records := map[string]bool{}
	for _, r := range b.Devices {
		records[r.ID] = true
	}
	for _, item := range b.Assets {
		a := item.Meta
		if a.SiteID != b.OriginalSite {
			return errors.New("asset references another site")
		}
		a.ID = ids[a.ID]
		a.SiteID = b.Site.ID
		if a.DeviceID != "" {
			if !records[a.DeviceID] {
				return errors.New("asset device reference missing")
			}
			a.DeviceID = ids[a.DeviceID]
		}
		if len(a.Title) > 300 || len(item.Payload) == 0 || len(item.Payload) > 16<<20 {
			return errors.New("invalid imported asset")
		}
		data := item.Payload
		if a.Kind == "configuration" {
			if !a.Encrypted {
				return errors.New("configuration asset must be encrypted")
			}
			var e error
			data, e = vault.Seal(key, data, "asset:"+a.ID)
			if e != nil {
				return e
			}
		} else {
			if a.Kind != "plan" && a.Kind != "snapshot" && a.Kind != "branding" {
				return errors.New("unsupported asset kind")
			}
			if a.Encrypted {
				return errors.New("unsupported encrypted image")
			}
			normal, w, h, e := mediaimage.Normalise(data)
			if e != nil {
				return e
			}
			data = normal
			a.MIME = "image/png"
			a.Width = w
			a.Height = h
		}
		a.Bytes = len(item.Payload)
		if _, e := tx.Exec(`INSERT INTO site_assets VALUES(?,?,?,?)`, a.ID, a.SiteID, encode(a), data); e != nil {
			return e
		}
	}
	for _, d := range b.Documents {
		if e := validateDocument(d); e != nil {
			return e
		}
		if d.SiteID != b.OriginalSite {
			return errors.New("document references another site")
		}
		d.ID = ids[d.ID]
		d.SiteID = b.Site.ID
		if d.DeviceID != "" {
			if !records[d.DeviceID] {
				return errors.New("document device reference missing")
			}
			d.DeviceID = ids[d.DeviceID]
		}
		if e := validateImportedReferences(d, b, records); e != nil {
			return e
		}
		d.Data = json.RawMessage(remapRunIDs(string(d.Data), ids))
		if _, e := tx.Exec(`INSERT INTO site_documents VALUES(?,?,?,?,?)`, d.ID, d.SiteID, d.Kind, d.Updated, encode(d)); e != nil {
			return e
		}
	}
	return nil
}

// Validate references before remapping so portable attachments cannot bind to
// data in another customer site or to a credential in place of a camera.
func validateImportedReferences(d Document, b *backupData, records map[string]bool) error {
	switch d.Kind {
	case "plan":
		var p installation.Plan
		if json.Unmarshal(d.Data, &p) != nil {
			return errors.New("invalid plan")
		}
		if e := p.Validate(records); e != nil {
			return e
		}
	case "retention":
		var r retention.Scenario
		if json.Unmarshal(d.Data, &r) != nil {
			return errors.New("invalid retention scenario")
		}
		if _, e := retention.Calculate(r); e != nil {
			return e
		}
	case "commissioning":
		var c installation.Commissioning
		if json.Unmarshal(d.Data, &c) != nil {
			return errors.New("invalid checklist")
		}
		if e := c.Validate(records); e != nil {
			return e
		}
	case "report":
		var s reports.Snapshot
		if json.Unmarshal(d.Data, &s) != nil {
			return errors.New("invalid report")
		}
		if e := s.Validate(); e != nil {
			return e
		}
	}
	images, assets, docs, profiles := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, a := range b.Assets {
		assets[a.Meta.ID] = true
		if a.Meta.Kind != "configuration" {
			images[a.Meta.ID] = true
		}
	}
	for _, x := range b.Documents {
		docs[x.ID] = true
	}
	for _, p := range b.Profiles {
		profiles[p.Profile.ID] = true
	}
	var value any
	if json.Unmarshal(d.Data, &value) != nil {
		return errors.New("invalid document data")
	}
	var visit func(any, string) error
	visit = func(v any, k string) error {
		switch x := v.(type) {
		case map[string]any:
			for key, item := range x {
				if e := visit(item, key); e != nil {
					return e
				}
			}
		case []any:
			for _, item := range x {
				if e := visit(item, k); e != nil {
					return e
				}
			}
		case string:
			if x == "" {
				return nil
			}
			valid := true
			switch k {
			case "siteID":
				valid = x == b.OriginalSite
			case "deviceID", "linkedDeviceID", "nvrID":
				valid = records[x]
			case "imageID", "snapshotID", "logoID", "imageIDs":
				valid = images[x]
			case "assetID":
				valid = assets[x]
			case "brandingID", "documentID", "documentIDs":
				valid = docs[x]
			case "evidenceIDs":
				valid = images[x] || docs[x]
			case "credentialID":
				valid = profiles[x]
			}
			if !valid {
				return errors.New("document reference missing or belongs to another site")
			}
		}
		return nil
	}
	if d.Kind == "channel-links" {
		var l struct {
			Links map[string]string `json:"links"`
		}
		if json.Unmarshal(d.Data, &l) != nil {
			return errors.New("invalid channel links")
		}
		for _, id := range l.Links {
			if id != "" && !records[id] {
				return errors.New("channel link device missing")
			}
		}
	}
	return visit(value, "")
}
func (s *Store) deviceHasEvidence(site, id string) bool {
	docs, e := s.documents(site, "")
	if e != nil {
		return true
	}
	for _, d := range docs {
		if d.DeviceID == id || strings.Contains(string(d.Data), id) {
			return true
		}
	}
	assets, e := s.assetsMetadata(site)
	if e != nil {
		return true
	}
	for _, a := range assets {
		if a.DeviceID == id {
			return true
		}
	}
	return false
}
