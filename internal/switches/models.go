// Package switches polls explicitly registered switches using read-only SNMP.
package switches

import (
	"encoding/json"
	"errors"
	"net/netip"
	"regexp"
	"strings"
)

type PortLink struct {
	IfIndex   int `json:"ifIndex"`
	PoEGroup  int `json:"poeGroup"`
	PoEPort   int `json:"poePort"`
	LLDPLocal int `json:"lldpLocal"`
}
type Asset struct {
	ID           string     `json:"id"`
	SiteID       string     `json:"siteID"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	Vendor       string     `json:"vendor"`
	Model        string     `json:"model"`
	Location     string     `json:"location"`
	Notes        string     `json:"notes"`
	Version      string     `json:"version"`
	AllowV2      bool       `json:"allowV2"`
	CredentialID string     `json:"credentialID"`
	AuthProtocol string     `json:"authProtocol"`
	Port         int        `json:"port"`
	ContextName  string     `json:"contextName"`
	Uplinks      []int      `json:"uplinks"`
	PortLinks    []PortLink `json:"portLinks"`
	Updated      string     `json:"updated"`
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func (a Asset) Validate() error {
	b, _ := json.Marshal(a)
	if len(b) > 64<<10 || strings.TrimSpace(a.Name) == "" || len(a.Name) > 200 || a.Host == "" || !hostname.MatchString(a.Host) || strings.Contains(a.Host, "..") || a.Port < 1 || a.Port > 65535 || len(a.ContextName) > 128 {
		return errors.New("enter a switch name, IPv4/hostname and UDP port 1–65535")
	}
	if ip, e := netip.ParseAddr(a.Host); e == nil && (!ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() || ip.String() == "255.255.255.255") {
		return errors.New("switch must have a usable IPv4 address")
	}
	if a.Version != "3" && (a.Version != "2c" || !a.AllowV2) {
		return errors.New("use SNMPv3 or explicitly enable SNMPv2c")
	}
	if a.Version == "3" && a.AuthProtocol != "sha256" && a.AuthProtocol != "sha512" && a.AuthProtocol != "sha1" {
		return errors.New("SNMPv3 supports SHA-256/SHA-512 or explicit legacy SHA-1, with AES-128 privacy")
	}
	if len(a.Uplinks) > 4096 || len(a.PortLinks) > 4096 {
		return errors.New("too many port annotations")
	}
	seen := map[int]bool{}
	for _, p := range a.PortLinks {
		if p.IfIndex <= 0 || seen[p.IfIndex] || p.PoEGroup < 0 || p.PoEPort < 0 || p.LLDPLocal < 0 {
			return errors.New("port mappings need unique positive ifIndex values and nonnegative MIB indices")
		}
		seen[p.IfIndex] = true
	}
	return nil
}

type Neighbour struct {
	IfIndex        int    `json:"ifIndex"`
	LocalPort      int    `json:"localPort"`
	Name           string `json:"name"`
	ChassisMAC     string `json:"chassisMAC"`
	PortID         string `json:"portID"`
	Infrastructure bool   `json:"infrastructure"`
}
type Port struct {
	Index          int      `json:"index"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Alias          string   `json:"alias"`
	MAC            string   `json:"mac"`
	Admin          string   `json:"admin"`
	Operational    string   `json:"operational"`
	SpeedMbps      *float64 `json:"speedMbps"`
	VLANs          []int    `json:"vlans"`
	Uplink         bool     `json:"uplink"`
	UplinkEvidence []string `json:"uplinkEvidence"`
	PoE            string   `json:"poe"`
	PowerWatts     *float64 `json:"powerWatts"`
	PoESource      string   `json:"poeSource"`
}
type FDB struct {
	MAC        string `json:"mac"`
	BridgePort int    `json:"bridgePort"`
	IfIndex    int    `json:"ifIndex"`
	FDBID      int    `json:"fdbID"`
	VLANs      []int  `json:"vlans"`
	State      string `json:"state"`
	Source     string `json:"source"`
}
type Power struct {
	Group   int      `json:"group"`
	Port    int      `json:"port"`
	IfIndex int      `json:"ifIndex"`
	State   string   `json:"state"`
	Watts   *float64 `json:"watts"`
	Source  string   `json:"source"`
}
type Mapping struct {
	DeviceID   string   `json:"deviceID"`
	IP         string   `json:"ip"`
	MAC        string   `json:"mac"`
	SwitchID   string   `json:"switchID"`
	SwitchName string   `json:"switchName"`
	Port       int      `json:"port"`
	PortName   string   `json:"portName"`
	VLANs      []int    `json:"vlans"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
	Link       string   `json:"link"`
	SpeedMbps  *float64 `json:"speedMbps"`
	PoE        string   `json:"poe"`
	PowerWatts *float64 `json:"powerWatts"`
	At         string   `json:"at"`
}
type Report struct {
	AssetID     string      `json:"assetID"`
	SiteID      string      `json:"siteID"`
	SwitchName  string      `json:"switchName"`
	Address     string      `json:"address"`
	At          string      `json:"at"`
	SystemName  string      `json:"systemName"`
	Description string      `json:"description"`
	Ports       []Port      `json:"ports"`
	FDB         []FDB       `json:"fdb"`
	Neighbours  []Neighbour `json:"neighbours"`
	Power       []Power     `json:"power"`
	Mappings    []Mapping   `json:"mappings"`
	Warnings    []string    `json:"warnings"`
	Cancelled   bool        `json:"cancelled"`
}
