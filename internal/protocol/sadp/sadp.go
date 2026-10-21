// Package sadp implements Hikvision discovery and device-management messages.
// Wire templates and XML field mappings adapted from hikvision-tooling.
// Copyright (c) 2025 Cameron Newman. MIT: see LICENSES/hikvision-tooling-MIT.txt.
package sadp

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/devices"
)

const MulticastAddress = "239.255.255.250"
const Port = 37020
const MaxPacketSize = 65535

type probe struct {
	XMLName xml.Name `xml:"Probe"`
	UUID    string   `xml:"Uuid"`
	Types   string   `xml:"Types"`
}

func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func Probes(id string) ([][]byte, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("empty inquiry UUID")
	}
	result := make([][]byte, 0, 2)
	for _, kind := range []string{"inquiry", "inquiry_v32"} {
		b, err := xml.Marshal(probe{UUID: id, Types: kind})
		if err != nil {
			return nil, err
		}
		result = append(result, append([]byte(xml.Header), b...))
	}
	return result, nil
}

type response struct {
	XMLName      xml.Name `xml:"ProbeMatch"`
	UUID         string   `xml:"Uuid"`
	Types        string   `xml:"Types"`
	DeviceType   string   `xml:"DeviceType"`
	Description  string   `xml:"DeviceDescription"`
	Serial       string   `xml:"DeviceSN"`
	MAC          string   `xml:"MAC"`
	IPv4         string   `xml:"IPv4Address"`
	Mask         string   `xml:"IPv4SubnetMask"`
	Gateway      string   `xml:"IPv4Gateway"`
	Firmware     string   `xml:"SoftwareVersion"`
	Activated    string   `xml:"Activated"`
	DHCP         string   `xml:"DHCP"`
	HTTPPort     string   `xml:"HttpPort"`
	Name         string   `xml:"DeviceName"`
	CommandPort  string   `xml:"CommandPort"`
	ResetAbility string   `xml:"PasswordResetAbility"`
	ResetMode    string   `xml:"PasswordResetModeSecond"`
}

type Reply struct {
	UUID   string
	Device devices.Device
}

func Parse(data []byte) (Reply, error) {
	if len(data) > MaxPacketSize {
		return Reply{}, errors.New("oversized SADP datagram")
	}
	data = bytes.TrimSpace(bytes.TrimRight(data, "\x00"))
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var r response
	if err := decoder.Decode(&r); err != nil {
		return Reply{}, fmt.Errorf("SADP XML: %w", err)
	}
	// Decode alone accepts trailing roots. One datagram is one document.
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Reply{}, err
		}
		switch v := token.(type) {
		case xml.CharData:
			if len(bytes.TrimSpace(v)) != 0 {
				return Reply{}, errors.New("trailing XML data")
			}
		case xml.Comment:
		default:
			return Reply{}, errors.New("trailing XML document")
		}
	}
	r.Types = strings.TrimSpace(r.Types)
	if r.Types != "" && r.Types != "inquiry" && r.Types != "inquiry_v32" {
		return Reply{}, errors.New("not an inquiry reply")
	}
	d := devices.Device{
		Model: strings.TrimSpace(r.Description), DeviceType: strings.TrimSpace(r.DeviceType),
		MAC: devices.NormalizeMAC(r.MAC), Serial: strings.TrimSpace(r.Serial),
		IPv4: ipv4(r.IPv4), SubnetMask: ipv4(r.Mask), Gateway: ipv4(r.Gateway),
		Firmware: strings.TrimSpace(r.Firmware), Name: strings.TrimSpace(r.Name),
		PasswordResetAbility: strings.TrimSpace(r.ResetAbility), PasswordResetMode: strings.TrimSpace(r.ResetMode),
		Activated: optionalBool(r.Activated), DHCP: optionalBool(r.DHCP),
	}
	if p, err := strconv.Atoi(strings.TrimSpace(r.HTTPPort)); err == nil && p > 0 && p <= 65535 {
		d.HTTPPort = &p
	}
	if p, err := strconv.Atoi(strings.TrimSpace(r.CommandPort)); err == nil && p > 0 && p <= 65535 {
		d.CommandPort = &p
	}
	if d.Key() == "" {
		return Reply{}, errors.New("reply has neither a valid MAC nor serial number")
	}
	d.ID = d.Key()
	return Reply{UUID: strings.TrimSpace(r.UUID), Device: d}, nil
}

func ipv4(s string) string {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || !a.Is4() {
		return ""
	}
	return a.String()
}

func optionalBool(s string) *bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		v := true
		return &v
	case "false", "0":
		v := false
		return &v
	default:
		return nil
	}
}
