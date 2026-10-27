package sites

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

const TransferFormat = "sadptools-site"

// ImportPreview deliberately excludes credentials and raw file contents.
type ImportPreview struct {
	Token                  string   `json:"token"`
	Format                 string   `json:"format"`
	Name                   string   `json:"name"`
	Customer               string   `json:"customer"`
	Devices                int      `json:"devices"`
	Credentials            int      `json:"credentials"`
	DevicesWithCredentials int      `json:"devicesWithCredentials"`
	Switches               int      `json:"switches"`
	Documents              int      `json:"documents"`
	Attachments            int      `json:"attachments"`
	History                int      `json:"history"`
	Runs                   int      `json:"runs"`
	NeedsVault             bool     `json:"needsVault"`
	Warnings               []string `json:"warnings"`
}

// PreparedImport is an immutable, backend-only snapshot of the chosen file.
// Its owner serializes access and clears it on cancellation, expiry or shutdown.
type PreparedImport struct {
	payload []byte
	preview ImportPreview
}

func (*PreparedImport) String() string               { return "site import (credentials redacted)" }
func (p *PreparedImport) GoString() string           { return p.String() }
func (*PreparedImport) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }
func (p *PreparedImport) Clear()                     { clear(p.payload); p.payload = nil }
func (p *PreparedImport) Preview() ImportPreview {
	v := p.preview
	v.Warnings = append([]string{}, v.Warnings...)
	return v
}

func clearBackup(b *backupData) {
	for i := range b.Profiles {
		clear(b.Profiles[i].Secret)
	}
	for i := range b.Assets {
		clear(b.Assets[i].Payload)
	}
}

func (s *Store) ExportNeedsVault(site string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.site(site); err != nil {
		return false, err
	}
	profiles, err := s.profiles(site)
	if err != nil {
		return false, err
	}
	assets, err := s.assetsMetadata(site)
	if err != nil {
		return false, err
	}
	for _, a := range assets {
		if a.Encrypted {
			return true, nil
		}
	}
	return len(profiles) > 0, nil
}

