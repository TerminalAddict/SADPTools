package onvif

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"net/url"
	"time"
)

const ptzNS = "http://www.onvif.org/ver20/ptz/wsdl"
const eventsNS = "http://www.onvif.org/ver10/events/wsdl"
const imagingNS = "http://www.onvif.org/ver20/imaging/wsdl"

func (c *Client) sameAuthority(raw string) bool {
	u, e := streaming.ValidateURL(c.target.Address, raw)
	if e != nil {
		return false
	}
	base, _ := url.Parse(c.target.URL)
	return sameHTTPAuthority(base, u)
}
func (c *Client) service(ctx context.Context, ns, category string, s vault.Secret) (string, error) {
	data, e := c.soap(ctx, c.target.URL, deviceNS, "GetServices", "<tds:IncludeCapability>false</tds:IncludeCapability>", s, 0)
	if e == nil {
		root, err := xmlsafe.Parse(data)
		clear(data)
		if err != nil {
			return "", err
		}
		for _, n := range root.All("Service") {
			if n.Value("Namespace") == ns {
				endpoint := n.Value("XAddr")
				if !c.sameAuthority(endpoint) {
					return "", errors.New("ONVIF service changed host, port or scheme")
				}
				return endpoint, nil
			}
		}
	} else if !errors.Is(e, ErrUnsupported) {
		return "", e
	}
	data, e = c.soap(ctx, c.target.URL, deviceNS, "GetCapabilities", "<tds:Category>"+category+"</tds:Category>", s, 0)
	if e != nil {
		return "", e
	}
	defer clear(data)
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return "", e
	}
	for _, n := range root.All(category) {
		raw := n.Value("XAddr")
		if raw != "" {
			if !c.sameAuthority(raw) {
				return "", errors.New("ONVIF service changed host, port or scheme")
			}
			return raw, nil
		}
	}
	return "", ErrUnsupported
}
func (c *Client) VerifySerial(ctx context.Context, expected string, s vault.Secret) error {
	if expected == "" {
		return errors.New("device serial is required for safe control; read ONVIF information and save it first")
	}
	data, e := c.Read(ctx, "GetDeviceInformation", s)
	if e != nil {
		return e
	}
	defer clear(data)
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return e
	}
	nodes := root.All("GetDeviceInformationResponse")
	if len(nodes) != 1 || nodes[0].Value("SerialNumber") != expected {
		return errors.New("device identity changed; controls blocked")
	}
	return nil
}
func addressHeaders(endpoint, action string) string {
	return `<wsa:Action xmlns:wsa="http://www.w3.org/2005/08/addressing" s:mustUnderstand="1">` + xmlText(action) + `</wsa:Action><wsa:To xmlns:wsa="http://www.w3.org/2005/08/addressing" s:mustUnderstand="1">` + xmlText(endpoint) + `</wsa:To>`
}
func (c *Client) action(ctx context.Context, endpoint, ns, op, params, action, headers string, s vault.Secret, write bool) ([]byte, error) {
	if !c.sameAuthority(endpoint) {
		return nil, errors.New("ONVIF operation endpoint is not the selected authority")
	}
	return c.soapRequest(ctx, endpoint, ns, op, params, action, addressHeaders(endpoint, action)+headers, s, time.Duration(0), write)
}
