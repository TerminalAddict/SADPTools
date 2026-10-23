package sites

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/switches"
)

type SaveResult struct {
	Added    int      `json:"added"`
	Existing int      `json:"existing"`
	Review   []string `json:"review"`
}

func (s *Store) SaveObserved(site string, observed []devices.Device) (SaveResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SaveResult{Review: []string{}}
	if len(observed) > 10000 {
		return out, errors.New("save at most 10000 devices at once")
	}
	if _, e := s.site(site); e != nil {
		return out, e
	}
	rows, e := s.records(site)
	if e != nil {
		return out, e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	for _, d := range observed {
		if d.IdentityConflict {
			out.Review = append(out.Review, d.IPv4+": conflicting live identity; review manually")
			continue
		}
		if d.Key() == "" {
			out.Review = append(out.Review, d.IPv4+": no stable identity")
			continue
		}
		match, ambiguous := Match(rows, d)
		if ambiguous {
			out.Review = append(out.Review, d.IPv4+": contradictory or multiple identities; review manually")
			continue
		}
		if match != "" {
			out.Existing++
			continue
		}
		r := Record{ID: ID(), SiteID: site, Device: d, CredentialMode: "inherit", FirstSaved: now(), Updated: now(), Tags: []string{}, ManualFields: []string{}}
		if _, e = tx.Exec(`INSERT INTO devices VALUES(?,?,?)`, r.ID, site, encode(r)); e != nil {
			return out, e
		}
		if e = event(tx, site, r.ID, "discovery", "device-saved", "", encode(r)); e != nil {
			return out, e
		}
		rows = append(rows, r)
		out.Added++
	}
	return out, tx.Commit()
}

type Difference struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
	Manual bool   `json:"manual"`
}
type Proposal struct {
	ID         string         `json:"id"`
	RecordID   string         `json:"recordID"`
	Kind       string         `json:"kind"`
	Confidence string         `json:"confidence"`
	Before     devices.Device `json:"before"`
	After      devices.Device `json:"after"`
	Changes    []Difference   `json:"changes"`
	Message    string         `json:"message"`
}

func fields(d devices.Device) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	json.Unmarshal([]byte(encode(d)), &m)
	return m
}
func Diff(r Record, d devices.Device) []Difference {
	old, next := fields(r.Device), observedFields(r.Device, d)
	out := []Difference{}
	manual := map[string]bool{}
	for _, f := range r.ManualFields {
		manual[f] = true
	}
	for _, k := range []string{"ipv4", "subnetMask", "gateway", "mac", "serial", "model", "name", "firmware", "vendor", "activated", "dhcp", "httpPort", "httpsPort", "commandPort", "rtspPort", "uuid", "protocols", "capabilities", "xaddrs", "interfaces"} {
		b, a := string(old[k]), string(next[k])
		if a == "null" || a == `""` || a == "[]" || a == "" || a == b {
			continue
		}
		out = append(out, Difference{k, b, a, manual[k]})
	}
	return out
}
func Reconcile(rows []Record, observed []devices.Device) []Proposal {
	out := []Proposal{}
	seen := map[string]bool{}
	for _, d := range observed {
		id, ambiguous := Match(rows, d)
		p := Proposal{ID: ID(), RecordID: id, After: d, Changes: []Difference{}}
		if ambiguous {
			p.Kind = "identity-review"
			p.Confidence = "ambiguous"
			p.Message = "Contradictory identity evidence; choose records manually before merging."
			out = append(out, p)
			continue
		}
		if id == "" {
			p.Kind = "new"
			p.Confidence = "unmatched"
			p.Message = "Proposed addition; not saved automatically."
			out = append(out, p)
			continue
		}
		seen[id] = true
		for _, r := range rows {
			if r.ID != id {
				continue
			}
			p.Before = r.Device
			p.Changes = Diff(r, d)
			p.Confidence = "strong"
			if r.Removed {
				p.Kind = "returned"
				p.Message = "Previously marked removed; seen again."
			} else if len(p.Changes) > 0 {
				p.Kind = "changed"
				p.Message = "Review observed changes before updating the saved device."
			} else {
				p.Kind = "unchanged"
			}
			out = append(out, p)
		}
	}
	for _, r := range rows {
		if !r.Removed && !seen[r.ID] {
			out = append(out, Proposal{ID: ID(), RecordID: r.ID, Kind: "not-seen", Confidence: "scope-dependent", Before: r.Device, Message: "Not seen in this scan; this does not prove offline or removed. Review scan scope and last-seen time.", Changes: []Difference{}})
		}
	}
	return out
}

