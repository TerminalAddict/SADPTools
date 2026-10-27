package onvif

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"math"
	"strconv"
	"strings"
)

type ImageSetting struct {
	Name    string   `json:"name"`
	Value   string   `json:"value"`
	State   string   `json:"state"`
	Reason  string   `json:"reason"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
	Choices []string `json:"choices"`
}
type Imaging struct {
	ProfileID    string         `json:"profileID"`
	Settings     []ImageSetting `json:"settings"`
	PrivacyMasks string         `json:"privacyMasks"`
}

var imageFields = []string{"Brightness", "ColorSaturation", "Contrast", "Sharpness", "Exposure.Mode", "Exposure.ExposureTime", "Exposure.Gain", "Exposure.Iris", "WideDynamicRange.Mode", "WideDynamicRange.Level", "BacklightCompensation.Mode", "BacklightCompensation.Level", "WhiteBalance.Mode", "WhiteBalance.CrGain", "WhiteBalance.CbGain", "IrCutFilter"}

func (c *Client) imagingSource(ctx context.Context, catalog MediaCatalog, profile string, s vault.Secret) (string, string, error) {
	for _, p := range catalog.Profiles {
		if p.ID == profile && p.sourceToken != "" {
			endpoint, e := c.service(ctx, imagingNS, "Imaging", s)
			return endpoint, p.sourceToken, e
		}
	}
	return "", "", errors.New("select a current ONVIF profile with a video source")
}
func (c *Client) ReadImaging(ctx context.Context, catalog MediaCatalog, profile string, s vault.Secret) (Imaging, error) {
	out := Imaging{ProfileID: profile, Settings: []ImageSetting{}, PrivacyMasks: "unsupported — no hardware-validated privacy-mask adapter; verify camera output and recorded footage manually"}
	endpoint, source, e := c.imagingSource(ctx, catalog, profile, s)
	if e != nil {
		return out, e
	}
	param := "<tds:VideoSourceToken>" + xmlText(source) + "</tds:VideoSourceToken>"
	options, e := c.soap(ctx, endpoint, imagingNS, "GetOptions", param, s, 0)
	if e != nil {
		return out, e
	}
	opt, e := xmlsafe.Parse(options)
	clear(options)
	if e != nil {
		return out, e
	}
	data, e := c.soap(ctx, endpoint, imagingNS, "GetImagingSettings", param, s, 0)
	if e != nil {
		return out, e
	}
	root, e := xmlsafe.Parse(data)
	clear(data)
	if e != nil {
		return out, e
	}
	roots := root.All("ImagingSettings")
	opts := opt.All("ImagingOptions")
	if len(roots) != 1 || len(opts) != 1 {
		return out, errors.New("imaging settings/options response is incomplete")
	}
	for _, name := range imageFields {
		path := strings.Split(name, ".")
		setting := ImageSetting{Name: name, Value: roots[0].Value(path...), State: "unsupported", Reason: "Not advertised by GetOptions", Choices: []string{}}
		parent := opts[0]
		for _, part := range path[:len(path)-1] {
			parent = parent.At(part)
		}
		leaf := path[len(path)-1]
		if parent != nil {
			node := parent.At(leaf)
			if min, max, ok := rangeValues(node); ok {
				setting.Min = &min
				setting.Max = &max
			}
			choiceName := leaf
			if leaf == "Mode" {
				choiceName = "Mode"
			}
			for _, child := range parent.Children {
				if child.Name == choiceName || child.Name == choiceName+"s" {
					v := strings.TrimSpace(child.Text)
					if v != "" {
						setting.Choices = append(setting.Choices, v)
					}
				}
			}
		}
		if setting.Value != "" && (setting.Min != nil || len(setting.Choices) > 0) {
			setting.State = "supported"
			setting.Reason = "Advertised range/modes and current value read; explicit confirmation required"
		} else if setting.Value != "" {
			setting.State = "read-only"
			setting.Reason = "Value available; writable range/modes not advertised"
		}
		if len(path) == 2 && path[1] != "Mode" && roots[0].Value(path[0], "Mode") == "" {
			setting.State = "read-only"
			setting.Reason = "Required mode was not reported; a safe grouped write cannot be constructed"
		}
		out.Settings = append(out.Settings, setting)
	}
	for _, name := range []string{"HLC", "IR intensity", "Privacy masks"} {
		out.Settings = append(out.Settings, ImageSetting{Name: name, State: "unsupported", Reason: "No verified adapter for this setting; use camera web interface", Choices: []string{}})
	}
	return out, nil
}

// SetImageSetting changes one advertised field, once, after rechecking identity
// and the previous value. Readback is required before claiming success.
func (c *Client) SetImageSetting(ctx context.Context, catalog MediaCatalog, profile, serial, field, value, expected string, s vault.Secret) (string, error) {
	if e := c.VerifySerial(ctx, serial, s); e != nil {
		return "blocked", e
	}
	current, e := c.ReadImaging(ctx, catalog, profile, s)
	if e != nil {
		return "blocked", e
	}
	var setting *ImageSetting
	for i := range current.Settings {
		if current.Settings[i].Name == field {
			setting = &current.Settings[i]
		}
	}
	if setting == nil || setting.State != "supported" || setting.Value != expected {
		return "blocked", errors.New("setting unsupported or changed since preview; reload before writing")
	}
	valid := false
	if setting.Min != nil {
		v, e := strconv.ParseFloat(value, 64)
		valid = e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= *setting.Min && v <= *setting.Max
	} else {
		for _, v := range setting.Choices {
			valid = valid || v == value
		}
	}
	if !valid {
		return "blocked", errors.New("proposed value is outside advertised capabilities")
	}
	endpoint, source, e := c.imagingSource(ctx, catalog, profile, s)
	if e != nil {
		return "blocked", e
	}
	parts := strings.Split(field, ".")
	xml := "<tt:" + parts[len(parts)-1] + ">" + xmlText(value) + "</tt:" + parts[len(parts)-1] + ">"
	if len(parts) == 2 {
		if parts[1] != "Mode" {
			mode := ""
			for _, v := range current.Settings {
				if v.Name == parts[0]+".Mode" {
					mode = v.Value
				}
			}
			if mode == "" {
				return "blocked", errors.New("required imaging mode is unknown")
			}
			xml = "<tt:Mode>" + xmlText(mode) + "</tt:Mode>" + xml
		}
		xml = "<tt:" + parts[0] + ">" + xml + "</tt:" + parts[0] + ">"
	}
	body := "<tds:VideoSourceToken>" + xmlText(source) + "</tds:VideoSourceToken><tds:ImagingSettings xmlns:tt=\"http://www.onvif.org/ver10/schema\">" + xml + "</tds:ImagingSettings><tds:ForcePersistence>true</tds:ForcePersistence>"
	data, e := c.action(ctx, endpoint, imagingNS, "SetImagingSettings", body, imagingNS+"/SetImagingSettings", "", s, true)
	clear(data)
	if e != nil {
		return "unconfirmed", e
	}
	after, e := c.ReadImaging(ctx, catalog, profile, s)
	if e != nil {
		return "unconfirmed", ErrOutcomeUnconfirmed
	}
	for _, v := range after.Settings {
		if v.Name == field && v.Value == value {
			return "verified", nil
		}
	}
	return "unconfirmed", ErrOutcomeUnconfirmed
}
