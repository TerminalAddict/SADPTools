package onvif

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/vault"
)

const media1NS = "http://www.onvif.org/ver10/media/wsdl"
const media2NS = "http://www.onvif.org/ver20/media/wsdl"

type MediaProfile struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Codec                 string `json:"codec"`
	Width                 int    `json:"width"`
	Height                int    `json:"height"`
	token                 string
	ptzToken, sourceToken string
}
type MediaCatalog struct {
	Profiles            []MediaProfile
	endpoint, namespace string
	offset              time.Duration
}

// MediaProfiles reads services and encoder metadata. No encoding configuration is
// changed. Service URLs must remain on the explicitly selected HTTP authority.
func (c *Client) MediaProfiles(ctx context.Context, secret vault.Secret, offsetSeconds int) (MediaCatalog, error) {
	if offsetSeconds < -300 || offsetSeconds > 300 {
		return MediaCatalog{}, errors.New("ONVIF clock correction must be within five minutes")
	}
	offset := time.Duration(offsetSeconds) * time.Second
	data, err := c.soap(ctx, c.target.URL, deviceNS, "GetServices", "<tds:IncludeCapability>false</tds:IncludeCapability>", secret, offset)
	services := map[string]string{}
	if err == nil {
		var doc struct {
			Body struct {
				Response struct {
					Services []struct{ Namespace, XAddr string } `xml:"Service"`
				} `xml:"GetServicesResponse"`
			}
		}
		err = decode(data, &doc)
		clear(data)
		if err != nil {
			return MediaCatalog{}, err
		}
		for _, s := range doc.Body.Response.Services {
			if s.Namespace == media1NS || s.Namespace == media2NS {
				services[s.Namespace] = s.XAddr
			}
		}
	} else if !errors.Is(err, ErrUnsupported) {
		return MediaCatalog{}, err
	}
	if len(services) == 0 {
		data, err = c.soap(ctx, c.target.URL, deviceNS, "GetCapabilities", "<tds:Category>Media</tds:Category>", secret, offset)
		if err != nil {
			return MediaCatalog{}, err
		}
		var doc struct {
			Body struct {
				Response struct {
					Capabilities struct{ Media struct{ XAddr string } }
				} `xml:"GetCapabilitiesResponse"`
			}
		}
		err = decode(data, &doc)
		clear(data)
		if err != nil {
			return MediaCatalog{}, err
		}
		services[media1NS] = doc.Body.Response.Capabilities.Media.XAddr
	}
	for _, namespace := range []string{media2NS, media1NS} {
		endpoint := services[namespace]
		if endpoint == "" {
			continue
		}
		base, _ := url.Parse(c.target.URL)
		target, e := url.Parse(endpoint)
		if e != nil || !ValidXAddr(endpoint, c.target.Address) || !sameHTTPAuthority(base, target) {
			return MediaCatalog{}, errors.New("ONVIF media service changed address or port; use an explicitly selected stream endpoint")
		}
		data, err = c.soap(ctx, endpoint, namespace, "GetProfiles", "", secret, offset)
		if errors.Is(err, ErrUnsupported) {
			continue
		}
		if err != nil {
			return MediaCatalog{}, err
		}
		catalog := MediaCatalog{endpoint: endpoint, namespace: namespace, offset: offset}
		catalog.Profiles, err = parseMediaProfiles(data, namespace)
		clear(data)
		if err != nil {
			return MediaCatalog{}, err
		}
		if len(catalog.Profiles) > 0 {
			return catalog, nil
		}
	}
	return MediaCatalog{}, errors.New("no supported ONVIF video profiles; select the Hikvision endpoint or an explicit custom URL")
}

// An explicit default port is the same origin (camera:80 == camera for HTTP).
// Different schemes, hosts and effective ports still require a new selection.
func sameHTTPAuthority(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return a != nil && b != nil && a.Scheme == b.Scheme && a.Hostname() == b.Hostname() && port(a) == port(b)
}

type mediaEncoder struct {
	Encoding   string
	Resolution struct{ Width, Height int }
}

