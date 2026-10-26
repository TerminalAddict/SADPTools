package onvif

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"math"
	"strconv"
	"strings"
)

type ControlCapabilities struct {
	ProfileID     string   `json:"profileID"`
	PanTilt       bool     `json:"panTilt"`
	OpticalZoom   bool     `json:"opticalZoom"`
	Presets       bool     `json:"presets"`
	RelativeFocus bool     `json:"relativeFocus"`
	AutoFocus     bool     `json:"autoFocus"`
	Iris          bool     `json:"iris"`
	Reasons       []string `json:"reasons"`
}
type Preset struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Control struct {
	client                                                *Client
	secret                                                vault.Secret
	serial, profile, ptzEndpoint, imagingEndpoint, source string
	caps                                                  ControlCapabilities
	presetTokens                                          map[string]string
	focusMin, focusMax, irisMin, irisMax                  float64
}

func refID(value string) string { s := sha256.Sum256([]byte(value)); return hex.EncodeToString(s[:16]) }
func (c *Client) Controls(ctx context.Context, catalog MediaCatalog, profileID, serial string, s vault.Secret) (*Control, error) {
	var p *MediaProfile
	for i := range catalog.Profiles {
		if catalog.Profiles[i].ID == profileID {
			p = &catalog.Profiles[i]
			break
		}
	}
	if p == nil {
		return nil, errors.New("choose a specific current ONVIF profile for camera controls")
	}
	out := &Control{client: c, secret: s, serial: serial, profile: p.token, source: p.sourceToken, presetTokens: map[string]string{}, caps: ControlCapabilities{ProfileID: profileID, Reasons: []string{}}}
	if serial == "" {
		out.caps.Reasons = append(out.caps.Reasons, "Read and save the device serial before enabling physical controls")
		return out, nil
	}
	if p.ptzToken != "" {
		endpoint, e := c.service(ctx, ptzNS, "PTZ", s)
		if e == nil {
			out.ptzEndpoint = endpoint
			data, err := c.soap(ctx, endpoint, ptzNS, "GetConfigurationOptions", "<tds:ConfigurationToken>"+xmlText(p.ptzToken)+"</tds:ConfigurationToken>", s, 0)
			if err == nil {
				root, err := xmlsafe.Parse(data)
				if err == nil {
					out.caps.PanTilt = hasSpace(root, "ContinuousPanTiltVelocitySpace", "PanTiltSpaces/VelocityGenericSpace")
					out.caps.OpticalZoom = hasSpace(root, "ContinuousZoomVelocitySpace", "ZoomSpaces/VelocityGenericSpace")
				}
				clear(data)
			}
			if _, err := out.Presets(ctx); err == nil {
				out.caps.Presets = true
			}
		} else {
			out.caps.Reasons = append(out.caps.Reasons, "PTZ service unavailable: "+e.Error())
		}
	} else {
		out.caps.Reasons = append(out.caps.Reasons, "This profile has no PTZ configuration")
	}
	if out.source != "" {
		endpoint, e := c.service(ctx, imagingNS, "Imaging", s)
		if e == nil {
			out.imagingEndpoint = endpoint
			data, err := c.soap(ctx, endpoint, imagingNS, "GetMoveOptions", out.sourceParam(), s, 0)
			if err == nil {
				root, err := xmlsafe.Parse(data)
				if err == nil {
					for _, n := range root.All("Relative") {
						min, max, ok := rangeValues(n.At("Distance"))
						if ok && min < 0 && max > 0 {
							out.focusMin = min
							out.focusMax = max
							out.caps.RelativeFocus = true
						}
					}
				}
				clear(data)
			}
			data, err = c.soap(ctx, endpoint, imagingNS, "GetOptions", out.sourceParam(), s, 0)
			if err == nil {
				root, err := xmlsafe.Parse(data)
				if err == nil {
					for _, n := range root.All("AutoFocusModes") {
						out.caps.AutoFocus = out.caps.AutoFocus || strings.TrimSpace(n.Text) == "AUTO"
					}
					for _, n := range root.All("Iris") {
						min, max, ok := rangeValues(n)
						if ok && max > min {
							out.irisMin = min
							out.irisMax = max
							out.caps.Iris = true
						}
					}
				}
				clear(data)
			}
		}
	}
	if !out.caps.RelativeFocus && !out.caps.AutoFocus {
		out.caps.Reasons = append(out.caps.Reasons, "Motorised focus/autofocus not advertised")
	}
	out.caps.Reasons = append(out.caps.Reasons, "Preset rename is not offered: ONVIF SetPreset also replaces its stored position")
	return out, nil
}
func rangeValues(n *xmlsafe.Node) (float64, float64, bool) {
	if n == nil {
		return 0, 0, false
	}
	a, e := strconv.ParseFloat(n.Value("Min"), 64)
	b, f := strconv.ParseFloat(n.Value("Max"), 64)
	return a, b, e == nil && f == nil && !math.IsNaN(a) && !math.IsNaN(b) && !math.IsInf(a, 0) && !math.IsInf(b, 0)
}
func hasSpace(root *xmlsafe.Node, name, suffix string) bool {
	for _, n := range root.All(name) {
		a, b, ok := rangeValues(n.At("XRange"))
		if strings.HasSuffix(n.Value("URI"), suffix) && ok && a <= -1 && b >= 1 {
			if name == "ContinuousPanTiltVelocitySpace" {
				a, b, ok = rangeValues(n.At("YRange"))
				if !ok || a > -1 || b < 1 {
					continue
				}
			}
			return true
		}
	}
	return false
}
func (c *Control) Capabilities() ControlCapabilities { return c.caps }
func (c *Control) sourceParam() string {
	return "<tds:VideoSourceToken>" + xmlText(c.source) + "</tds:VideoSourceToken>"
}
func (c *Control) profileParam() string {
	return "<tds:ProfileToken>" + xmlText(c.profile) + "</tds:ProfileToken>"
}
func (c *Control) verify(ctx context.Context) error {
	return c.client.VerifySerial(ctx, c.serial, c.secret)
}
func (c *Control) command(ctx context.Context, endpoint, ns, op, params string) error {
	data, e := c.client.action(ctx, endpoint, ns, op, params, ns+"/"+op, "", c.secret, true)
	defer clear(data)
	if e != nil {
		return e
	}
	root, e := xmlsafe.Parse(data)
	if e != nil || len(root.All(op+"Response")) != 1 {
		return ErrOutcomeUnconfirmed
	}
	return nil
}
func (c *Control) Move(ctx context.Context, x, y, z float64) error {
	if !finiteUnit(x) || !finiteUnit(y) || !finiteUnit(z) || x == 0 && y == 0 && z == 0 {
		return errors.New("choose a bounded movement vector")
	}
	if (x != 0 || y != 0) && !c.caps.PanTilt || z != 0 && !c.caps.OpticalZoom {
		return ErrUnsupported
	}
	if e := c.verify(ctx); e != nil {
		return e
	}
	velocity := ""
	if c.caps.PanTilt {
		velocity += fmt.Sprintf(`<tt:PanTilt x="%.4f" y="%.4f"/>`, x, y)
	}
	if c.caps.OpticalZoom {
		velocity += fmt.Sprintf(`<tt:Zoom x="%.4f"/>`, z)
	}
	return c.command(ctx, c.ptzEndpoint, ptzNS, "ContinuousMove", c.profileParam()+`<tds:Velocity xmlns:tt="http://www.onvif.org/ver10/schema">`+velocity+`</tds:Velocity><tds:Timeout>PT0.5S</tds:Timeout>`)
}
func finiteUnit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= -1 && v <= 1 }
func (c *Control) Stop(ctx context.Context) error {
	var result error
	if c.ptzEndpoint != "" {
		result = c.command(ctx, c.ptzEndpoint, ptzNS, "Stop", c.profileParam()+"<tds:PanTilt>true</tds:PanTilt><tds:Zoom>true</tds:Zoom>")
	}
	if c.imagingEndpoint != "" && c.caps.RelativeFocus {
		e := c.command(ctx, c.imagingEndpoint, imagingNS, "Stop", c.sourceParam())
		if result == nil {
			result = e
		}
	}
	return result
}
func (c *Control) Presets(ctx context.Context) ([]Preset, error) {
	if c.ptzEndpoint == "" {
		return nil, ErrUnsupported
	}
	data, e := c.client.soap(ctx, c.ptzEndpoint, ptzNS, "GetPresets", c.profileParam(), c.secret, 0)
	if e != nil {
		return nil, e
	}
	defer clear(data)
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return nil, e
	}
	nodes := root.All("Preset")
	if len(nodes) > 256 {
		return nil, errors.New("too many PTZ presets")
	}
	out := []Preset{}
	tokens := map[string]string{}
	for _, n := range nodes {
		token := n.Attribute("token")
		if token == "" || len(token) > 512 {
			return nil, errors.New("invalid preset token")
		}
		id := refID(token)
		if _, ok := tokens[id]; ok {
			return nil, errors.New("duplicate preset token")
		}
		tokens[id] = token
		out = append(out, Preset{id, sanitize.Label(n.Value("Name"))})
	}
	c.presetTokens = tokens
	return out, nil
}
func (c *Control) preset(ctx context.Context, id string) (string, error) {
	if !c.caps.Presets {
		return "", ErrUnsupported
	}
	if _, e := c.Presets(ctx); e != nil {
		return "", e
	}
	token := c.presetTokens[id]
	if token == "" {
		return "", errors.New("preset changed or was removed; refresh the list")
	}
	return token, nil
}
func (c *Control) Recall(ctx context.Context, id string) error {
	token, e := c.preset(ctx, id)
	if e != nil {
		return e
	}
	if e = c.verify(ctx); e != nil {
		return e
	}
	return c.command(ctx, c.ptzEndpoint, ptzNS, "GotoPreset", c.profileParam()+"<tds:PresetToken>"+xmlText(token)+"</tds:PresetToken>")
}
func (c *Control) AddPreset(ctx context.Context, name string) error {
	if !c.caps.Presets || strings.TrimSpace(name) == "" || len(name) > 64 {
		return errors.New("enter a preset name up to 64 bytes on a preset-capable profile")
	}
	if e := c.verify(ctx); e != nil {
		return e
	}
	return c.command(ctx, c.ptzEndpoint, ptzNS, "SetPreset", c.profileParam()+"<tds:PresetName>"+xmlText(name)+"</tds:PresetName>")
}
func (c *Control) DeletePreset(ctx context.Context, id string) error {
	token, e := c.preset(ctx, id)
	if e != nil {
		return e
	}
	if e = c.verify(ctx); e != nil {
		return e
	}
	return c.command(ctx, c.ptzEndpoint, ptzNS, "RemovePreset", c.profileParam()+"<tds:PresetToken>"+xmlText(token)+"</tds:PresetToken>")
}
func (c *Control) Focus(ctx context.Context, mode string, direction float64) error {
	if !finiteUnit(direction) {
		return errors.New("invalid focus/iris step")
	}
	if e := c.verify(ctx); e != nil {
		return e
	}
	switch mode {
	case "relative":
		if !c.caps.RelativeFocus {
			return ErrUnsupported
		}
		distance := math.Min(c.focusMax, math.Max(c.focusMin, direction*0.05))
		return c.command(ctx, c.imagingEndpoint, imagingNS, "Move", c.sourceParam()+fmt.Sprintf(`<tds:Focus xmlns:tt="http://www.onvif.org/ver10/schema"><tt:Relative><tt:Distance>%.5f</tt:Distance></tt:Relative></tds:Focus>`, distance))
	case "auto":
		if !c.caps.AutoFocus {
			return ErrUnsupported
		}
		return c.command(ctx, c.imagingEndpoint, imagingNS, "SetImagingSettings", c.sourceParam()+`<tds:ImagingSettings xmlns:tt="http://www.onvif.org/ver10/schema"><tt:Focus><tt:AutoFocusMode>AUTO</tt:AutoFocusMode></tt:Focus></tds:ImagingSettings><tds:ForcePersistence>false</tds:ForcePersistence>`)
	case "iris":
		if !c.caps.Iris {
			return ErrUnsupported
		}
		data, e := c.client.soap(ctx, c.imagingEndpoint, imagingNS, "GetImagingSettings", c.sourceParam(), c.secret, 0)
		if e != nil {
			return e
		}
		defer clear(data)
		root, e := xmlsafe.Parse(data)
		if e != nil {
			return e
		}
		exposures := root.All("Exposure")
		if len(exposures) != 1 || exposures[0].Value("Mode") != "MANUAL" {
			return errors.New("iris control requires an already manual exposure mode; no automatic mode change was made")
		}
		current, e := strconv.ParseFloat(exposures[0].Value("Iris"), 64)
		if e != nil || math.IsNaN(current) || math.IsInf(current, 0) {
			return errors.New("current iris value unavailable")
		}
		next := math.Min(c.irisMax, math.Max(c.irisMin, current+direction*(c.irisMax-c.irisMin)*0.02))
		return c.command(ctx, c.imagingEndpoint, imagingNS, "SetImagingSettings", c.sourceParam()+fmt.Sprintf(`<tds:ImagingSettings xmlns:tt="http://www.onvif.org/ver10/schema"><tt:Exposure><tt:Mode>MANUAL</tt:Mode><tt:Iris>%.5f</tt:Iris></tt:Exposure></tds:ImagingSettings><tds:ForcePersistence>false</tds:ForcePersistence>`, next))
	}
	return ErrUnsupported
}
func (c *Control) Clear() { c.secret.Clear() }
