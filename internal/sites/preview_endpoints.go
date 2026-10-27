package sites

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
)

const previewEndpointsKind = "preview-endpoints"

// Endpoint paths and queries can contain access tokens. Keep them in encrypted
// assets, outside inventory, reports, logs and desktop bindings.
type previewEndpoints struct {
	Version int      `json:"version"`
	Address string   `json:"address"`
	RTSP    []string `json:"rtsp"`
	ONVIF   string   `json:"onvif,omitempty"`
}

type PreviewEndpoints struct{ data previewEndpoints }

func (PreviewEndpoints) String() string               { return "protected camera endpoints" }
func (p PreviewEndpoints) GoString() string           { return p.String() }
func (PreviewEndpoints) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }
func (p *PreviewEndpoints) Clear()                    { p.data = previewEndpoints{} }
func (p PreviewEndpoints) ONVIF() string              { return p.data.ONVIF }
func (p PreviewEndpoints) StreamCount() int           { return len(p.data.RTSP) }

// NX stream slots are ordered main, sub, then additional streams. Never guess
// a vendor path or fall back to another URL after an authentication failure.
func (p PreviewEndpoints) Stream(profile string) (string, error) {
	i := 0
	switch profile {
	case "main":
	case "auto", "sub", "":
		if len(p.data.RTSP) > 1 {
			i = 1
		}
	default:
		var e error
		i, e = strconv.Atoi(strings.TrimPrefix(profile, "saved:"))
		if e != nil || !strings.HasPrefix(profile, "saved:") {
			return "", errors.New("select an imported stream")
		}
	}
	if i < 0 || i >= len(p.data.RTSP) {
		return "", errors.New("selected imported stream is unavailable")
	}
	return p.data.RTSP[i], nil
}

func decodePreviewEndpoints(data []byte) (previewEndpoints, error) {
	var p previewEndpoints
	invalid := errors.New("invalid protected camera endpoints")
	if len(data) > 64<<10 || json.Unmarshal(data, &p) != nil || p.Version != 1 || len(p.RTSP) > 16 || len(p.RTSP) == 0 && p.ONVIF == "" {
		return p, invalid
	}
	for _, raw := range p.RTSP {
		u, e := streaming.ValidateURL(p.Address, raw)
		if e != nil || (u.Scheme != "rtsp" && u.Scheme != "rtsps") {
			return previewEndpoints{}, invalid
		}
	}
	if p.ONVIF != "" && !onvif.ValidXAddr(p.ONVIF, p.Address) {
		return previewEndpoints{}, invalid
	}
	return p, nil
}

func (s *Store) previewEndpointAsset(site, device string) (Asset, error) {
	assets, e := s.assetsMetadata(site)
	if e != nil {
		return Asset{}, e
	}
	for _, a := range assets {
		if a.DeviceID == device && a.Kind == previewEndpointsKind && a.Encrypted {
			return a, nil
		}
	}
	return Asset{}, nil
}

func (s *Store) HasPreviewEndpoints(site, device string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, e := s.previewEndpointAsset(site, device)
	return e == nil && a.ID != ""
}

// ReadPreviewEndpoints is Go-only and requires the local vault. Refuse stale
// endpoints after an inventory IP change instead of sending credentials there.
func (s *Store) ReadPreviewEndpoints(site, device string) (PreviewEndpoints, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.record(site, device)
	if e != nil {
		return PreviewEndpoints{}, e
	}
	a, e := s.previewEndpointAsset(site, device)
	if e != nil || a.ID == "" {
		return PreviewEndpoints{}, errors.New("no imported stream addresses; re-import the camera export or enter a custom URL")
	}
	_, data, e := s.asset(site, a.ID)
	if e != nil {
		return PreviewEndpoints{}, e
	}
	defer clear(data)
	p, e := decodePreviewEndpoints(data)
	if e != nil {
		return PreviewEndpoints{}, e
	}
	if p.Address != r.Device.IPv4 {
		return PreviewEndpoints{}, errors.New("camera address changed since import; re-import its current stream addresses or use a custom URL")
	}
	return PreviewEndpoints{data: p}, nil
}

// Invalid/foreign addresses are omitted from NX imports, with a count-only
// warning. URL userinfo is discarded: the assigned camera credential is used.
func addNXPreviewEndpoints(b *backupData, n nxCamera) bool {
	r := b.Devices[len(b.Devices)-1]
	p := previewEndpoints{Version: 1, Address: r.Device.IPv4}
	skipped := false
	keys := make([]string, 0, len(n.StreamURLs))
	for k := range n.StreamURLs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, ea := strconv.Atoi(keys[i])
		c, ec := strconv.Atoi(keys[j])
		if ea == nil && ec == nil {
			return a < c
		}
		return keys[i] < keys[j]
	})
	seen := map[string]bool{}
	add := func(raw string) {
		if raw == "" {
			return
		}
		u, e := url.Parse(raw)
		if e != nil {
			skipped = true
			return
		}
		u.User = nil
		raw = u.String()
		u, e = streaming.ValidateURL(p.Address, raw)
		if e != nil || (u.Scheme != "rtsp" && u.Scheme != "rtsps") || len(p.RTSP) >= 16 {
			skipped = true
			return
		}
		if !seen[raw] {
			p.RTSP = append(p.RTSP, raw)
			seen[raw] = true
		}
	}
	for _, k := range keys {
		add(n.StreamURLs[k])
	}
	if len(p.RTSP) == 0 && (strings.HasPrefix(n.URL, "rtsp://") || strings.HasPrefix(n.URL, "rtsps://")) {
		add(n.URL)
	}
	if n.ONVIFURL != "" {
		if onvif.ValidXAddr(n.ONVIFURL, p.Address) {
			p.ONVIF = n.ONVIFURL
		} else {
			skipped = true
		}
	}
	if len(p.RTSP) > 0 || p.ONVIF != "" {
		payload, _ := json.Marshal(p)
		b.Assets = append(b.Assets, backupAsset{Meta: Asset{ID: ID(), SiteID: b.Site.ID, DeviceID: r.ID, Kind: previewEndpointsKind, Title: "Imported camera endpoints", MIME: "application/octet-stream", Encrypted: true, Created: now(), Bytes: len(payload)}, Payload: payload})
		if r.Device.RTSPPort == nil && len(p.RTSP) > 0 {
			u, _ := url.Parse(p.RTSP[0])
			port := u.Port()
			if port == "" {
				port = "554"
				if u.Scheme == "rtsps" {
					port = "322"
				}
			}
			b.Devices[len(b.Devices)-1].Device.RTSPPort = nxPort(port)
		}
	}
	return skipped
}
