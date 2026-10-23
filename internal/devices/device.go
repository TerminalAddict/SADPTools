package devices

import (
	"net"
	"strings"
	"time"
)

type Interface struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Nil booleans and ports mean the device did not report a recognized value.
type Device struct {
	ID                   string      `json:"id"`
	Model                string      `json:"model"`
	DeviceType           string      `json:"deviceType"`
	IPv4                 string      `json:"ipv4"`
	SubnetMask           string      `json:"subnetMask"`
	Gateway              string      `json:"gateway"`
	MAC                  string      `json:"mac"`
	Serial               string      `json:"serial"`
	Firmware             string      `json:"firmware"`
	Activated            *bool       `json:"activated"`
	DHCP                 *bool       `json:"dhcp"`
	HTTPPort             *int        `json:"httpPort"`
	CommandPort          *int        `json:"commandPort"`
	PasswordResetAbility string      `json:"passwordResetAbility"`
	PasswordResetMode    string      `json:"passwordResetMode"`
	Name                 string      `json:"name"`
	Interfaces           []Interface `json:"interfaces"`
	FirstSeen            time.Time   `json:"firstSeen"`
	LastSeen             time.Time   `json:"lastSeen"`
}

func NormalizeMAC(s string) string {
	m, err := net.ParseMAC(strings.TrimSpace(s))
	if err != nil || len(m) != 6 || m[0]&1 != 0 || m.String() == "00:00:00:00:00:00" {
		return ""
	}
	return strings.ToUpper(m.String())
}

func (d Device) Key() string {
	if m := NormalizeMAC(d.MAC); m != "" {
		return "mac:" + m
	}
	if s := strings.TrimSpace(d.Serial); s != "" {
		return "serial:" + s
	}
	return ""
}