// ApplyObservation accepts only fields that the caller explicitly selected from
// a saved observation. Manual fields remain protected unless explicitly chosen.
func (s *Store) ApplyObservation(site, runID, recordID, observationKey string, selected []string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var data, kind string
	if e := s.db.QueryRow(`SELECT kind,data FROM runs WHERE site_id=? AND id=?`, site, runID).Scan(&kind, &data); e != nil {
		return Record{}, e
	}
	if kind != "discovery" && kind != "onvif-information" {
		return Record{}, errors.New("select a discovery observation")
	}
	var list []devices.Device
	if json.Unmarshal([]byte(data), &list) != nil {
		return Record{}, errors.New("invalid stored observations")
	}
	var d *devices.Device
	for _, x := range list {
		if x.Key() == observationKey {
			if d != nil {
				return Record{}, errors.New("ambiguous observation")
			}
			v := x
			d = &v
		}
	}
	if d == nil {
		return Record{}, errors.New("observation not found")
	}
	rows, e := s.records(site)
	if e != nil {
		return Record{}, e
	}
	match, ambiguous := Match(rows, *d)
	if ambiguous {
		return Record{}, errors.New("resolve identity conflict before accepting changes")
	}
	var r Record
	before := ""
	if recordID == "" {
		if match != "" {
			return r, errors.New("observation already matches a saved record")
		}
		r = Record{ID: ID(), SiteID: site, Device: *d, CredentialMode: "inherit", FirstSaved: now(), ManualFields: []string{}, Tags: []string{}}
	} else {
		if match != recordID {
			return r, errors.New("observation does not strongly match the selected saved device")
		}
		r, e = s.record(site, recordID)
		if e != nil {
			return r, e
		}
		before = encode(r)
		old, next := fields(r.Device), observedFields(r.Device, *d)
		allowed := map[string]bool{}
		for _, v := range Diff(r, *d) {
			allowed[v.Field] = true
		}
		for _, k := range selected {
			if !allowed[k] {
				return r, fmt.Errorf("field %s is not an observed change", k)
			}
			old[k] = next[k]
		}
		b, _ := json.Marshal(old)
		if e = json.Unmarshal(b, &r.Device); e != nil {
			return r, e
		}
		r.Device.ID = r.Device.Key()
		r.Device.LastSeen = d.LastSeen
		r.Removed = false
	}
	r.Updated = now()
	tx, e := s.db.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO devices VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, r.ID, site, encode(r)); e != nil {
		return r, e
	}
	if e = event(tx, site, r.ID, "accepted-discovery", "observation-accepted", before, encode(r)); e != nil {
		return r, e
	}
	return r, tx.Commit()
}
func (s *Store) IgnoreObservation(site, run, identity string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = event(tx, site, "", "manual", "observation-ignored", "", encode(map[string]string{"run": run, "identity": identity})); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) MoveRecord(site, id, destination string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.record(site, id)
	if e != nil {
		return r, e
	}
	if _, e = s.site(destination); e != nil {
		return r, e
	}
	rows, e := s.records(destination)
	if e != nil {
		return r, e
	}
	if match, amb := Match(rows, r.Device); match != "" || amb {
		return r, errors.New("destination has a possible match; review instead of moving")
	}
	old := encode(r)
	r.SiteID = destination
	r.CredentialMode = "none"
	r.CredentialID = ""
	r.Updated = now()
	tx, e := s.db.Begin()
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`UPDATE devices SET site_id=?,data=? WHERE id=?`, destination, encode(r), id); e != nil {
		return r, e
	}
	if e = event(tx, site, id, "manual", "device-moved-out", old, destination); e != nil {
		return r, e
	}
	if e = event(tx, destination, id, "manual", "device-moved-in", old, encode(r)); e != nil {
		return r, e
	}
	return r, tx.Commit()
}
func (s *Store) MergeRecords(site, keepID, removeID, confirm string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if keepID == removeID || confirm != "MERGE" {
		return Record{}, errors.New("explicitly confirm MERGE for two distinct records")
	}
	keep, e := s.record(site, keepID)
	if e != nil {
		return keep, e
	}
	remove, e := s.record(site, removeID)
	if e != nil {
		return keep, e
	}
	before := encode([]Record{keep, remove})
	if keep.Notes == "" {
		keep.Notes = remove.Notes
	}
	if keep.Location == "" {
		keep.Location = remove.Location
	}
	keep.Updated = now()
	tx, e := s.db.Begin()
	if e != nil {
		return keep, e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`UPDATE devices SET data=? WHERE id=?`, encode(keep), keepID); e != nil {
		return keep, e
	}
	if _, e = tx.Exec(`DELETE FROM devices WHERE id=?`, removeID); e != nil {
		return keep, e
	}
	if e = event(tx, site, keepID, "manual", "records-merged", before, encode(keep)); e != nil {
		return keep, e
	}
	return keep, tx.Commit()
}

