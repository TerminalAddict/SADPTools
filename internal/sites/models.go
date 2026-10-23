// Package sites owns private persistent inventories, observations and encrypted
// credential references. No Wails dependency or automatic network writes.
package sites

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"net/netip"
	"strings"
	"time"
)

type Site struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Customer          string   `json:"customer"`
	Address           string   `json:"address"`
	Notes             string   `json:"notes"`
	Tags              []string `json:"tags"`
	Networks          []string `json:"networks"`
	Interface         string   `json:"interface"`
	Protocols         []string `json:"protocols"`
	DefaultCredential string   `json:"defaultCredential"`
	Archived          bool     `json:"archived"`
	Created           string   `json:"created"`
	LastVisited       string   `json:"lastVisited"`
	LastScan          string   `json:"lastScan"`
}
type Record struct {
	ID             string         `json:"id"`
	SiteID         string         `json:"siteID"`
	Device         devices.Device `json:"device"`
	Location       string         `json:"location"`
	Notes          string         `json:"notes"`
	Tags           []string       `json:"tags"`
	ManualFields   []string       `json:"manualFields"`
	CredentialMode string         `json:"credentialMode"`
	CredentialID   string         `json:"credentialID"`
	FirstSaved     string         `json:"firstSaved"`
	Updated        string         `json:"updated"`
	Removed        bool           `json:"removed"`
}
type Profile struct {
	ID       string `json:"id"`
	SiteID   string `json:"siteID"`
	Label    string `json:"label"`
	Username string `json:"username"`
}
type Reservation struct {
	IP   string `json:"ip"`
	Note string `json:"note"`
}
type Event struct {
	ID       string `json:"id"`
	SiteID   string `json:"siteID"`
	DeviceID string `json:"deviceID"`
	Source   string `json:"source"`
	Kind     string `json:"kind"`
	Before   string `json:"before"`
	After    string `json:"after"`
	At       string `json:"at"`
}
type Run struct {
	ID      string `json:"id"`
	SiteID  string `json:"siteID"`
	Kind    string `json:"kind"`
	Scope   string `json:"scope"`
	At      string `json:"at"`
	Outcome string `json:"outcome"`
	Data    string `json:"data"`
}
type View struct {
	Site         Site          `json:"site"`
	Devices      []Record      `json:"devices"`
	Profiles     []Profile     `json:"profiles"`
	Reservations []Reservation `json:"reservations"`
	Runs         []Run         `json:"runs"`
}
type VaultStatus struct {
	Initialized bool   `json:"initialized"`
	Unlocked    bool   `json:"unlocked"`
	Mode        string `json:"mode"`
}

func ID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic("random source unavailable")
	}
	return hex.EncodeToString(b)
}
func now() string         { return time.Now().UTC().Format(time.RFC3339Nano) }
func encode(v any) string { b, _ := json.Marshal(v); return string(b) }
func validateSite(s Site) error {
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 200 || len(s.Notes) > 32768 || len(s.Customer) > 500 || len(s.Address) > 1000 || len(s.Networks) > 128 || len(s.Tags) > 100 {
		return errors.New("enter a site name and keep metadata within limits")
	}
	for _, n := range s.Networks {
		p, e := netip.ParsePrefix(n)
		if e != nil || !p.Addr().Is4() {
			return errors.New("site networks must be IPv4 CIDRs")
		}
	}
	for _, p := range s.Protocols {
		if p != "sadp" && p != "onvif" {
			return errors.New("unknown discovery protocol")
		}
	}
	return nil
}
func validateRecord(r Record) error {
	if len(encode(r)) > 64<<10 {
		return errors.New("device metadata exceeds 64 KiB")
	}
	if r.CredentialMode != "inherit" && r.CredentialMode != "none" && r.CredentialMode != "profile" {
		return errors.New("choose inherited, none or a credential profile")
	}
	if r.Device.IPv4 != "" {
		a, e := netip.ParseAddr(r.Device.IPv4)
		if e != nil || !a.Is4() {
			return errors.New("invalid device IPv4 address")
		}
	}
	if r.Device.MAC != "" && devices.NormalizeMAC(r.Device.MAC) == "" {
		return errors.New("invalid device MAC")
	}
	return nil
}

// Match never uses an IP address as a permanent identity. Contradictory signals
// and multiple possible matches are review cases, not silent merges.
func Match(list []Record, d devices.Device) (string, bool) {
	match := ""
	ambiguous := d.IdentityConflict
	for _, r := range list {
		x := r.Device
		mac := d.MAC != "" && devices.NormalizeMAC(d.MAC) == devices.NormalizeMAC(x.MAC)
		serial := d.Serial != "" && d.Serial == x.Serial
		uuid := d.UUID != "" && d.UUID == x.UUID
		if !mac && !serial && !uuid {
			continue
		}
		conflict := (d.MAC != "" && x.MAC != "" && !mac) || (d.Serial != "" && x.Serial != "" && !serial) || (d.Vendor != "" && x.Vendor != "" && !strings.EqualFold(d.Vendor, x.Vendor))
		if conflict || match != "" {
			ambiguous = true
		}
		match = r.ID
	}
	return match, ambiguous
}
