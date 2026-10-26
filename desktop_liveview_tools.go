//go:build desktop

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/mediaimage"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/ptz"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"strings"
	"sync"
	"time"
)

type viewerControl struct {
	provider   *onvif.Control
	controller *ptz.Controller
	once       sync.Once
}

func (c *viewerControl) close() {
	if c != nil {
		c.once.Do(func() { c.controller.Close(); c.provider.Clear() })
	}
}
func (s *LiveViewService) LoadControls(ctx context.Context) (onvif.ControlCapabilities, error) {
	s.mu.Lock()
	v, e := s.owner(ctx)
	if e != nil {
		s.mu.Unlock()
		return onvif.ControlCapabilities{}, e
	}
	id := v.windowID
	o, secret, other := v.options, v.secret, v.onvifSecret
	old := v.controls
	v.controls = nil
	s.mu.Unlock()
	old.close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	provider, e := s.kit.PreviewControls(ctx, o, secret, other)
	if e != nil {
		return onvif.ControlCapabilities{}, e
	}
	c := &viewerControl{provider: provider, controller: ptz.New(provider)}
	s.mu.Lock()
	v, e = s.owner(ctx)
	if e != nil || v.windowID != id {
		s.mu.Unlock()
		c.close()
		return onvif.ControlCapabilities{}, errors.New("viewer changed while reading controls")
	}
	old = v.controls
	v.controls = c
	s.mu.Unlock()
	old.close()
	return provider.Capabilities(), nil
}
func (s *LiveViewService) controls(ctx context.Context) (*viewerControl, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.owner(ctx)
	if e != nil {
		return nil, e
	}
	if v.controls == nil {
		return nil, errors.New("read this profile's supported camera controls first")
	}
	if v.options.CredentialSite != "" && !s.kit.Store.CredentialSessionActive() {
		return nil, errors.New("saved credentials are locked")
	}
	return v.controls, nil
}
func (s *LiveViewService) MoveCamera(ctx context.Context, x, y, z float64) error {
	c, e := s.controls(ctx)
	if e != nil {
		return e
	}
	task, finish, e := s.kit.Controller.AcquireTask()
	if e != nil {
		return e
	}
	defer finish()
	stop := context.AfterFunc(ctx, func() { c.controller.Stop() })
	defer stop()
	return c.controller.Pulse(task, x, y, z)
}
func (s *LiveViewService) StopCamera(ctx context.Context) string {
	c, e := s.controls(ctx)
	if e != nil {
		return ""
	}
	c.controller.Stop()
	return c.controller.StopError()
}
func (s *LiveViewService) CameraPresets(ctx context.Context) ([]onvif.Preset, error) {
	c, e := s.controls(ctx)
	if e != nil {
		return nil, e
	}
	var out []onvif.Preset
	e = c.controller.Action(ctx, func(ctx context.Context) error { var e error; out, e = c.provider.Presets(ctx); return e })
	return out, e
}
func (s *LiveViewService) CameraPreset(ctx context.Context, action, id, name string) error {
	c, e := s.controls(ctx)
	if e != nil {
		return e
	}
	task, finish, e := s.kit.Controller.AcquireTask()
	if e != nil {
		return e
	}
	defer finish()
	if action == "recall" {
		return c.controller.Recall(task, id)
	}
	return c.controller.Action(task, func(ctx context.Context) error {
		switch action {
		case "add":
			return c.provider.AddPreset(ctx, name)
		case "delete":
			return c.provider.DeletePreset(ctx, id)
		default:
			return errors.New("preset rename is unavailable: ONVIF SetPreset can overwrite position")
		}
	})
}
func (s *LiveViewService) CameraFocus(ctx context.Context, mode string, direction float64) error {
	c, e := s.controls(ctx)
	if e != nil {
		return e
	}
	task, finish, e := s.kit.Controller.AcquireTask()
	if e != nil {
		return e
	}
	defer finish()
	return c.controller.Action(task, func(ctx context.Context) error { return c.provider.Focus(ctx, mode, direction) })
}

