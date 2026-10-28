package sites

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

const nxWitnessKind = "nx-witness"
const maxNXWitness = 4 << 20

// NXWitnessInfo is an explicitly opened, read-only snapshot. Licence keys and
// user/server metadata live in an encrypted asset, never ordinary inventory.
// Numeric inventory values are decimal strings to preserve all int64 bits in JS.
type NXWitnessInfo struct {
	Version    int              `json:"version"`
	ImportedAt string           `json:"importedAt"`
	System     map[string]any   `json:"system"`
	Servers    []map[string]any `json:"servers"`
	Users      []map[string]any `json:"users"`
	Storage    []map[string]any `json:"storage"`
	Licenses   []map[string]any `json:"licenses"`
}

func (NXWitnessInfo) String() string     { return "protected NX Witness information" }
func (n NXWitnessInfo) GoString() string { return n.String() }

type NXWitnessSummary struct {
	Servers  int `json:"servers"`
	Users    int `json:"users"`
	Storage  int `json:"storage"`
	Licenses int `json:"licenses"`
}

func (n NXWitnessInfo) summary() NXWitnessSummary {
	return NXWitnessSummary{len(n.Servers), len(n.Users), len(n.Storage), len(n.Licenses)}
}
func (n NXWitnessInfo) hasData() bool {
	return nxMeaningful(n.System) || len(n.Servers)+len(n.Users)+len(n.Storage)+len(n.Licenses) > 0
}
func nxMeaningful(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case []string:
		return len(x) > 0
	case []map[string]any:
		return len(x) > 0
	case map[string]any:
		for _, item := range x {
			if nxMeaningful(item) {
				return true
			}
		}
		return false
	default:
		return true // Explicit false/zero are known values.
	}
}

// Contract from NX Camera Info's full-report JSON. Unknown fields (including
// password hashes, tokens and arbitrary plugin properties) are never copied.
func nxFields(text, number, boolean, lists string) map[string]byte {
	out := map[string]byte{}
	for _, group := range []struct {
		names string
		kind  byte
	}{{text, 's'}, {number, 'n'}, {boolean, 'b'}, {lists, 'a'}} {
		for _, name := range strings.Fields(group.names) {
			out[name] = group.kind
		}
	}
	return out
}

var nxSystemFields = nxFields("name local_id cloud_id cloud_account cloud_host latest_transaction", "", "", "")
var nxServerFields = nxFields("id name url public_ip version full_version operating_system os_info architecture cpu time_zone", "memory_bytes time_zone_offset_ms max_cameras", "redundancy", "network_addresses lan_discovery_addresses")
var nxUserFields = nxFields("id account type display_name role email", "direct_permissions", "enabled", "group_ids groups")
var nxStorageFields = nxFields("id name path server_id server_name type archive_mode status_flags", "stored_space_bytes space_limit_bytes", "recording_enabled backup", "")
var nxLicenseFields = nxFields("id key serial name class version brand hardware_id expiration company support deactivation_information activation_information license_block", "channels activations_remaining deactivations_used deactivations_remaining", "deactivation_eligible", "")
var nxPropertyFields = nxFields("systemName localSystemId cloudSystemID cloudAccountName cloudHost auditTrailEnabled autoDiscoveryEnabled trafficEncryptionForced backupSettings fullVersion systemRuntime osInfo cpuArchitecture cpuModelName physicalMemory publicIp networkInterfaces timeZoneInformation timezoneUtcOffset hddList productNameShort flavor beta portForwardingConfigurations metadataStorageId space storageArchiveMode persistentStorageStatusFlags storageInfo forceAnalyticsDbStoragePermissions", "", "", "")
var nxLicenseBlockFields = nxFields("SERIAL NAME COUNT CLASS VERSION BRAND HWID EXPIRATION COMPANY SUPPORT DEACTIVATIONS SIGNATURE TYPE", "", "", "")

