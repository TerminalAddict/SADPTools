package sites

import (
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"strings"
	"time"
)

type Replacement struct {
	OldDeviceID  string         `json:"oldDeviceID"`
	NewDeviceID  string         `json:"newDeviceID"`
	OldIdentity  devices.Device `json:"oldIdentity"`
	NewIdentity  devices.Device `json:"newIdentity"`
	State        string         `json:"state"`
	Notes        string         `json:"notes"`
	Checks       []string       `json:"checks"`
	Steps        []string       `json:"steps"`
	Verification string         `json:"verification"`
	VerifiedAt   time.Time      `json:"verifiedAt"`
	VerifiedIP   string         `json:"verifiedIP"`
}

func (s *Store) CreateReplacement(site, oldID, newID, notes string) (Document, error) {
	rows, e := s.Records(site)
	if e != nil {
		return Document{}, e
	}
	var old, new *Record
	for i := range rows {
		if rows[i].ID == oldID && !rows[i].Removed {
			old = &rows[i]
		}
		if rows[i].ID == newID && !rows[i].Removed {
			new = &rows[i]
		}
	}
	if old == nil || new == nil || oldID == newID || (old.Device.MAC == "" && old.Device.Serial == "") || (new.Device.MAC == "" && new.Device.Serial == "") || old.Device.Key() == new.Device.Key() || old.Device.IdentityConflict || new.Device.IdentityConflict {
		return Document{}, errors.New("select two distinct saved devices with verified MAC/serial identities")
	}
	p := Replacement{OldDeviceID: oldID, NewDeviceID: newID, OldIdentity: old.Device, NewIdentity: new.Device, State: "planned", Notes: sanitize.Text(notes), Checks: []string{}, Steps: []string{"Confirm replacement label MAC/serial and physical location", "Check destination IP conflicts before assigning the old address", "If inactive, use the existing single-device Activation dialog with explicit confirmation", "Use existing reviewed Network configuration/provisioning; do not retry an unconfirmed write", "Reconfigure NVR channel in its web interface; automatic reassociation has no verified adapter", "Verify replacement by stable identity, Live View and recording evidence", "Confirm retiring the old inventory item; history and references remain available"}, Verification: "not-tested"}
	if old.Device.Model != new.Device.Model {
		p.Checks = append(p.Checks, "Models differ: configuration cloning is blocked")
	}
	if old.Device.Firmware != new.Device.Firmware {
		p.Checks = append(p.Checks, "Firmware differs: settings transfer is not approved")
	}
	if new.Device.Activated != nil && !*new.Device.Activated {
		p.Checks = append(p.Checks, "Replacement requires activation")
	}
	p.Checks = append(p.Checks, "No device write, credential transfer or NVR change is executed by this planning wizard")
	raw, _ := json.Marshal(p)
	return s.SaveDocument(Document{SiteID: site, DeviceID: newID, Kind: "replacement", Title: "Camera replacement", Version: 1, Data: raw})
}

// CompleteReplacement atomically retires the old record and links both identities.
// It does not rewrite historic NVR associations or transfer credentials.
func (s *Store) CompleteReplacement(site, id, updated, confirmation, notes string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, e := s.document(site, id)
	if e != nil || doc.Kind != "replacement" || doc.Updated != updated {
		return doc, errors.New("replacement plan changed; reload")
	}
	var p Replacement
	if json.Unmarshal(doc.Data, &p) != nil || p.State != "verified" {
		return doc, errors.New("verify replacement connectivity and identity before reconciliation")
	}
	if p.VerifiedAt.IsZero() || time.Since(p.VerifiedAt) < 0 || time.Since(p.VerifiedAt) > 5*time.Minute {
		return doc, errors.New("replacement verification expired; run preflight again")
	}
	if strings.TrimSpace(notes) == "" || len(notes) > 4096 {
		return doc, errors.New("record the stream/NVR/recording checks and any outstanding manual work before retiring the old record")
	}
	old, e := s.record(site, p.OldDeviceID)
	if e != nil {
		return doc, e
	}
	new, e := s.record(site, p.NewDeviceID)
	if e != nil {
		return doc, e
	}
	if new.Removed || old.Removed || new.Device.IdentityConflict || old.Device.IdentityConflict || old.Device.Key() != p.OldIdentity.Key() || new.Device.Key() != p.NewIdentity.Key() || devices.ConflictingIdentity(old.Device, p.OldIdentity) || devices.ConflictingIdentity(new.Device, p.NewIdentity) || new.Device.IPv4 != p.VerifiedIP {
		return doc, errors.New("inventory identity changed; replacement blocked")
	}
	if confirmation != "REPLACE "+new.Device.Key() {
		return doc, errors.New("type REPLACE followed by the exact replacement stable identity")
	}
	tx, e := s.db.Begin()
	if e != nil {
		return doc, e
	}
	defer tx.Rollback()
	before := encode(old)
	old.Removed = true
	old.Updated = now()
	p.State = "completed"
	p.Notes = sanitize.Text(notes)
	doc.Data = json.RawMessage(encode(p))
	doc.Updated = now()
	if _, e = tx.Exec(`UPDATE devices SET data=? WHERE site_id=? AND id=?`, encode(old), site, old.ID); e != nil {
		return doc, e
	}
	if _, e = tx.Exec(`UPDATE site_documents SET data=?,updated=? WHERE site_id=? AND id=?`, encode(doc), doc.Updated, site, id); e != nil {
		return doc, e
	}
	if e = event(tx, site, old.ID, "manual", "device-replaced", before, encode(map[string]string{"newDeviceID": new.ID, "planID": id})); e != nil {
		return doc, e
	}
	if e = event(tx, site, new.ID, "manual", "replacement-completed", "", encode(map[string]string{"oldDeviceID": old.ID, "planID": id})); e != nil {
		return doc, e
	}
	return doc, tx.Commit()
}
