package toolkit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

type PreviewOptions struct {
	StreamID         string `json:"streamID"`
	SiteID           string `json:"siteID"`
	DeviceID         string `json:"deviceID"`
	Method           string `json:"method"`
	Profile          string `json:"profile"`
	Channel          int    `json:"channel"`
	Port             int    `json:"port"`
	URL              string `json:"url"`
	StreamPin        string `json:"streamPin"`
	ONVIFPin         string `json:"onvifPin"`
	ClockOffset      int    `json:"clockOffset"`
	CredentialSite   string `json:"credentialSite"`
	CredentialRecord string `json:"credentialRecord"`
	SeparateONVIF    bool   `json:"separateONVIF"`
}
type PreviewCredential struct {
	SiteID   string `json:"siteID"`
	RecordID string `json:"recordID"`
	Label    string `json:"label"`
}
type PreviewDevice struct {
	Device       devices.Device      `json:"device"`
	Credentials  []PreviewCredential `json:"credentials"`
	SavedPreview bool                `json:"savedPreview"`
}

func (m *Manager) previewDevice(site, id string) (devices.Device, error) {
	if site != "" {
		rows, e := m.Store.Records(site)
		if e != nil {
			return devices.Device{}, errors.New("saved camera could not be read")
		}
		for _, r := range rows {
			if r.ID == id && !r.Removed {
				return validatePreviewDevice(r.Device)
			}
		}
	} else {
		for _, d := range m.Controller.Snapshot().Devices {
			if d.ID == id {
				return validatePreviewDevice(d)
			}
		}
	}
	return devices.Device{}, errors.New("select a current discovered camera or a saved site device")
}
func validatePreviewDevice(d devices.Device) (devices.Device, error) {
	ip := net.ParseIP(d.IPv4)
	if d.IdentityConflict || ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || d.IPv4 == "255.255.255.255" {
		return d, errors.New("camera needs an unambiguous identity and usable IPv4 address")
	}
	if d.Activated != nil && !*d.Activated {
		return d, errors.New("activate this camera before opening Live View")
	}
	return d, nil
}
func samePreviewDevice(a, b devices.Device) bool {
	if a.IdentityConflict || b.IdentityConflict {
		return false
	}
	am, bm := devices.NormalizeMAC(a.MAC), devices.NormalizeMAC(b.MAC)
	if am != "" && bm != "" && am != bm {
		return false
	}
	if a.Serial != "" && b.Serial != "" && a.Serial != b.Serial {
		return false
	}
	return am != "" && am == bm || a.Serial != "" && a.Serial == b.Serial
}
func (m *Manager) PreviewDevice(site, id string) (PreviewDevice, error) {
	d, e := m.previewDevice(site, id)
	if e != nil {
		return PreviewDevice{}, e
	}
	out := PreviewDevice{Device: d, Credentials: []PreviewCredential{}}
	out.SavedPreview = site != "" && m.Store.HasPreviewEndpoints(site, id)
	sites, e := m.Store.Sites()
	if e != nil {
		return out, nil
	}
	for _, s := range sites {
		rows, e := m.Store.Records(s.ID)
		if e != nil {
			continue
		}
		for _, r := range rows {
			if !r.Removed && ((site == s.ID && id == r.ID) || samePreviewDevice(d, r.Device)) && ((r.CredentialMode == "profile" && r.CredentialID != "") || (r.CredentialMode == "inherit" && s.DefaultCredential != "")) {
				out.Credentials = append(out.Credentials, PreviewCredential{s.ID, r.ID, s.Name + " · assigned device/site credential"})
			}
		}
	}
	return out, nil
}