func cleanNXRecord(data json.RawMessage, fields map[string]byte) (map[string]any, error) {
	out := map[string]any{}
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return out, nil
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return nil, errors.New("NX inventory record must be an object")
	}
	invalid := errors.New("invalid or oversized NX inventory field")
	for name, kind := range fields {
		value, present := raw[name]
		if !present {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			out[name] = nil
			continue
		}
		switch kind {
		case 's':
			var s string
			if json.Unmarshal(value, &s) != nil || len(s) > 64<<10 {
				return nil, invalid
			}
			out[name] = s
		case 'n':
			var n int64
			// Accept our normalized representation when restoring a site backup.
			if json.Unmarshal(value, &n) != nil {
				var s string
				if json.Unmarshal(value, &s) != nil {
					return nil, invalid
				}
				var e error
				if n, e = strconv.ParseInt(s, 10, 64); e != nil {
					return nil, invalid
				}
			}
			out[name] = strconv.FormatInt(n, 10)
		case 'b':
			var b bool
			if json.Unmarshal(value, &b) != nil {
				return nil, invalid
			}
			out[name] = b
		case 'a':
			var list []string
			if json.Unmarshal(value, &list) != nil || len(list) > 10000 {
				return nil, invalid
			}
			for _, s := range list {
				if len(s) > 32768 {
					return nil, invalid
				}
			}
			out[name] = list
		}
	}
	if fieldsName := raw["properties"]; fieldsName != nil && (fields["local_id"] != 0 || fields["public_ip"] != 0 || fields["archive_mode"] != 0) {
		p, e := cleanNXRecord(fieldsName, nxPropertyFields)
		if e != nil {
			return nil, e
		}
		out["properties"] = p
	}
	if fields["license_block"] != 0 && raw["fields"] != nil {
		p, e := cleanNXRecord(raw["fields"], nxLicenseBlockFields)
		if e != nil {
			return nil, e
		}
		out["fields"] = p
	}
	if fields["public_ip"] != 0 && raw["discovery"] != nil {
		list, e := cleanNXList(raw["discovery"], nxFields("url", "", "ignored", ""))
		if e != nil {
			return nil, e
		}
		out["discovery"] = list
	}
	return out, nil
}
func cleanNXList(data json.RawMessage, fields map[string]byte) ([]map[string]any, error) {
	out := []map[string]any{}
	if len(data) == 0 {
		return out, nil
	}
	var raw []json.RawMessage
	if json.Unmarshal(data, &raw) != nil || len(raw) > 10000 {
		return nil, errors.New("invalid or oversized NX inventory section")
	}
	for _, row := range raw {
		v, e := cleanNXRecord(row, fields)
		if e != nil {
			return nil, e
		}
		if nxMeaningful(v) {
			out = append(out, v)
		}
	}
	return out, nil
}

func readNXSections(data []byte) (NXWitnessInfo, error) {
	var raw map[string]json.RawMessage
	var n NXWitnessInfo
	if json.Unmarshal(data, &raw) != nil {
		return n, errors.New("invalid NX inventory JSON")
	}
	var e error
	if n.System, e = cleanNXRecord(raw["system"], nxSystemFields); e != nil {
		return n, e
	}
	for _, part := range []struct {
		name   string
		fields map[string]byte
		dest   *[]map[string]any
	}{
		{"servers", nxServerFields, &n.Servers}, {"users", nxUserFields, &n.Users}, {"storage", nxStorageFields, &n.Storage}, {"licenses", nxLicenseFields, &n.Licenses},
	} {
		if *part.dest, e = cleanNXList(raw[part.name], part.fields); e != nil {
			return n, e
		}
	}
	return n, nil
}

func addNXWitness(b *backupData, data []byte) error {
	n, e := readNXSections(data)
	if e != nil {
		return e
	}
	if !n.hasData() {
		return nil
	}
	n.Version, n.ImportedAt = 1, now()
	payload, e := json.Marshal(n)
	if e != nil || len(payload) > maxNXWitness {
		return errors.New("NX Witness information exceeds 4 MiB")
	}
	b.Assets = append(b.Assets, backupAsset{Meta: Asset{ID: ID(), SiteID: b.Site.ID, Kind: nxWitnessKind, Title: "NX Witness information", MIME: "application/octet-stream", Encrypted: true, Created: n.ImportedAt, Bytes: len(payload)}, Payload: payload})
	return nil
}

func decodeNXWitness(data []byte) (NXWitnessInfo, error) {
	var header struct {
		Version    int    `json:"version"`
		ImportedAt string `json:"importedAt"`
	}
	if len(data) > maxNXWitness || json.Unmarshal(data, &header) != nil || header.Version != 1 {
		return NXWitnessInfo{}, errors.New("invalid NX Witness snapshot")
	}
	if _, e := time.Parse(time.RFC3339Nano, header.ImportedAt); e != nil {
		return NXWitnessInfo{}, errors.New("invalid NX import date")
	}
	n, e := readNXSections(data)
	if e != nil {
		return n, e
	}
	if !n.hasData() {
		return n, errors.New("empty NX Witness snapshot")
	}
	n.Version, n.ImportedAt = header.Version, header.ImportedAt
	return n, nil
}

func (s *Store) nxWitnessAsset(site string) (Asset, error) {
	assets, e := s.assetsMetadata(site)
	if e != nil {
		return Asset{}, e
	}
	var found Asset
	for _, a := range assets {
		if a.Kind == nxWitnessKind {
			if found.ID != "" || !a.Encrypted || a.DeviceID != "" {
				return Asset{}, errors.New("invalid NX Witness attachment")
			}
			found = a
		}
	}
	return found, nil
}

// NXWitness returns sensitive site information only on an explicit user read.
func (s *Store) NXWitness(site string) (NXWitnessInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, e := s.nxWitnessAsset(site)
	if e != nil {
		return NXWitnessInfo{}, e
	}
	if a.ID == "" {
		return NXWitnessInfo{}, errors.New("this site has no imported NX Witness information")
	}
	_, payload, e := s.asset(site, a.ID)
	if e != nil {
		return NXWitnessInfo{}, e
	}
	defer clear(payload)
	return decodeNXWitness(payload)
}
