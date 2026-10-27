package diagnostics

import (
	"bytes"
	"encoding/xml"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"net/url"
	"regexp"
	"strings"
)

type Capability struct {
	ID        string `json:"id"`
	Protocol  string `json:"protocol"`
	Operation string `json:"operation"`
	Purpose   string `json:"purpose"`
}

var Catalogue = []Capability{
	{"onvif-users", "onvif", "GetUsers", "Reported account roles; username/password fields redacted"},
	{"onvif-services", "onvif", "GetServices", "Advertised service namespaces and endpoints"},
	{"onvif-capabilities", "onvif", "GetCapabilities", "Advertised capabilities (not verified by advertisement alone)"},
	{"onvif-device", "onvif", "GetDeviceInformation", "Manufacturer, model, firmware and hardware identity"},
	{"onvif-profiles", "onvif", "GetProfiles", "Media profiles; protected tokens redacted in output"},
	{"onvif-network", "onvif", "GetNetworkInterfaces", "Interface/address configuration"},
	{"onvif-protocols", "onvif", "GetNetworkProtocols", "Enabled HTTP/HTTPS/RTSP service settings"},
	{"onvif-clock", "onvif", "GetSystemDateAndTime", "Device clock, time zone and daylight-saving mode"},
	{"onvif-dns", "onvif", "GetDNS", "DNS configuration"},
	{"onvif-ntp", "onvif", "GetNTP", "Time synchronization settings"},
	{"isapi-device", "isapi", "/ISAPI/System/deviceInfo", "Device identity and exposed hardware/platform identifiers"},
	{"isapi-storage", "isapi", "/ISAPI/ContentMgmt/Storage/hdd", "Disk state, capacity and free space when exposed"},
	{"isapi-clock", "isapi", "/ISAPI/System/time", "Device clock/time zone"},
	{"isapi-streams", "isapi", "/ISAPI/Streaming/channels", "Configured stream metadata"},
}

func FindCapability(id string) (Capability, bool) {
	for _, c := range Catalogue {
		if c.ID == id {
			return c, true
		}
	}
	return Capability{}, false
}

type Probe struct {
	Endpoints   []ServiceEndpoint `json:"endpoints"`
	Capability  Capability        `json:"capability"`
	Step        Step              `json:"step"`
	ContentType string            `json:"contentType"`
	Response    string            `json:"response"`
}

// ScrubXML reconstructs XML from a bounded parser. It removes whole protected
// subtrees/attributes and literal credentials, never returning a failed parse's
// raw bytes. Namespaces remain visible for interoperability diagnosis.
func ScrubXML(data []byte, secrets ...string) (string, error) {
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return "", e
	}
	clean := func(s string) string {
		for _, v := range secrets {
			if v != "" {
				s = strings.ReplaceAll(s, v, "[redacted]")
			}
		}
		return sanitize.Text(s)
	}
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	var write func(*xmlsafe.Node) error
	write = func(n *xmlsafe.Node) error {
		start := xml.StartElement{Name: xml.Name{Local: n.Name, Space: publicNamespace(n.Space, secrets...)}}
		protected := protectedField(n.Name)
		for k, v := range n.Attrs {
			if (k == "Name" || k == "name") && protectedField(v) {
				protected = true
			}
		}
		for k, v := range n.Attrs {
			if protected || protectedField(k) {
				v = "[redacted]"
			} else {
				v = clean(v)
			}
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: k}, Value: v})
		}
		if e := enc.EncodeToken(start); e != nil {
			return e
		}
		if protected {
			enc.EncodeToken(xml.CharData("[redacted]"))
		} else {
			value := clean(n.Text)
			if n.Name == "Namespace" {
				if ns := publicNamespace(strings.TrimSpace(n.Text), secrets...); ns != "" {
					value = ns
				}
			}
			enc.EncodeToken(xml.CharData(value))
			for _, c := range n.Children {
				if e := write(c); e != nil {
					return e
				}
			}
		}
		return enc.EncodeToken(start.End())
	}
	if e = write(root); e != nil {
		return "", e
	}
	if e = enc.Flush(); e != nil {
		return "", e
	}
	return b.String(), nil
}

// Preserve only public protocol namespace identifiers, never arbitrary URI
// contents supplied by a device. Unknown namespaces are omitted, not followed.
var namespaceChars = regexp.MustCompile(`^[A-Za-z0-9/:._-]+$`)

func publicNamespace(raw string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(raw, secret) {
			return ""
		}
	}
	if len(raw) > 256 || !namespaceChars.MatchString(raw) {
		return ""
	}
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	switch u.Host {
	case "www.onvif.org", "www.w3.org", "schemas.xmlsoap.org", "docs.oasis-open.org", "www.isapi.org", "www.hikvision.com":
		return raw
	}
	return ""
}

type ServiceEndpoint struct {
	Namespace string `json:"namespace"`
	Scheme    string `json:"scheme"`
	Authority string `json:"authority"`
	Path      string `json:"path"`
	State     string `json:"state"`
}

func ServiceEndpoints(data []byte, target string, secrets ...string) []ServiceEndpoint {
	out := []ServiceEndpoint{}
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return out
	}
	for _, n := range root.All("Service") {
		if len(out) >= 64 {
			break
		}
		item := ServiceEndpoint{Namespace: publicNamespace(n.Value("Namespace"), secrets...), State: "advertised; not a capability verification"}
		raw := n.Value("XAddr")
		u, e := url.Parse(raw)
		if e != nil || u.Hostname() != target || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || len(raw) > 2048 {
			item.State = "rejected endpoint — not a plain service address on the selected device"
		} else {
			item.Scheme = u.Scheme
			item.Authority = u.Host
			item.Path = u.EscapedPath()
			for _, secret := range secrets {
				if secret != "" {
					item.Path = strings.ReplaceAll(item.Path, secret, "[redacted]")
				}
			}
			item.Path = sanitize.Label(item.Path)
		}
		out = append(out, item)
	}
	return out
}

func protectedField(name string) bool {
	if sanitize.SensitiveKey(name) {
		return true
	}
	name = strings.ToLower(name)
	for _, part := range []string{"plate", "snapshot", "image", "raw", "binary", "session", "cookie", "nonce", "challenge", "signature", "digest", "csrf", "bearer", "authenticate"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}