// Export emits only ordinary metadata. Credential assignments/usernames are not
// necessary for technician exports and are removed along with profile IDs.
func (s *Store) Export(site string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, e := s.site(site)
	if e != nil {
		return nil, e
	}
	rows, e := s.records(site)
	if e != nil {
		return nil, e
	}
	current.DefaultCredential = ""
	for i := range rows {
		rows[i].CredentialID = ""
		rows[i].CredentialMode = ""
	}
	assets, e := s.assets(site)
	if e != nil {
		return nil, e
	}
	for i := range assets {
		assets[i].CredentialID = ""
	}
	reports, e := s.switchReports(site)
	if e != nil {
		return nil, e
	}
	return json.MarshalIndent(struct {
		Reports  []switches.Report `json:"switchObservations"`
		Site     Site              `json:"site"`
		Devices  []Record          `json:"devices"`
		Switches []switches.Asset  `json:"switches"`
	}{reports, current, rows, assets}, "", "  ")
}

// SaveProvisioned is called only for independently verified results of an
// explicitly approved plan with SaveVerified selected. Notes and credentials
// are retained; only the planned network/activation fields are updated.
func (s *Store) SaveProvisioned(site string, verified []devices.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, e := s.site(site); e != nil {
		return e
	}
	rows, e := s.records(site)
	if e != nil {
		return e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, d := range verified {
		id, amb := Match(rows, d)
		if amb || d.IdentityConflict {
			return errors.New("verified result has ambiguous saved identity; review manually")
		}
		r := Record{ID: ID(), SiteID: site, CredentialMode: "inherit", FirstSaved: now(), Tags: []string{}, ManualFields: []string{}}
		before := ""
		if id != "" {
			for _, x := range rows {
				if x.ID == id {
					r = x
					before = encode(r)
					break
				}
			}
			if d.IPv4 != "" {
				r.Device.IPv4 = d.IPv4
			}
			if d.SubnetMask != "" {
				r.Device.SubnetMask = d.SubnetMask
			}
			if d.Gateway != "" {
				r.Device.Gateway = d.Gateway
			}
			if d.CommandPort != nil {
				r.Device.CommandPort = d.CommandPort
			}
			if d.DHCP != nil {
				r.Device.DHCP = d.DHCP
			}
			if d.Activated != nil {
				r.Device.Activated = d.Activated
			}
			r.Device.LastSeen = d.LastSeen
		} else {
			r.Device = d
		}
		r.Updated = now()
		r.Removed = false
		if _, e = tx.Exec(`INSERT INTO devices VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, r.ID, site, encode(r)); e != nil {
			return e
		}
		if e = event(tx, site, r.ID, "verified-provisioning", "confirmed-settings", ""+before, encode(r)); e != nil {
			return e
		}
		if id == "" {
			rows = append(rows, r)
		}
	}
	return tx.Commit()
}

func observedFields(old, d devices.Device) map[string]json.RawMessage {
	next := fields(d)
	previous := fields(old)
	for _, key := range []string{"protocols", "capabilities", "xaddrs", "interfaces"} {
		var a, b []json.RawMessage
		json.Unmarshal(previous[key], &a)
		json.Unmarshal(next[key], &b)
		seen := map[string]bool{}
		merged := []json.RawMessage{}
		for _, v := range append(a, b...) {
			if !seen[string(v)] {
				seen[string(v)] = true
				merged = append(merged, v)
			}
		}
		next[key], _ = json.Marshal(merged)
	}
	return next
}