func parseMediaProfiles(data []byte, namespace string) ([]MediaProfile, error) {
	var doc struct {
		Body struct {
			Response struct {
				Profiles []struct {
					Token                     string `xml:"token,attr"`
					Name                      string
					VideoEncoderConfiguration *mediaEncoder
					PTZConfiguration          struct {
						Token string `xml:"token,attr"`
					}
					VideoSourceConfiguration struct{ SourceToken string }
					Configurations           struct {
						VideoEncoder *mediaEncoder
						PTZ          struct {
							Token string `xml:"token,attr"`
						}
						VideoSource struct{ SourceToken string }
					}
				} `xml:"Profiles"`
			} `xml:"GetProfilesResponse"`
		}
	}
	if err := decode(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Body.Response.Profiles) > 64 {
		return nil, errors.New("too many ONVIF profiles")
	}
	out := []MediaProfile{}
	for _, p := range doc.Body.Response.Profiles {
		encoder := p.VideoEncoderConfiguration
		if namespace == media2NS {
			encoder = p.Configurations.VideoEncoder
		}
		if encoder == nil {
			continue
		}
		if p.Token == "" || len(p.Token) > 512 {
			return nil, errors.New("invalid ONVIF profile token")
		}
		sum := sha256.Sum256([]byte(namespace + "\x00" + p.Token))
		codec := strings.ToUpper(encoder.Encoding)
		switch codec {
		case "H264", "H265", "JPEG", "MPEG4":
		default:
			codec = "Unknown"
		}
		w, h := encoder.Resolution.Width, encoder.Resolution.Height
		if w < 0 || h < 0 || w > 65536 || h > 65536 {
			return nil, errors.New("invalid ONVIF profile dimensions")
		}
		ptzToken, sourceToken := p.PTZConfiguration.Token, p.VideoSourceConfiguration.SourceToken
		if namespace == media2NS {
			ptzToken, sourceToken = p.Configurations.PTZ.Token, p.Configurations.VideoSource.SourceToken
		}
		out = append(out, MediaProfile{ptzToken: ptzToken, sourceToken: sourceToken, ID: hex.EncodeToString(sum[:16]), Name: bounded(p.Name), Codec: codec, Width: w, Height: h, token: p.Token})
	}
	return out, nil
}

// StreamURI returns a protected value to Go only. The caller must validate the
// returned destination again before providing credentials to its media engine.
func (c *Client) StreamURI(ctx context.Context, catalog MediaCatalog, id string, snapshot bool, secret vault.Secret) (string, error) {
	var token string
	for _, p := range catalog.Profiles {
		if p.ID == id {
			token = p.token
			break
		}
	}
	if token == "" {
		return "", errors.New("selected ONVIF profile is no longer available")
	}
	operation := "GetStreamUri"
	parameters := ""
	if snapshot {
		operation = "GetSnapshotUri"
	} else if catalog.namespace == media2NS {
		parameters = "<tds:Protocol>RTSP</tds:Protocol>"
	} else {
		parameters = `<tds:StreamSetup xmlns:tt="http://www.onvif.org/ver10/schema"><tt:Stream>RTP-Unicast</tt:Stream><tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport></tds:StreamSetup>`
	}
	parameters += "<tds:ProfileToken>" + xmlText(token) + "</tds:ProfileToken>"
	data, err := c.soap(ctx, catalog.endpoint, catalog.namespace, operation, parameters, secret, catalog.offset)
	if err != nil {
		return "", err
	}
	defer clear(data)
	var doc struct {
		Body struct {
			Stream struct {
				URI      string `xml:"Uri"`
				MediaURI struct {
					URI string `xml:"Uri"`
				} `xml:"MediaUri"`
			} `xml:"GetStreamUriResponse"`
			Snapshot struct {
				URI      string `xml:"Uri"`
				MediaURI struct {
					URI string `xml:"Uri"`
				} `xml:"MediaUri"`
			} `xml:"GetSnapshotUriResponse"`
		}
	}
	if err = decode(data, &doc); err != nil {
		return "", err
	}
	raw := doc.Body.Stream.URI
	if raw == "" {
		raw = doc.Body.Stream.MediaURI.URI
	}
	if snapshot {
		raw = doc.Body.Snapshot.URI
		if raw == "" {
			raw = doc.Body.Snapshot.MediaURI.URI
		}
	}
	if len(raw) == 0 || len(raw) > 2048 {
		return "", errors.New("ONVIF response did not contain a usable stream URI")
	}
	return raw, nil
}
