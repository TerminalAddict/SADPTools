package sadp

import (
	"bytes"
	"crypto/aes"
	"crypto/md5" // Required by the device's SADP protocol, not password storage.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TerminalAddict/SADPTools/internal/devices"
)

// NetworkProfile is ephemeral: authentication material never enters inventory,
// settings, exports or logs. Only ParseNetworkProfile creates a usable profile.
// See docs/device-management.md for protocol evidence and compatibility limits.
type NetworkProfile struct {
	Device    devices.Device
	encrypted bool
	salt      string
	preserved [][2]string
	valid     bool
}

func (p NetworkProfile) String() string       { return "SADP network profile (authentication omitted)" }
func (p NetworkProfile) GoString() string     { return p.String() }
func (p NetworkProfile) LogValue() slog.Value { return slog.StringValue(p.String()) }

func ParseNetworkProfile(data []byte) (NetworkProfile, error) {
	var p NetworkProfile
	fields, err := parseCommandFields(data)
	if err != nil {
		return p, err
	}
	r, err := Parse(data)
	if err != nil || r.Device.MAC == "" {
		return p, errors.New("network check did not identify the device")
	}
	p.Device = r.Device
	if p.Device.Activated == nil || !*p.Device.Activated {
		return p, errors.New("camera must explicitly report active before changing its network")
	}
	encrypted := optionalBool(fields["Encrypt"])
	if encrypted == nil {
		return p, errors.New("camera did not advertise a supported network authentication profile")
	}
	p.encrypted = *encrypted
	p.salt = fields["Salt"]
	if p.salt != "" {
		if len(p.salt) != 64 {
			return NetworkProfile{}, errors.New("unsupported network authentication salt format")
		}
		for _, b := range []byte(p.salt) {
			if b < 0x21 || b > 0x7e {
				return NetworkProfile{}, errors.New("unsupported network authentication salt format")
			}
		}
	}
	// Preserve unrelated fields from the fresh reply, including an explicitly
	// empty IPv6 value. Never substitute a default port or invented IPv6 setting.
	if p.Device.HTTPPort == nil {
		return NetworkProfile{}, errors.New("camera did not advertise a valid HTTP port to preserve")
	}
	p.preserved = append(p.preserved, [2]string{"HttpPort", strconv.Itoa(*p.Device.HTTPPort)})
	ipv6Count := 0
	for _, key := range []string{"IPv6Address", "IPv6Gateway", "IPv6MaskLen", "SDKOverTLSPort"} {
		value, exists := fields[key]
		if !exists {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "IPv6Address", "IPv6Gateway":
			ipv6Count++
			if value != "" {
				ip, parseErr := netip.ParseAddr(value)
				if parseErr != nil || !ip.Is6() || ip.Is4In6() || ip.Zone() != "" {
					return NetworkProfile{}, errors.New("camera advertised unsupported IPv6 settings")
				}
			}
		case "IPv6MaskLen", "SDKOverTLSPort":
			max := 65535
			if key == "IPv6MaskLen" {
				ipv6Count++
				max = 128
			}
			n, parseErr := strconv.Atoi(value)
			if parseErr != nil || n < 0 || n > max {
				return NetworkProfile{}, errors.New("camera advertised unsupported network settings")
			}
			value = strconv.Itoa(n)
		}
		p.preserved = append(p.preserved, [2]string{key, value})
	}
	if ipv6Count != 0 && ipv6Count != 3 {
		return NetworkProfile{}, errors.New("camera advertised incomplete IPv6 settings; cannot safely preserve them")
	}
	p.valid = true
	return p, nil
}

func (p NetworkProfile) password(id, password string) (string, error) {
	// The salted SADP profile hashes admin + salt + password. The observed SDK
	// supports up to 16 password bytes; do not silently truncate longer inputs.
	if !p.valid || password == "" || len(password) > 128 || !utf8.ValidString(password) || strings.ContainsRune(password, 0) {
		return "", errors.New("unsupported network authentication input")
	}
	input := []byte(password)
	defer clear(input)
	if p.salt != "" {
		if len(password) > 16 {
			return "", errors.New("this salted SADP network profile supports passwords up to 16 bytes; longer passwords are not truncated")
		}
		h := sha256.New()
		_, _ = h.Write([]byte("admin"))
		_, _ = h.Write([]byte(p.salt))
		_, _ = h.Write(input)
		digest := h.Sum(nil)
		clear(input)
		input = []byte(hex.EncodeToString(digest))
		defer clear(input)
		clear(digest)
	}
	key := md5.Sum(input)
	defer clear(key[:])
	if !p.encrypted {
		return base64.StdEncoding.EncodeToString(key[:]), nil
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", errors.New("could not prepare network authentication")
	}
	var plain, cipher [64]byte
	defer clear(plain[:])
	defer clear(cipher[:])
	copy(plain[:16], key[:])
	plain[16] = ';'
	copy(plain[17:], id)
	for offset := 0; offset < len(plain); offset += aes.BlockSize {
		block.Encrypt(cipher[offset:offset+aes.BlockSize], plain[offset:offset+aes.BlockSize])
	}
	return base64.StdEncoding.EncodeToString(cipher[:]), nil
}

// NetworkCommand is the only network write encoder. Encrypt is negotiated from
// a fresh inquiry, never inferred from a timeout. No plaintext fallback exists.
func NetworkCommand(id, mac, password string, network NetworkSettings, profile NetworkProfile) ([]byte, error) {
	mac = devices.NormalizeMAC(mac)
	if !profile.valid || mac == "" || mac != profile.Device.MAC || len(id) != 36 {
		return nil, errors.New("invalid network command identity or profile")
	}
	for i, c := range []byte(id) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c == '-' {
				continue
			}
		} else if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return nil, errors.New("invalid network command UUID")
	}
	auth, err := profile.password(id, password)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	e := xml.NewEncoder(&b)
	root := xml.StartElement{Name: xml.Name{Local: "Probe"}}
	if err = e.EncodeToken(root); err != nil {
		return nil, err
	}
	fields := [][2]string{{"Uuid", id}, {"Types", "update"}, {"PWErrorParse", "true"}, {"MAC", activationMAC(mac)}, {"Password", auth},
		{"IPv4Address", network.IPv4}, {"CommandPort", strconv.Itoa(network.CommandPort)}, {"IPv4SubnetMask", network.SubnetMask}, {"IPv4Gateway", network.Gateway}, {"DHCP", strconv.FormatBool(network.DHCP)}}
	fields = append(fields, profile.preserved...)
	for _, f := range fields {
		element := xml.StartElement{Name: xml.Name{Local: f[0]}}
		if f[0] == "Password" && profile.salt != "" {
			element.Attr = []xml.Attr{{Name: xml.Name{Local: "bSalt"}, Value: "true"}}
		}
		if err = e.EncodeElement(f[1], element); err != nil {
			return nil, errors.New("cannot encode network command")
		}
	}
	if err = e.EncodeToken(root.End()); err != nil {
		return nil, err
	}
	if err = e.Flush(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