// PrepareImport reads no network services and changes no stored sites.
func PrepareImport(data []byte, passphrase string) (*PreparedImport, error) {
	if len(data) > 2*vault.MaxBackup {
		return nil, errors.New("site file exceeds 128 MiB")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var header struct {
		Format string `json:"format"`
	}
	if json.Unmarshal(data, &header) != nil {
		return nil, errors.New("invalid site JSON; choose a .sadpsite or JSON export")
	}
	var b backupData
	defer clearBackup(&b)
	format := "SADPTools encrypted site"
	warnings := []string{}
	switch header.Format {
	case vault.Format:
		plain, err := vault.DecryptBackup(data, passphrase)
		if err != nil {
			return nil, err
		}
		defer clear(plain)
		if json.Unmarshal(plain, &b) != nil {
			return nil, errors.New("invalid encrypted site contents")
		}
	case TransferFormat:
		if len(data) > vault.MaxBackup {
			return nil, errors.New("site JSON exceeds 64 MiB")
		}
		var err error
		b, err = decodeTransfer(data)
		if err != nil {
			return nil, err
		}
		format = "SADPTools site JSON v1"
	case "":
		if len(data) > vault.MaxBackup {
			return nil, errors.New("site JSON exceeds 64 MiB")
		}
		var err error
		b, warnings, err = decodeNX(data)
		if err != nil {
			return nil, err
		}
		format = "NX Camera Info JSON"
	default:
		return nil, errors.New("unsupported site format; choose a SADPTools site or NX Camera Info JSON export")
	}
	if err := validateTransferBackup(b); err != nil {
		return nil, err
	}
	// Adapter names/indices belong to the source laptop. Keep saved scope, but
	// require the destination user to choose a local adapter before scanning.
	if b.Site.Interface != "" {
		warnings = append(warnings, "Choose a network adapter on this computer before rescanning the imported site.")
	}
	b.Site.Interface = ""
	for i := range b.Devices {
		b.Devices[i].Device.Interfaces = nil
	}
	p := ImportPreview{Format: format, Name: b.Site.Name, Customer: b.Site.Customer, Devices: len(b.Devices),
		Credentials: len(b.Profiles), Switches: len(b.Switches), Documents: len(b.Documents), Attachments: len(b.Assets),
		History: len(b.Events), Runs: len(b.Runs), NeedsVault: len(b.Profiles) > 0, Warnings: warnings}
	for _, r := range b.Devices {
		if r.CredentialMode == "profile" || r.CredentialMode == "inherit" && b.Site.DefaultCredential != "" {
			p.DevicesWithCredentials++
		}
	}
	for _, a := range b.Assets {
		p.NeedsVault = p.NeedsVault || a.Meta.Encrypted
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return nil, errors.New("could not prepare site import")
	}
	if len(payload) > vault.MaxBackup {
		clear(payload)
		return nil, errors.New("site contents exceed 64 MiB")
	}
	return &PreparedImport{payload: payload, preview: p}, nil
}

func (s *Store) ImportSite(p *PreparedImport, name string) (Site, error) {
	if p == nil || len(p.payload) == 0 {
		return Site{}, errors.New("site import has expired; choose the file again")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return Site{}, errors.New("enter a site name of at most 200 bytes")
	}
	var b backupData
	if json.Unmarshal(p.payload, &b) != nil {
		return Site{}, errors.New("invalid prepared site import")
	}
	// restoreBackup owns and clears this copy. Failed imports are safe to retry.
	return s.restoreBackup(b, name)
}

func validateTransferBackup(b backupData) error {
	if (b.Version != 1 && b.Version != 2) || len(b.Devices) > 10000 || len(b.Profiles) > 10000 || len(b.Switches) > 10000 || len(b.Documents) > 10000 || len(b.Assets) > 1000 || len(b.Reservations) > 10000 || len(b.Events) > 100000 || len(b.Runs) > 10000 {
		return errors.New("invalid version or oversized site contents")
	}
	if err := validateSite(b.Site); err != nil {
		return err
	}
	ids := map[string]bool{}
	add := func(id string) error {
		if id == "" || len(id) > 256 || ids[id] {
			return errors.New("missing or duplicate site identity")
		}
		ids[id] = true
		return nil
	}
	if err := add(b.Site.ID); err != nil {
		return err
	}
	profiles := map[string]bool{}
	for _, p := range b.Profiles {
		if err := add(p.Profile.ID); err != nil {
			return err
		}
		secret, err := vault.DecodeSecret(p.Secret)
		valid := err == nil && len(p.Profile.Label) > 0 && len(p.Profile.Label) <= 200 && secret.Username == p.Profile.Username && len(secret.Username) <= 256 && len(secret.Password) <= 4096 && len(secret.PrivacyPassword) <= 4096
		secret.Clear()
		if !valid {
			return errors.New("invalid site credential")
		}
		profiles[p.Profile.ID] = true
	}
	if b.Site.DefaultCredential != "" && !profiles[b.Site.DefaultCredential] {
		return errors.New("missing site default credential")
	}
	for _, r := range b.Devices {
		if err := add(r.ID); err != nil {
			return err
		}
		if err := validateRecord(r); err != nil {
			return err
		}
		if r.CredentialID != "" && !profiles[r.CredentialID] || r.CredentialMode == "profile" && r.CredentialID == "" {
			return errors.New("missing camera credential reference")
		}
	}
	for _, a := range b.Switches {
		if err := add(a.ID); err != nil {
			return err
		}
		if err := a.Validate(); err != nil {
			return err
		}
		if a.CredentialID != "" && !profiles[a.CredentialID] {
			return errors.New("missing switch credential reference")
		}
	}
	for _, d := range b.Documents {
		if err := add(d.ID); err != nil {
			return err
		}
		if err := validateDocument(d); err != nil {
			return err
		}
	}
	total := 0
	for _, a := range b.Assets {
		if err := add(a.Meta.ID); err != nil {
			return err
		}
		total += len(a.Payload)
		if len(a.Payload) > 16<<20 || total > 32<<20 {
			return errors.New("site attachment limits exceeded")
		}
	}
	for _, r := range b.Reservations {
		ip, err := netip.ParseAddr(r.IP)
		if err != nil || !ip.Is4() || len(r.Note) > 32768 {
			return errors.New("invalid site address reservation")
		}
	}
	return nil
}

// Public interoperability contract; intentionally independent of SQLite IDs.
type transferCredential struct {
	Username string  `json:"username"`
	Password *string `json:"password"`
}
type transferSite struct {
	Name               string              `json:"name"`
	Customer           string              `json:"customer"`
	Address            string              `json:"address"`
	Notes              string              `json:"notes"`
	Tags               []string            `json:"tags"`
	Networks           []string            `json:"networks"`
	DefaultCredentials *transferCredential `json:"defaultCredentials"`
}
type transferCamera struct {
	Name               string              `json:"name"`
	IPv4               string              `json:"ipv4"`
	MAC                string              `json:"mac"`
	Serial             string              `json:"serial"`
	Vendor             string              `json:"vendor"`
	Model              string              `json:"model"`
	Firmware           string              `json:"firmware"`
	SubnetMask         string              `json:"subnetMask"`
	Gateway            string              `json:"gateway"`
	HTTPPort           *int                `json:"httpPort"`
	HTTPSPort          *int                `json:"httpsPort"`
	RTSPPort           *int                `json:"rtspPort"`
	Activated          *bool               `json:"activated"`
	DHCP               *bool               `json:"dhcp"`
	Location           string              `json:"location"`
	Notes              string              `json:"notes"`
	Tags               []string            `json:"tags"`
	Credentials        *transferCredential `json:"credentials"`
	InheritCredentials *bool               `json:"inheritCredentials"`
}
type transferFile struct {
	Format  string           `json:"format"`
	Version int              `json:"version"`
	Site    transferSite     `json:"site"`
	Cameras []transferCamera `json:"cameras"`
}

func decodeTransfer(data []byte) (b backupData, err error) {
	var f transferFile
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&f) != nil || d.Decode(new(any)) != io.EOF || f.Version != 1 || f.Cameras == nil || len(f.Cameras) > 10000 {
		return b, errors.New("invalid SADPTools site JSON v1; check the documented fields and camera limit")
	}
	b.Version = 2
	b.Site = Site{ID: ID(), Name: f.Site.Name, Customer: f.Site.Customer, Address: f.Site.Address, Notes: f.Site.Notes, Tags: f.Site.Tags, Networks: f.Site.Networks, Created: now()}
	if f.Site.DefaultCredentials != nil {
		b.Site.DefaultCredential, err = addTransferCredential(&b, "Site default", f.Site.DefaultCredentials)
		if err != nil {
			clearBackup(&b)
			return b, err
		}
	}
	for i, c := range f.Cameras {
		if err = addTransferCamera(&b, c); err != nil {
			clearBackup(&b)
			return b, fmt.Errorf("camera %d: %w", i+1, err)
		}
	}
	return b, nil
}

