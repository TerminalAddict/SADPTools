// Package streaming implements bounded, read-only camera previews without a GUI
// dependency, listening socket, external media process or public control API.
package streaming

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/TerminalAddict/SADPTools/internal/vault"
)

const MaxFrame = 4 << 20
const MaxWidth, MaxHeight = 4096, 2160

var ErrAuthentication = errors.New("camera authentication was rejected; check the username, password and stream permissions")
var ErrEndpoint = errors.New("stream endpoint was rejected; use the selected camera's numeric IPv4 address and an authorised port")
var ErrCodec = errors.New("no supported video track; select an H.264 stream, or try H.265 only on a compatible decoder")
var ErrMedia = errors.New("invalid or oversized camera media; try the sub stream or a different profile")

// Source never crosses the Go/frontend boundary and is never persisted.
type Source struct {
	address, raw, mode, pin string
	secret                  vault.Secret
}

func (Source) String() string               { return "protected camera stream" }
func (s Source) GoString() string           { return s.String() }
func (Source) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }

func NewSource(address, raw, mode, pin string, secret vault.Secret) (Source, error) {
	if _, err := ValidateURL(address, raw); err != nil {
		return Source{}, err
	}
	if mode != "rtsp" && mode != "mjpeg" && mode != "snapshot" {
		return Source{}, errors.New("select RTSP, MJPEG or snapshot refresh")
	}
	u, _ := url.Parse(raw)
	if (mode == "rtsp") != (u.Scheme == "rtsp" || u.Scheme == "rtsps") {
		return Source{}, ErrEndpoint
	}
	if err := ValidateCredentials(secret); err != nil {
		return Source{}, err
	}
	if _, err := TLSConfig(pin); err != nil {
		return Source{}, err
	}
	return Source{address: address, raw: raw, mode: mode, pin: pin, secret: secret}, nil
}

func ValidateCredentials(secret vault.Secret) error {
	if len(secret.Username) > 256 || len(secret.Password) > 4096 || strings.ContainsAny(secret.Username+secret.Password, "\r\n\x00") {
		return errors.New("invalid credential size or characters")
	}
	return nil
}

// ValidateURL accepts an exact, explicitly selected numeric address. No DNS,
// userinfo, alternate destinations, fragments, URL schemes or shell sources.
func ValidateURL(address, raw string) (*url.URL, error) {
	ip := net.ParseIP(address)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || address == "255.255.255.255" || len(raw) > 2048 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return nil, ErrEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != address || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrEndpoint
	}
	switch u.Scheme {
	case "rtsp", "rtsps", "http", "https":
	default:
		return nil, ErrEndpoint
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return nil, ErrEndpoint
		}
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

// TLSConfig preserves normal verification or trusts only an explicitly approved
// leaf fingerprint. A pin never grants trust to another endpoint automatically.
func TLSConfig(pin string) (*tls.Config, error) {
	c := &tls.Config{MinVersion: tls.VersionTLS12}
	if pin == "" {
		return c, nil
	}
	expected, err := hex.DecodeString(pin)
	if err != nil || len(expected) != 32 {
		return nil, errors.New("invalid certificate SHA-256 fingerprint")
	}
	c.InsecureSkipVerify = true // Replaced by exact leaf verification below.
	c.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) > 0 {
			sum := sha256.Sum256(state.PeerCertificates[0].Raw)
			if bytes.Equal(sum[:], expected) {
				return nil
			}
		}
		return errors.New("camera certificate does not match the approved fingerprint")
	}
	return c, nil
}

func endpointPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	case "rtsps":
		return "322"
	default:
		return "554"
	}
}

type Profile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Info struct {
	MIME      string `json:"mime"`
	Codec     string `json:"codec"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Transport string `json:"transport"`
}

// Packet kinds: 0 JSON control, 1 fMP4 bytes, 2 JPEG image. Sources and secrets
// cannot be encoded as control messages; only the explicit Status/Info types.
type Packet struct {
	Kind byte
	Data []byte
}
type Sink interface {
	Information(Info) error
	Write(Packet) error
}
type Engine interface {
	Play(context.Context, Source, Sink) error
}
