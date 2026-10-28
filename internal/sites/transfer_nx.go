package sites

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/devices"
)

// Matches NX Camera Info's Report/Camera JSON, not an Nx database or API dump.
// Credential errors, raw URLs and arbitrary properties never enter inventory.
type nxCamera struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	IP               string            `json:"ip"`
	Host             string            `json:"host"`
	URL              string            `json:"url"`
	ONVIFURL         string            `json:"onvif_url"`
	StreamURLs       map[string]string `json:"stream_urls"`
	Username         string            `json:"username"`
	Password         *string           `json:"password"`
	HasPassword      bool              `json:"has_password"`
	CredentialStatus string            `json:"credential_status"`
	Vendor           string            `json:"vendor"`
	Model            string            `json:"model"`
	MAC              string            `json:"mac"`
	Serial           string            `json:"serial"`
	Firmware         string            `json:"firmware"`
	Group            string            `json:"group"`
	ServerName       string            `json:"server_name"`
	HTTPPort         string            `json:"http_port"`
	Virtual          bool              `json:"virtual"`
}

func decodeNX(data []byte) (b backupData, warnings []string, err error) {
	var report struct {
		Summary *struct {
			PasswordsRevealed bool `json:"passwords_revealed"`
		} `json:"summary"`
		Cameras []nxCamera `json:"cameras"`
	}
	if json.Unmarshal(data, &report) != nil || report.Summary == nil || report.Cameras == nil || len(report.Cameras) > 10000 {
		return b, nil, errors.New("choose an NX Camera Info JSON report with summary and cameras, or a SADPTools site file")
	}
	b.Version = 2
	b.Site = Site{ID: ID(), Name: "Imported NX site", Created: now()}
	virtual, hidden, noIP, badMAC, badPort, badEndpoints := 0, 0, 0, 0, 0, 0
	for i, n := range report.Cameras {
		if n.Virtual {
			virtual++
			continue
		}
		c := transferCamera{Name: n.Name, Vendor: n.Vendor, Model: n.Model, MAC: n.MAC, Serial: n.Serial, Firmware: n.Firmware, Location: n.Group}
		// Preserve Nx's record reference as a note, never as an ONVIF camera UUID.
		c.Notes = "Imported from NX Camera Info."
		if n.ID != "" {
			c.Notes += "\nNX record: " + n.ID
		}
		if n.ServerName != "" {
			c.Notes += "\nNX server: " + n.ServerName
		}
		u, _ := url.Parse(n.URL)
		candidates := []string{n.IP, n.Host}
		if u != nil {
			candidates = append(candidates, u.Hostname())
		}
		for _, raw := range candidates {
			if ip, e := netip.ParseAddr(raw); e == nil && ip.Unmap().Is4() {
				c.IPv4 = ip.Unmap().String()
				break
			}
		}
		if c.IPv4 == "" {
			noIP++
		}
		if c.MAC != "" && devices.NormalizeMAC(c.MAC) == "" {
			badMAC++
			c.MAC = ""
		}
		if n.HTTPPort != "" {
			if port := nxPort(n.HTTPPort); port != nil {
				c.HTTPPort = port
			} else {
				badPort++
			}
		}
		// Only a concrete URL scheme identifies HTTPS/RTSP. media_port may be a
		// vendor SDK port, so it must not be invented as the RTSP port.
		if u != nil && u.Hostname() != "" {
			port := nxPort(u.Port())
			switch strings.ToLower(u.Scheme) {
			case "http":
				if port == nil && u.Port() == "" {
					port = nxPort("80")
				}
				if c.HTTPPort == nil {
					c.HTTPPort = port
				}
			case "https":
				if port == nil && u.Port() == "" {
					port = nxPort("443")
				}
				c.HTTPSPort = port
			case "rtsp":
				if port == nil && u.Port() == "" {
					port = nxPort("554")
				}
				c.RTSPPort = port
			}
		}
		// A masked report is useful for inventory, but [redacted] is not a password.
		if n.CredentialStatus == "recovered" && n.Password != nil && (report.Summary.PasswordsRevealed || !n.HasPassword && *n.Password == "") {
			if n.HasPassword && *n.Password == "" {
				clearBackup(&b)
				return b, nil, fmt.Errorf("camera %d: inconsistent NX password metadata", i+1)
			}
			c.Credentials = &transferCredential{Username: n.Username, Password: n.Password}
		} else if n.HasPassword || n.CredentialStatus == "decode_error" {
			hidden++
		}
		if err = addTransferCamera(&b, c); err != nil {
			clearBackup(&b)
			return b, nil, fmt.Errorf("camera %d: %w", i+1, err)
		}
		if addNXPreviewEndpoints(&b, n) {
			badEndpoints++
		}
	}
	if err = addNXWitness(&b, data); err != nil {
		clearBackup(&b)
		return b, nil, err
	}
	warnings = []string{"Imported cameras are saved offline; their connectivity, credentials and discovery capabilities have not been checked."}
	for _, v := range []struct {
		n       int
		message string
	}{
		{virtual, "virtual camera records were omitted"},
		{hidden, "camera passwords were hidden or unavailable; export JSON with passwords revealed in NX Camera Info to carry known passwords"},
		{noIP, "cameras have no usable IPv4 address; their metadata was retained without resolving hostnames"},
		{badMAC, "unrecognised MAC addresses were left empty"},
		{badPort, "unrecognised HTTP ports were left empty"},
		{badEndpoints, "cameras had unsupported, excessive or different-address stream/ONVIF URLs omitted; enter a custom URL in Live View if needed"},
	} {
		if v.n > 0 {
			warnings = append(warnings, fmt.Sprintf("%d %s.", v.n, v.message))
		}
	}
	return b, warnings, nil
}

func nxPort(raw string) *int {
	n, e := strconv.Atoi(raw)
	if e != nil || n < 1 || n > 65535 {
		return nil
	}
	return &n
}
