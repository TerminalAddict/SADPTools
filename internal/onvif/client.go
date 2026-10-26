package onvif

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1" // Required by the ONVIF WS-Security UsernameToken PasswordDigest standard.
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/icholy/digest"
)

var ErrAuthentication = errors.New("ONVIF authentication denied; check the credential profile and the camera's ONVIF user permissions")
var ErrUnsupported = errors.New("ONVIF operation unsupported or rejected by this device")

const deviceNS = "http://www.onvif.org/ver10/device/wsdl"

// Target is an explicit, previously discovered service endpoint. A certificate
// pin is accepted only after the operator confirms that exact fingerprint.
type Target struct {
	Address           string
	URL               string
	Source            string
	CertificateSHA256 string
}
type Client struct {
	offset time.Duration
	target Target
	http   *http.Client
}

func NewClient(t Target) (*Client, error) {
	if !ValidXAddr(t.URL, t.Address) {
		return nil, errors.New("ONVIF service URL must belong to the discovered IPv4 device")
	}
	if t.Source != "" && net.ParseIP(t.Source).To4() == nil {
		return nil, errors.New("invalid source IPv4 address")
	}
	var config *tls.Config
	if t.CertificateSHA256 != "" {
		pin, e := hex.DecodeString(t.CertificateSHA256)
		if e != nil || len(pin) != 32 {
			return nil, errors.New("invalid certificate fingerprint")
		}
		config = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) > 0 {
				sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
				if bytes.Equal(sum[:], pin) {
					return nil
				}
			}
			return errors.New("device certificate changed")
		}}
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(t.Source)}}
	transport := &http.Transport{DialContext: dialer.DialContext, TLSClientConfig: config, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 4 * time.Second, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10}
	return &Client{target: t, http: &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func usernameToken(s vault.Secret, now time.Time, nonce []byte) string {
	created := now.UTC().Format(time.RFC3339Nano)
	raw := append(append(append([]byte{}, nonce...), []byte(created)...), []byte(s.Password)...)
	hash := sha1.Sum(raw)
	clear(raw)
	return `<wsse:Security s:mustUnderstand="1" xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"><wsse:UsernameToken><wsse:Username>` + xmlText(s.Username) + `</wsse:Username><wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">` + base64.StdEncoding.EncodeToString(hash[:]) + `</wsse:Password><wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">` + base64.StdEncoding.EncodeToString(nonce) + `</wsse:Nonce><wsu:Created>` + created + `</wsu:Created></wsse:UsernameToken></wsse:Security>`
}

var readOperations = map[string]bool{"GetDeviceInformation": true, "GetNetworkInterfaces": true, "GetDNS": true, "GetNTP": true, "GetSystemDateAndTime": true, "GetNetworkProtocols": true, "GetNetworkDefaultGateway": true}

func (c *Client) Read(ctx context.Context, operation string, secret vault.Secret) ([]byte, error) {
	if !readOperations[operation] {
		return nil, errors.New("only implemented read-only ONVIF operations are allowed")
	}
	return c.soap(ctx, c.target.URL, deviceNS, operation, "", secret, 0)
}

// soap retains the read-only path. Explicit control clients use soapRequest
// with write=true so network failures never imply that a write did not happen.
func (c *Client) soap(ctx context.Context, endpoint, namespace, operation, parameters string, secret vault.Secret, offset time.Duration) ([]byte, error) {
	return c.soapRequest(ctx, endpoint, namespace, operation, parameters, namespace+"/"+operation, "", secret, offset, false)
}

var ErrOutcomeUnconfirmed = errors.New("ONVIF write outcome unconfirmed — verify before retry")

func (c *Client) soapRequest(ctx context.Context, endpoint, namespace, operation, parameters, action, headers string, secret vault.Secret, offset time.Duration, write bool) ([]byte, error) {
	if offset == 0 {
		offset = c.offset
	}
	security := ""
	if secret.Username != "" {
		nonce := make([]byte, 20)
		rand.Read(nonce)
		security = usernameToken(secret, time.Now().Add(offset), nonce)
		clear(nonce)
	}
	payload := "<tds:" + operation + "/>"
	if parameters != "" {
		payload = "<tds:" + operation + ">" + parameters + "</tds:" + operation + ">"
	}
	body := []byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="` + namespace + `"><s:Header>` + security + headers + `</s:Header><s:Body>` + payload + `</s:Body></s:Envelope>`)
	defer clear(body)
	do := func(auth string) (*http.Response, error) {
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if e != nil {
			return nil, errors.New("could not prepare ONVIF request")
		}
		req.GetBody = nil
		req.Header.Set("Content-Type", `application/soap+xml; charset=utf-8; action="`+action+`"`)
		req.Header.Set("Cache-Control", "no-store")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		return c.http.Do(req)
	}
	response, e := do("")
	if e != nil {
		if write {
			return nil, ErrOutcomeUnconfirmed
		}
		return nil, safeConnectionError(ctx, e)
	}
	if response.StatusCode == http.StatusUnauthorized && secret.Username != "" {
		challenge, err := digest.FindChallenge(response.Header)
		response.Body.Close()
		if err != nil {
			return nil, ErrAuthentication
		}
		parsed, _ := url.Parse(endpoint)
		credential, err := digest.Digest(challenge, digest.Options{Method: http.MethodPost, URI: parsed.RequestURI(), GetBody: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }, Count: 1, Username: secret.Username, Password: secret.Password})
		if err != nil {
			return nil, ErrAuthentication
		}
		// A 401 is a rejected unauthenticated request; answer its challenge once.
		// No network failure or uncertain write is retried.
		response, e = do(credential.String())
		if e != nil {
			if write {
				return nil, ErrOutcomeUnconfirmed
			}
			return nil, safeConnectionError(ctx, e)
		}
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, ErrAuthentication
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if write && response.StatusCode != 404 && response.StatusCode != 405 && response.StatusCode != 501 {
			return nil, ErrOutcomeUnconfirmed
		}
		return nil, ErrUnsupported
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if e != nil || len(data) > 1<<20 {
		clear(data)
		if write {
			return nil, ErrOutcomeUnconfirmed
		}
		return nil, errors.New("incomplete or oversized ONVIF response")
	}
	var envelope struct {
		XMLName xml.Name
		Body    struct {
			Fault *struct {
				Code struct {
					Value   string
					Subcode struct{ Value string }
				}
				Reason struct{ Text string }
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if e = decode(data, &envelope); e != nil {
		clear(data)
		if write {
			return nil, ErrOutcomeUnconfirmed
		}
		return nil, e
	}
	if envelope.XMLName.Local != "Envelope" || envelope.XMLName.Space != "http://www.w3.org/2003/05/soap-envelope" {
		clear(data)
		if write {
			return nil, ErrOutcomeUnconfirmed
		}
		return nil, errors.New("invalid ONVIF SOAP envelope")
	}
	if envelope.Body.Fault != nil {
		fault := envelope.Body.Fault
		code := fault.Code.Value + " " + fault.Code.Subcode.Value
		if strings.Contains(code, "NotAuthorized") || strings.Contains(code, "FailedAuthentication") || strings.Contains(code, "InvalidSecurity") {
			clear(data)
			return nil, ErrAuthentication
		}
		clear(data)
		return nil, ErrUnsupported
	}
	return data, nil
}
func safeConnectionError(ctx context.Context, e error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var cert *tls.CertificateVerificationError
	if errors.As(e, &cert) {
		return errors.New("ONVIF HTTPS certificate is not trusted; verify and explicitly accept its fingerprint before sending credentials")
	}
	return errors.New("ONVIF connection failed or timed out; no device changes were requested")
}
func decode(data []byte, v any) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	if e := d.Decode(v); e != nil {
		return errors.New("malformed ONVIF response")
	}
	for {
		t, e := d.Token()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return errors.New("malformed trailing ONVIF XML")
		}
		if s, ok := t.(xml.CharData); !ok || len(bytes.TrimSpace(s)) != 0 {
			return errors.New("unexpected trailing ONVIF XML")
		}
	}
}

type Information struct {
	Device   devices.Device `json:"device"`
	Warnings []string       `json:"warnings"`
}

func (c *Client) Information(ctx context.Context, base devices.Device, s vault.Secret) (Information, error) {
	result := Information{Device: base, Warnings: []string{}}
	data, e := c.Read(ctx, "GetDeviceInformation", s)
	if e != nil {
		return result, e
	}
	defer clear(data)
	var doc struct {
		Body struct {
			Info *struct{ Manufacturer, Model, FirmwareVersion, SerialNumber, HardwareId string } `xml:"GetDeviceInformationResponse"`
		} `xml:"Body"`
	}
	if e = decode(data, &doc); e != nil {
		return result, e
	}
	if doc.Body.Info == nil {
		return result, errors.New("device information missing from ONVIF response")
	}
	info := doc.Body.Info
	result.Device.Vendor = bounded(info.Manufacturer)
	result.Device.Model = bounded(info.Model)
	result.Device.Firmware = bounded(info.FirmwareVersion)
	result.Device.Serial = bounded(info.SerialNumber)
	network, e := c.Read(ctx, "GetNetworkInterfaces", s)
	if e != nil {
		result.Warnings = append(result.Warnings, e.Error())
	} else {
		defer clear(network)
		var n struct {
			Body struct {
				Response struct {
					Interfaces []struct {
						Enabled bool
						Info    struct{ HwAddress string }
						IPv4    struct {
							Enabled bool
							Config  struct {
								Manual []struct {
									Address      string
									PrefixLength int
								}
								FromDHCP struct {
									Address      string
									PrefixLength int
								}
								DHCP *bool
							}
						}
					} `xml:"NetworkInterfaces"`
				} `xml:"GetNetworkInterfacesResponse"`
			} `xml:"Body"`
		}
		if decode(network, &n) == nil {
			for _, iface := range n.Body.Response.Interfaces {
				if !iface.Enabled || !iface.IPv4.Enabled {
					continue
				}
				ips := iface.IPv4.Config.Manual
				ips = append(ips, iface.IPv4.Config.FromDHCP)
				for _, ip := range ips {
					if ip.Address == c.target.Address {
						mac := devices.NormalizeMAC(iface.Info.HwAddress)
						if mac != "" {
							result.Device.MAC = mac
						}
						if ip.PrefixLength >= 0 && ip.PrefixLength <= 32 {
							result.Device.SubnetMask = net.IP(net.CIDRMask(ip.PrefixLength, 32)).String()
						}
						result.Device.DHCP = iface.IPv4.Config.DHCP
					}
				}
			}
		}
	}
	protocols, e := c.Read(ctx, "GetNetworkProtocols", s)
	if e != nil {
		result.Warnings = append(result.Warnings, e.Error())
	} else {
		defer clear(protocols)
		var p struct {
			Body struct {
				Response struct {
					Protocols []struct {
						Name    string
						Enabled bool
						Port    []int
					} `xml:"NetworkProtocols"`
				} `xml:"GetNetworkProtocolsResponse"`
			} `xml:"Body"`
		}
		if decode(protocols, &p) == nil {
			for _, protocol := range p.Body.Response.Protocols {
				if !protocol.Enabled || len(protocol.Port) == 0 || protocol.Port[0] < 1 || protocol.Port[0] > 65535 {
					continue
				}
				port := protocol.Port[0]
				switch protocol.Name {
				case "HTTP":
					result.Device.HTTPPort = &port
				case "HTTPS":
					result.Device.HTTPSPort = &port
				case "RTSP":
					result.Device.RTSPPort = &port
				}
			}
		}
	}
	gateway, e := c.Read(ctx, "GetNetworkDefaultGateway", s)
	if e != nil {
		result.Warnings = append(result.Warnings, e.Error())
	} else {
		defer clear(gateway)
		var g struct {
			Body struct {
				Response struct {
					Gateway struct {
						Address []string `xml:"IPv4Address"`
					} `xml:"NetworkGateway"`
				} `xml:"GetNetworkDefaultGatewayResponse"`
			} `xml:"Body"`
		}
		if decode(gateway, &g) == nil && len(g.Body.Response.Gateway.Address) > 0 && net.ParseIP(g.Body.Response.Gateway.Address[0]).To4() != nil {
			result.Device.Gateway = g.Body.Response.Gateway.Address[0]
		}
	}
	result.Device.LastSeen = time.Now().UTC()
	result.Device.ID = result.Device.Key()
	return result, nil
}
func bounded(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return ""
	}
	return s
}

// Configuration returns only supported, read-only health data; absent values
// remain unknown. Errors are associated with individual reads, not hidden.
type Configuration struct {
	DNS    []string          `json:"dns"`
	NTP    []string          `json:"ntp"`
	UTC    string            `json:"utc"`
	Errors map[string]string `json:"errors"`
}

func (c *Client) Configuration(ctx context.Context, s vault.Secret) Configuration {
	out := Configuration{DNS: []string{}, NTP: []string{}, Errors: map[string]string{}}
	type host struct{ IPv4Address, IPv6Address, DNSname string }
	hosts := func(h []host) []string {
		r := []string{}
		for _, v := range h {
			for _, x := range []string{v.IPv4Address, v.IPv6Address, v.DNSname} {
				if x != "" && len(x) <= 253 {
					r = append(r, x)
				}
			}
		}
		return r
	}
	data, e := c.Read(ctx, "GetDNS", s)
	if e != nil {
		out.Errors["dns"] = e.Error()
	} else {
		var doc struct {
			Body struct {
				Response struct {
					DNS struct {
						FromDHCP bool
						Manual   []host `xml:"DNSManual"`
						DHCP     []host `xml:"DNSFromDHCP"`
					}
				} `xml:"GetDNSResponse"`
			} `xml:"Body"`
		}
		if decode(data, &doc) == nil {
			if doc.Body.Response.DNS.FromDHCP {
				out.DNS = hosts(doc.Body.Response.DNS.DHCP)
			} else {
				out.DNS = hosts(doc.Body.Response.DNS.Manual)
			}
		}
		clear(data)
	}
	data, e = c.Read(ctx, "GetNTP", s)
	if e != nil {
		out.Errors["ntp"] = e.Error()
	} else {
		var doc struct {
			Body struct {
				Response struct {
					NTP struct {
						FromDHCP bool
						Manual   []host `xml:"NTPManual"`
						DHCP     []host `xml:"NTPFromDHCP"`
					}
				} `xml:"GetNTPResponse"`
			} `xml:"Body"`
		}
		if decode(data, &doc) == nil {
			if doc.Body.Response.NTP.FromDHCP {
				out.NTP = hosts(doc.Body.Response.NTP.DHCP)
			} else {
				out.NTP = hosts(doc.Body.Response.NTP.Manual)
			}
		}
		clear(data)
	}
	data, e = c.Read(ctx, "GetSystemDateAndTime", s)
	if e != nil {
		out.Errors["clock"] = e.Error()
	} else {
		var doc struct {
			Body struct {
				Response struct {
					Clock struct {
						UTC *struct {
							Time struct{ Hour, Minute, Second int }
							Date struct{ Year, Month, Day int }
						} `xml:"UTCDateTime"`
					} `xml:"SystemDateAndTime"`
				} `xml:"GetSystemDateAndTimeResponse"`
			} `xml:"Body"`
		}
		if decode(data, &doc) == nil && doc.Body.Response.Clock.UTC != nil {
			v := doc.Body.Response.Clock.UTC
			stamp := fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02dZ", v.Date.Year, v.Date.Month, v.Date.Day, v.Time.Hour, v.Time.Minute, v.Time.Second)
			if _, e = time.Parse(time.RFC3339, stamp); e == nil {
				out.UTC = stamp
			}
		}
		clear(data)
	}
	return out
}
func servicePort(raw string) int {
	u, e := url.Parse(raw)
	if e != nil {
		return 0
	}
	if u.Port() != "" {
		p, _ := strconv.Atoi(u.Port())
		return p
	}
	if u.Scheme == "https" {
		return 443
	}
	return 80
}

// SetClockOffset applies an explicitly approved session correction to WS-Security timestamps.
func (c *Client) SetClockOffset(seconds int) error {
	if seconds < -300 || seconds > 300 {
		return errors.New("clock correction must be within five minutes")
	}
	c.offset = time.Duration(seconds) * time.Second
	return nil
}
