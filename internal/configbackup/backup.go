// Package configbackup captures documented read-only ONVIF settings. Production
// restoration is deliberately gated until a model/firmware is hardware-validated.
package configbackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"sort"
	"strings"
	"time"
)

type Setting struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	State    string `json:"state"`
	Portable bool   `json:"portable"`
	Reason   string `json:"reason"`
}
type Backup struct {
	Version  int            `json:"version"`
	Device   devices.Device `json:"device"`
	At       string         `json:"at"`
	Provider string         `json:"provider"`
	Settings []Setting      `json:"settings"`
	Checksum string         `json:"checksum"`
}
type Change struct {
	Key      string `json:"key"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Kind     string `json:"kind"`
	Portable bool   `json:"portable"`
}
type Preview struct {
	Compatible   bool      `json:"compatible"`
	Enabled      bool      `json:"enabled"`
	TargetID     string    `json:"targetID"`
	TargetSerial string    `json:"targetSerial"`
	Reasons      []string  `json:"reasons"`
	Settings     []Setting `json:"settings"`
	Excluded     []Setting `json:"excluded"`
}

func Capture(ctx context.Context, c *onvif.Client, d devices.Device, s vault.Secret) (Backup, error) {
	info, e := c.Information(ctx, d, s)
	if e != nil {
		return Backup{}, e
	}
	if d.Serial != "" && info.Device.Serial != d.Serial || d.MAC != "" && info.Device.MAC != "" && devices.NormalizeMAC(d.MAC) != devices.NormalizeMAC(info.Device.MAC) {
		return Backup{}, errors.New("device identity changed; backup cancelled")
	}
	b := Backup{Version: 1, Device: info.Device, At: time.Now().UTC().Format(time.RFC3339Nano), Provider: "ONVIF documented read settings", Settings: []Setting{}}
	for _, op := range []string{"GetNTP", "GetDNS", "GetSystemDateAndTime"} {
		data, e := c.Read(ctx, op, s)
		if e != nil {
			b.Settings = append(b.Settings, Setting{Key: op, State: "unreadable", Reason: sanitize.Text(e.Error())})
			continue
		}
		root, e := xmlsafe.Parse(data)
		clear(data)
		if e != nil || len(root.All(op+"Response")) != 1 {
			b.Settings = append(b.Settings, Setting{Key: op, State: "unreadable", Reason: "malformed or missing response"})
			continue
		}
		paths := map[string][]string{}
		switch op {
		case "GetNTP":
			paths["ntp.servers"] = []string{"NTPManual", "NTPFromDHCP"}
		case "GetDNS":
			paths["dns.servers"] = []string{"DNSManual", "DNSFromDHCP"}
		default:
			paths["clock.timezone"] = []string{"TZ"}
			paths["clock.mode"] = []string{"DateTimeType"}
			paths["clock.daylightSavings"] = []string{"DaylightSavings"}
		}
		for key, names := range paths {
			values := []string{}
			for _, name := range names {
				for _, n := range root.All(name) {
					if n.Text != "" {
						values = append(values, sanitize.Label(n.Text))
					}
					for _, child := range n.Children {
						if child.Name == "IPv4Address" || child.Name == "IPv6Address" || child.Name == "DNSname" {
							values = append(values, sanitize.Label(child.Text))
						}
					}
				}
			}
			state := "available"
			if len(values) == 0 {
				state = "unknown"
			}
			b.Settings = append(b.Settings, Setting{Key: key, Value: strings.Join(values, ", "), State: state, Portable: strings.HasPrefix(key, "clock.") || key == "ntp.servers"})
		}
	}
	catalog, e := c.MediaProfiles(ctx, s, 0)
	if e != nil {
		b.Settings = append(b.Settings, Setting{Key: "video.profiles", State: "unreadable", Reason: sanitize.Text(e.Error())})
	} else {
		for _, p := range catalog.Profiles {
			value, _ := json.Marshal(struct {
				Name, Codec   string
				Width, Height int
			}{sanitize.Label(p.Name), p.Codec, p.Width, p.Height})
			b.Settings = append(b.Settings, Setting{Key: "video.profile." + p.ID, Value: string(value), State: "available", Portable: false, Reason: "Profile tokens and encoder configuration writes are device-specific and unvalidated"})
		}
	}
	b.Settings = append(b.Settings, Setting{Key: "vendor.full-backup", State: "unsupported", Reason: "No documented full-export provider validated for this model/firmware"}, Setting{Key: "network.identity", State: "excluded", Reason: "IP, MAC, serial, accounts, keys and recovery material are never cloned"})
	sort.Slice(b.Settings, func(i, j int) bool { return b.Settings[i].Key < b.Settings[j].Key })
	b.Seal()
	return b, nil
}
func (b *Backup) Seal() {
	b.Checksum = ""
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	b.Checksum = hex.EncodeToString(sum[:])
}
func (b Backup) Validate() error {
	if b.Version != 1 || b.Device.Serial == "" || b.Provider == "" || len(b.Settings) > 2048 {
		return errors.New("unsupported or incomplete configuration package")
	}
	want := b.Checksum
	b.Seal()
	if want == "" || want != b.Checksum {
		return errors.New("configuration checksum failed")
	}
	seen := map[string]bool{}
	for _, s := range b.Settings {
		if s.Key == "" || seen[s.Key] || len(s.Value) > 16384 || sanitize.SensitiveKey(s.Key) {
			return errors.New("configuration contains invalid or sensitive settings")
		}
		seen[s.Key] = true
	}
	return nil
}
func Export(b Backup, passphrase string) ([]byte, error) {
	if e := b.Validate(); e != nil {
		return nil, e
	}
	raw, e := json.Marshal(b)
	if e != nil {
		return nil, e
	}
	defer clear(raw)
	return vault.EncryptBackup(raw, passphrase)
}
func Import(data []byte, passphrase string) (Backup, error) {
	raw, e := vault.DecryptBackup(data, passphrase)
	if e != nil {
		return Backup{}, e
	}
	defer clear(raw)
	var b Backup
	if e = json.Unmarshal(raw, &b); e != nil {
		return b, errors.New("invalid configuration package")
	}
	return b, b.Validate()
}
func Diff(a, b Backup) []Change {
	left, right := map[string]Setting{}, map[string]Setting{}
	keys := map[string]bool{}
	for _, x := range a.Settings {
		left[x.Key] = x
		keys[x.Key] = true
	}
	for _, x := range b.Settings {
		right[x.Key] = x
		keys[x.Key] = true
	}
	out := []Change{}
	for k := range keys {
		x, xok := left[k]
		y, yok := right[k]
		kind := "unchanged"
		switch {
		case !xok:
			kind = "added"
		case !yok:
			kind = "removed"
		case x.State != "available" || y.State != "available":
			kind = "unknown"
		case x.Value != y.Value:
			kind = "changed"
		}
		before, after := sanitize.Text(x.Value), sanitize.Text(y.Value)
		if !x.Portable || !y.Portable || sanitize.SensitiveKey(k) {
			before = "[non-portable value omitted]"
			after = before
		}
		out = append(out, Change{k, before, after, kind, x.Portable && y.Portable})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
func Plan(b Backup, d devices.Device) Preview {
	p := Preview{TargetID: d.ID, TargetSerial: d.Serial, Reasons: []string{"Production restore is disabled until approved model/firmware hardware validation. Preview does not send writes."}, Settings: []Setting{}, Excluded: []Setting{}}
	p.Compatible = d.Serial != "" && !d.IdentityConflict && b.Device.Model != "" && b.Device.Model == d.Model && strings.EqualFold(b.Device.Vendor, d.Vendor) && b.Device.Firmware != "" && b.Device.Firmware == d.Firmware
	if !p.Compatible {
		p.Reasons = append(p.Reasons, "Manufacturer, model, firmware or stable target identity differs/is unknown; restore blocked")
	}
	for _, s := range b.Settings {
		if s.Portable && s.State == "available" {
			p.Settings = append(p.Settings, s)
		} else {
			s.Value = "[excluded]"
			p.Excluded = append(p.Excluded, s)
		}
	}
	return p
}

var ErrGated = errors.New("configuration restoration is disabled pending approved physical-device validation")

func Restore(context.Context, Preview) error { return ErrGated }

// The executor is reachable only by package fixtures until a hardware-qualified
// provider is added. An uncertain write halts the sequence without retrying.
type writer interface {
	Identity(context.Context) (string, error)
	Write(context.Context, Setting) error
	Verify(context.Context, Setting) error
}
type Outcome struct {
	Key   string
	State string
}

func execute(ctx context.Context, p Preview, w writer) ([]Outcome, error) {
	out := []Outcome{}
	if !p.Compatible || p.TargetSerial == "" {
		return out, ErrGated
	}
	id, e := w.Identity(ctx)
	if e != nil || id != p.TargetSerial {
		return out, errors.New("target identity changed")
	}
	for _, s := range p.Settings {
		if !s.Portable {
			return out, errors.New("non-portable setting blocked")
		}
		if e = w.Write(ctx, s); e != nil {
			return append(out, Outcome{s.Key, "unconfirmed"}), e
		}
		if e = w.Verify(ctx, s); e != nil {
			return append(out, Outcome{s.Key, "unconfirmed"}), e
		}
		out = append(out, Outcome{s.Key, "verified"})
	}
	return out, nil
}