type PlayerMeasurement struct {
	DisplayedFPS  *float64 `json:"displayedFPS"`
	DroppedFrames *uint64  `json:"droppedFrames"`
	TestSeconds   float64  `json:"testSeconds"`
	Label         string   `json:"label"`
}
type StreamDiagnostic struct {
	StreamID string            `json:"streamID"`
	DeviceID string            `json:"deviceID"`
	At       string            `json:"at"`
	Profile  string            `json:"profile"`
	Status   streaming.Status  `json:"status"`
	Player   PlayerMeasurement `json:"player"`
	Notes    string            `json:"notes"`
}

func (s *LiveViewService) SaveDiagnostic(ctx context.Context, id string, p PlayerMeasurement) (sites.Document, error) {
	s.mu.Lock()
	v, e := s.owner(ctx)
	if e != nil || v.ticket != id {
		s.mu.Unlock()
		return sites.Document{}, errors.New("preview session changed")
	}
	site, device, profile, streamID := v.site, v.device, v.options.Profile, v.streamID
	s.mu.Unlock()
	if site == "" {
		return sites.Document{}, errors.New("save this device to a site before saving diagnostic evidence")
	}
	if p.TestSeconds < 0 || p.TestSeconds > 3600 || p.DisplayedFPS != nil && (*p.DisplayedFPS < 0 || *p.DisplayedFPS > 1000) {
		return sites.Document{}, errors.New("invalid player measurement")
	}
	p.Label = sanitize.Label(p.Label)
	status := s.manager.Status(id)
	status.ID = ""
	d := StreamDiagnostic{StreamID: streamID, DeviceID: device, At: time.Now().UTC().Format(time.RFC3339Nano), Profile: profile, Status: status, Player: p, Notes: "RTP bitrate includes RTP headers; displayed frame rate depends on decoder callbacks. Nominal FPS/jitter unavailable. TCP only. No camera settings changed."}
	raw, e := json.Marshal(d)
	if e != nil {
		return sites.Document{}, e
	}
	return s.kit.Store.SaveDocument(sites.Document{SiteID: site, DeviceID: device, Kind: "stream-diagnostic", Title: "Stream diagnostic · " + p.Label, Version: 1, Data: raw})
}
func (s *LiveViewService) SaveSnapshot(ctx context.Context, data, title string) (sites.Asset, error) {
	s.mu.Lock()
	v, e := s.owner(ctx)
	if e != nil {
		s.mu.Unlock()
		return sites.Asset{}, e
	}
	site, device := v.site, v.device
	if v.streamID != "" {
		title += " · NVR stream " + v.streamID
	}
	s.mu.Unlock()
	if site == "" {
		return sites.Asset{}, errors.New("save this device to a site before capturing site evidence")
	}
	if !strings.HasPrefix(data, "data:image/png;base64,") || len(data) > 22<<20 {
		return sites.Asset{}, errors.New("snapshot must be a PNG within 16 MiB")
	}
	raw, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	if e != nil {
		return sites.Asset{}, errors.New("invalid snapshot")
	}
	return s.kit.Store.SaveAsset(sites.Asset{SiteID: site, DeviceID: device, Kind: "snapshot", Title: sanitize.Label(title)}, raw)
}

func (s *LiveViewService) ExportSnapshot(ctx context.Context, data string) (string, error) {
	s.mu.Lock()
	v, e := s.owner(ctx)
	identity := ""
	if e == nil {
		sum := sha256.Sum256([]byte(v.device))
		identity = fmt.Sprintf("camera-%x", sum[:4])
		if v.streamID != "" {
			identity += "-stream-" + v.streamID
		}
	}
	s.mu.Unlock()
	if e != nil {
		return "", e
	}
	if !strings.HasPrefix(data, "data:image/png;base64,") || len(data) > 22<<20 {
		return "", errors.New("snapshot must be a PNG within 16 MiB")
	}
	raw, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	if e != nil {
		return "", errors.New("invalid snapshot")
	}
	raw, _, _, e = mediaimage.Normalise(raw)
	if e != nil {
		return "", e
	}
	service := ToolkitService{app: s.app}
	return service.saveFile("SADPTools-snapshot-"+identity+"-"+time.Now().UTC().Format("20060102T150405Z")+".png", raw)
}