func addTransferCredential(b *backupData, label string, c *transferCredential) (string, error) {
	if c.Password == nil || len(c.Username) > 256 || len(*c.Password) > 4096 {
		return "", errors.New("credentials require a password field and bounded username/password")
	}
	id := ID()
	secret := vault.Secret{Username: c.Username, Password: *c.Password}
	plain, err := vault.EncodeSecret(secret)
	secret.Clear()
	if err != nil {
		return "", errors.New("invalid credential")
	}
	b.Profiles = append(b.Profiles, backupProfile{Profile: Profile{ID: id, SiteID: b.Site.ID, Label: label, Username: c.Username}, Secret: plain})
	return id, nil
}

func addTransferCamera(b *backupData, c transferCamera) error {
	for _, p := range []*int{c.HTTPPort, c.HTTPSPort, c.RTSPPort} {
		if p != nil && (*p < 1 || *p > 65535) {
			return errors.New("port must be 1–65535 or omitted")
		}
	}
	if c.Credentials != nil && c.InheritCredentials != nil && *c.InheritCredentials {
		return errors.New("choose camera credentials or inherited credentials")
	}
	r := Record{ID: ID(), SiteID: b.Site.ID, Location: c.Location, Notes: c.Notes, Tags: c.Tags, CredentialMode: "none", FirstSaved: now(), Updated: now(),
		Device: devices.Device{Name: c.Name, IPv4: c.IPv4, MAC: c.MAC, Serial: c.Serial, Vendor: c.Vendor, Model: c.Model, Firmware: c.Firmware,
			SubnetMask: c.SubnetMask, Gateway: c.Gateway, HTTPPort: c.HTTPPort, HTTPSPort: c.HTTPSPort, RTSPPort: c.RTSPPort, Activated: c.Activated, DHCP: c.DHCP, Protocols: []string{"imported"}}}
	if err := validateRecord(r); err != nil {
		return err
	}
	r.Device.MAC = devices.NormalizeMAC(r.Device.MAC)
	r.Device.ID = r.Device.Key()
	if r.Device.ID == "" {
		r.Device.ID = "manual:" + r.ID
	}
	if c.Credentials != nil {
		id, err := addTransferCredential(b, fmt.Sprintf("Imported camera %d", len(b.Devices)+1), c.Credentials)
		if err != nil {
			return err
		}
		r.CredentialMode = "profile"
		r.CredentialID = id
	} else if (c.InheritCredentials == nil || *c.InheritCredentials) && b.Site.DefaultCredential != "" {
		r.CredentialMode = "inherit"
	}
	b.Devices = append(b.Devices, r)
	return nil
}