// PreparePreview keeps network work in the stream worker, outside discovery and
// configuration task locks. Only explicitly selected credentials are retrieved.
func (m *Manager) PreparePreview(player *streaming.Manager, o PreviewOptions, secret, onvifSecret vault.Secret) (streaming.Ticket, error) {
	if e := streaming.ValidateCredentials(secret); e != nil {
		return streaming.Ticket{}, e
	}
	if e := streaming.ValidateCredentials(onvifSecret); e != nil {
		return streaming.Ticket{}, e
	}
	d, e := m.previewDevice(o.SiteID, o.DeviceID)
	if e != nil {
		return streaming.Ticket{}, e
	}
	if o.Channel < 1 || o.Channel > 256 || o.Port < 0 || o.Port > 65535 || o.ClockOffset < -300 || o.ClockOffset > 300 {
		return streaming.Ticket{}, errors.New("check channel (1–256), port and ONVIF clock correction")
	}
	if o.Profile == "" {
		o.Profile = "sub"
	}
	switch o.Method {
	case "saved", "onvif", "onvif-snapshot", "hikvision", "rtsp", "mjpeg", "snapshot":
	default:
		return streaming.Ticket{}, errors.New("select a supported Live View method")
	}
	if o.Method == "rtsp" || o.Method == "mjpeg" || o.Method == "snapshot" {
		if _, e = streaming.NewSource(d.IPv4, o.URL, o.Method, o.StreamPin, secret); e != nil {
			return streaming.Ticket{}, e
		}
	}
	if _, e = streaming.TLSConfig(o.ONVIFPin); e != nil {
		return streaming.Ticket{}, e
	}
	if _, e = streaming.TLSConfig(o.StreamPin); e != nil {
		return streaming.Ticket{}, e
	}
	credentialSource := "Session credentials"
	var alive func() bool
	var endpoints sites.PreviewEndpoints
	if o.Method == "saved" {
		endpoints, e = m.Store.ReadPreviewEndpoints(o.SiteID, o.DeviceID)
		if e != nil {
			return streaming.Ticket{}, e
		}
		alive = m.Store.CredentialSessionActive
	}
	if o.CredentialSite != "" {
		saved, e := m.previewDevice(o.CredentialSite, o.CredentialRecord)
		if e != nil {
			return streaming.Ticket{}, e
		}
		if !(o.SiteID == o.CredentialSite && o.DeviceID == o.CredentialRecord) && !samePreviewDevice(d, saved) {
			return streaming.Ticket{}, errors.New("credential assignment does not match this camera identity")
		}
		var assigned string
		secret, assigned, e = m.Store.DeviceSecret(o.CredentialSite, o.CredentialRecord)
		if e == nil && assigned == "none" {
			return streaming.Ticket{}, errors.New("this camera no longer has an assigned credential; enter session credentials")
		}
		if e != nil {
			return streaming.Ticket{}, e
		}
		credentialSource = "Encrypted vault · device/site assignment"
		alive = m.Store.CredentialSessionActive
	}
	if !o.SeparateONVIF {
		onvifSecret = secret
	}
	dispose := func() { secret.Clear(); onvifSecret.Clear(); o.URL = ""; endpoints.Clear() }
	resolve := func(ctx context.Context, state func(string, string), profiles func([]streaming.Profile)) (streaming.Source, error) {
		raw, mode := o.URL, o.Method
		if mode == "saved" {
			if endpoints.StreamCount() > 0 {
				state("resolving_profile", "Using the camera stream address from the imported site")
				choices := []streaming.Profile{}
				for i := 0; i < endpoints.StreamCount(); i++ {
					choices = append(choices, streaming.Profile{ID: fmt.Sprintf("saved:%d", i), Name: fmt.Sprintf("Imported stream %d", i+1)})
				}
				profiles(choices)
				var e error
				raw, e = endpoints.Stream(o.Profile)
				if e != nil {
					return streaming.Source{}, e
				}
				mode = "rtsp"
			} else {
				d.XAddrs = []string{endpoints.ONVIF()}
				mode = "onvif"
			}
		}
		if mode == "onvif" || mode == "onvif-snapshot" {
			state("resolving_profile", "Authenticating and reading ONVIF video profiles")
			target, e := targetForONVIF(d, o.ONVIFPin)
			if e != nil {
				return streaming.Source{}, e
			}
			client, e := onvif.NewClient(target)
			if e != nil {
				return streaming.Source{}, e
			}
			resolveCtx, cancel := context.WithTimeout(ctx, 24*time.Second)
			defer cancel()
			catalog, e := client.MediaProfiles(resolveCtx, onvifSecret, o.ClockOffset)
			if e != nil {
				if errors.Is(e, onvif.ErrAuthentication) {
					return streaming.Source{}, streaming.ErrAuthentication
				}
				return streaming.Source{}, e
			}
			choices := []streaming.Profile{}
			for _, p := range catalog.Profiles {
				choices = append(choices, streaming.Profile{ID: p.ID, Name: p.Name, Codec: p.Codec, Width: p.Width, Height: p.Height})
			}
			profiles(choices)
			selected := o.Profile
			if selected == "auto" || selected == "sub" || selected == "main" {
				selected = choosePreviewProfile(catalog.Profiles, selected, mode == "onvif-snapshot")
				if selected == "" {
					return streaming.Source{}, streaming.ErrCodec
				}
			}
			raw, e = client.StreamURI(resolveCtx, catalog, selected, mode == "onvif-snapshot", onvifSecret)
			if e != nil {
				if errors.Is(e, onvif.ErrAuthentication) {
					return streaming.Source{}, streaming.ErrAuthentication
				}
				return streaming.Source{}, e
			}
			if mode == "onvif-snapshot" {
				mode = "snapshot"
			} else {
				mode = "rtsp"
			}
		} else if mode == "hikvision" {
			if o.Profile != "auto" && o.Profile != "sub" && o.Profile != "main" {
				return streaming.Source{}, errors.New("select Main or Sub for the Hikvision endpoint")
			}
			port := o.Port
			if port == 0 && d.RTSPPort != nil {
				port = int(*d.RTSPPort)
			}
			if port == 0 {
				port = 554
			}
			stream := 2
			if o.Profile == "main" {
				stream = 1
			}
			if o.StreamID != "" {
				if _, e := m.ChannelStream(o.SiteID, o.DeviceID, "", o.StreamID); e != nil {
					return streaming.Source{}, e
				}
			}
			raw = "rtsp://" + net.JoinHostPort(d.IPv4, strconv.Itoa(port)) + fmt.Sprintf("/Streaming/channels/%d", o.Channel*100+stream)
			if o.StreamID != "" {
				raw = "rtsp://" + net.JoinHostPort(d.IPv4, strconv.Itoa(port)) + "/Streaming/channels/" + o.StreamID
			}
			mode = "rtsp"
		}
		return streaming.NewSource(d.IPv4, raw, mode, o.StreamPin, secret)
	}
	return player.Prepare(resolve, dispose, alive, credentialSource)
}

func choosePreviewProfile(profiles []onvif.MediaProfile, preference string, snapshot bool) string {
	var chosen string
	var area int
	for _, p := range profiles {
		if !snapshot && p.Codec != "H264" && p.Codec != "H265" {
			continue
		}
		n := p.Width * p.Height
		if chosen == "" || area == 0 && n > 0 || n > 0 && ((preference == "main" && n > area) || (preference != "main" && n < area)) {
			chosen = p.ID
			area = n
		}
	}
	return chosen
}

func (m *Manager) PreviewCertificate(site, id, raw string, forONVIF bool) (streaming.Certificate, error) {
	d, e := m.previewDevice(site, id)
	if e != nil {
		return streaming.Certificate{}, e
	}
	if forONVIF {
		t, e := targetForONVIF(d, "")
		if e != nil {
			return streaming.Certificate{}, e
		}
		raw = t.URL
	}
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "rtsps://") {
		return streaming.Certificate{}, errors.New("select an HTTPS or RTSPS endpoint to inspect its certificate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return streaming.InspectCertificate(ctx, d.IPv4, raw)
}
