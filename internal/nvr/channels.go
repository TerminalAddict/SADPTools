package nvr

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/isapi"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"strconv"
	"time"
)

type Stream struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Codec          string   `json:"codec"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	ConfiguredKbps *float64 `json:"configuredKbps"`
	InputChannel   string   `json:"inputChannel"`
	Source         string   `json:"source"`
}
type Channel struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Address        string   `json:"address"`
	Model          string   `json:"model"`
	MAC            string   `json:"mac"`
	Serial         string   `json:"serial"`
	Enabled        *bool    `json:"enabled"`
	Status         string   `json:"status"`
	Recording      string   `json:"recording"`
	Streams        []Stream `json:"streams"`
	LinkedDeviceID string   `json:"linkedDeviceID"`
	Confidence     string   `json:"confidence"`
	Source         string   `json:"source"`
}
type Inventory struct {
	NVRID            string    `json:"nvrID"`
	At               string    `json:"at"`
	Provider         string    `json:"provider"`
	RecorderEvidence string    `json:"recorderEvidence"`
	Channels         []Channel `json:"channels"`
	Warnings         []string  `json:"warnings"`
}
type Reader interface {
	Read(context.Context, string, vault.Secret) ([]byte, error)
}

func Read(ctx context.Context, r Reader, s vault.Secret) (Inventory, error) {
	out := Inventory{At: time.Now().UTC().Format(time.RFC3339Nano), Provider: "Hikvision ISAPI", Channels: []Channel{}, Warnings: []string{}}
	data, e := r.Read(ctx, "/ISAPI/ContentMgmt/InputProxy/channels", s)
	if e != nil {
		return out, e
	}
	out.Channels, e = ParseChannels(data)
	clear(data)
	if e != nil {
		return out, e
	}
	out.RecorderEvidence = "Device exposes ISAPI input-proxy channel inventory"
	data, e = r.Read(ctx, "/ISAPI/ContentMgmt/InputProxy/channels/status", s)
	if e != nil {
		if errors.Is(e, isapi.ErrAuthentication) {
			return out, e
		}
		out.Warnings = append(out.Warnings, "Channel connection status unavailable; unknown does not mean offline")
	} else {
		if e = ApplyStatus(out.Channels, data); e != nil {
			out.Warnings = append(out.Warnings, e.Error())
		}
		clear(data)
	}
	data, e = r.Read(ctx, "/ISAPI/Streaming/channels", s)
	if e != nil {
		out.Warnings = append(out.Warnings, "Advertised stream mapping unavailable; no stream IDs invented")
	} else {
		streams, err := ParseStreams(data)
		clear(data)
		if err != nil {
			out.Warnings = append(out.Warnings, err.Error())
		} else {
			for i := range out.Channels {
				for _, st := range streams {
					found := false
					for j, advertised := range out.Channels[i].Streams {
						if advertised.ID == st.ID {
							found = true
							if st.InputChannel != "" && st.InputChannel != out.Channels[i].ID {
								out.Warnings = append(out.Warnings, "Conflicting stream mapping excluded")
								out.Channels[i].Streams[j].ID = ""
							} else {
								out.Channels[i].Streams[j] = st
							}
						}
					}
					if !found && st.InputChannel == out.Channels[i].ID {
						out.Channels[i].Streams = append(out.Channels[i].Streams, st)
					}
				}
			}
		}
	}
	for i := range out.Channels {
		valid := []Stream{}
		for _, st := range out.Channels[i].Streams {
			if st.ID != "" {
				valid = append(valid, st)
			}
		}
		out.Channels[i].Streams = valid
	}
	return out, nil
}
func ParseChannels(data []byte) ([]Channel, error) {
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return nil, e
	}
	if root.Name != "InputProxyChannelList" {
		return nil, isapi.ErrUnsupported
	}
	out := []Channel{}
	seen := map[string]bool{}
	for _, n := range root.Children {
		if n.Name != "InputProxyChannel" {
			continue
		}
		id := n.Value("id")
		if id == "" || len(id) > 64 || seen[id] || len(out) >= 1024 {
			return nil, errors.New("invalid, duplicate or excessive NVR channels")
		}
		seen[id] = true
		src := n.At("sourceInputPortDescriptor")
		ch := Channel{ID: id, Name: sanitize.Label(n.Value("name")), Address: sanitize.Label(src.Value("ipAddress")), Model: sanitize.Label(src.Value("deviceTypeName")), MAC: devices.NormalizeMAC(src.Value("macAddress")), Serial: sanitize.Label(src.Value("serialNumber")), Status: "unknown", Recording: "unknown", Streams: []Stream{}, Source: "device ISAPI input-proxy inventory", Confidence: "unmatched"}
		if ch.Address == "" {
			ch.Address = sanitize.Label(src.Value("hostName"))
		}
		if b, e := strconv.ParseBool(n.Value("enabled")); e == nil {
			ch.Enabled = &b
			if !b {
				ch.Status = "disabled"
			}
		}
		if src != nil && ch.Address == "" && ch.Serial == "" && ch.Status == "unknown" {
			ch.Status = "empty"
		}
		out = append(out, ch)
	}
	return out, nil
}
func ApplyStatus(channels []Channel, data []byte) error {
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return e
	}
	if root.Name != "InputProxyChannelStatusList" {
		return errors.New("channel status response unsupported")
	}
	statuses := map[string]*xmlsafe.Node{}
	for _, n := range root.All("InputProxyChannelStatus") {
		id := n.Value("id")
		if statuses[id] != nil {
			return errors.New("duplicate channel status")
		}
		statuses[id] = n
	}
	for i := range channels {
		ch := &channels[i]
		n := statuses[ch.ID]
		if n == nil || ch.Status == "disabled" {
			continue
		}
		if online, e := strconv.ParseBool(n.Value("online")); e == nil {
			if online {
				ch.Status = "connected"
			} else if ch.Status != "empty" {
				ch.Status = "offline"
			}
		}
		for _, n := range n.All("streamingProxyChannelId") {
			id, e := strconv.Atoi(n.Text)
			if e == nil && id > 0 && id <= 999999 {
				ch.Streams = append(ch.Streams, Stream{ID: n.Text, InputChannel: ch.ID, Source: "device advertised status stream ID"})
			}
		}
		if src := n.At("sourceInputPortDescriptor"); src != nil {
			if ch.Serial == "" {
				ch.Serial = sanitize.Label(src.Value("serialNumber"))
			}
			if ch.MAC == "" {
				ch.MAC = devices.NormalizeMAC(src.Value("macAddress"))
			}
		}
	}
	return nil
}
func ParseStreams(data []byte) ([]Stream, error) {
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return nil, e
	}
	if root.Name != "StreamingChannelList" {
		return nil, errors.New("stream mapping response unsupported")
	}
	out := []Stream{}
	seen := map[string]bool{}
	for _, n := range root.Children {
		if n.Name != "StreamingChannel" {
			continue
		}
		id := n.Value("id")
		num, e := strconv.Atoi(id)
		if e != nil || num < 1 || num > 999999 || seen[id] || len(out) >= 2048 {
			return nil, errors.New("invalid stream identifier")
		}
		seen[id] = true
		v := n.At("Video")
		st := Stream{ID: id, Name: sanitize.Label(n.Value("channelName")), InputChannel: v.Value("videoInputChannelID"), Codec: sanitize.Label(v.Value("videoCodecType")), Source: "device ISAPI videoInputChannelID (not inferred from stream ID)"}
		st.Width, _ = strconv.Atoi(v.Value("videoResolutionWidth"))
		st.Height, _ = strconv.Atoi(v.Value("videoResolutionHeight"))
		if k, e := strconv.ParseFloat(v.Value("constantBitRate"), 64); e == nil && k > 0 && k < 1e7 {
			st.ConfiguredKbps = &k
		}
		out = append(out, st)
	}
	return out, nil
}
func Match(channels []Channel, records []sites.Record) {
	for i := range channels {
		ch := &channels[i]
		id, ambiguous := sites.Match(records, devices.Device{MAC: ch.MAC, Serial: ch.Serial})
		if ambiguous {
			ch.Confidence = "conflicting identity — manual review"
			continue
		}
		if id != "" {
			ch.LinkedDeviceID = id
			ch.Confidence = "stable identifier"
			continue
		}
		matches := 0
		for _, r := range records {
			if !r.Removed && ch.Address != "" && r.Device.IPv4 == ch.Address {
				matches++
			}
		}
		if matches > 0 {
			ch.Confidence = "IP candidate only — manual link required"
		}
	}
}
