//go:build desktop

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type liveViewWindow struct {
	window               *application.WebviewWindow
	windowID             uint
	site, device, ticket string
	secret, onvifSecret  vault.Secret
	options              toolkit.PreviewOptions
	controls             *viewerControl
	streamID             string
}

func (v *liveViewWindow) clear() { v.secret.Clear(); v.onvifSecret.Clear() }

type PreviewWindow struct {
	SiteID   string                `json:"siteID"`
	DeviceID string                `json:"deviceID"`
	Theme    string                `json:"theme"`
	StreamID string                `json:"streamID"`
	Detail   toolkit.PreviewDevice `json:"detail"`
}

type LiveViewService struct {
	manager *streaming.Manager
	kit     *toolkit.Manager
	app     *application.App
	openMu  sync.Mutex
	mu      sync.Mutex
	view    *liveViewWindow
	closed  bool
}

// Open creates a native, independently movable window. Reopening the same
// camera focuses it; choosing Live View on another replaces the single stream.
func (s *LiveViewService) Open(site, id string) error { return s.open(site, id, "") }
func (s *LiveViewService) OpenChannel(site, id, channel, stream string) error {
	value, e := s.kit.ChannelStream(site, id, channel, stream)
	if e != nil {
		return e
	}
	return s.open(site, id, value)
}
func (s *LiveViewService) open(site, id, streamID string) error {
	if s.kit == nil {
		return errors.New("device storage is unavailable")
	}
	detail, e := s.kit.PreviewDevice(site, id)
	if e != nil {
		return e
	}
	s.openMu.Lock()
	defer s.openMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("Live View is closing")
	}
	old := s.view
	if old != nil && old.site == site && old.device == id && old.streamID == streamID {
		window := old.window
		s.mu.Unlock()
		window.Show()
		if window.IsMinimised() {
			window.Restore()
		}
		window.Focus()
		return nil
	}
	s.mu.Unlock()
	if old != nil {
		s.retire(old.windowID)
		old.window.Close()
	}
	name := detail.Device.Name
	if name == "" {
		name = detail.Device.Model
	}
	if name == "" {
		name = "Camera"
	}
	window := application.NewWindow(application.WebviewWindowOptions{
		Title: "Live View — " + name + " · " + detail.Device.IPv4,
		Width: 960, Height: 820, MinWidth: 540, MinHeight: 420, URL: "/live-view.html",
		BackgroundColour: application.NewRGB(17, 27, 40), Linux: application.LinuxWindow{Icon: appIcon},
	})
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		window.Close()
		return errors.New("Live View is closing")
	}
	s.view = &liveViewWindow{window: window, windowID: window.ID(), site: site, device: id, streamID: streamID}
	s.mu.Unlock()
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { s.retire(window.ID()) })
	s.app.Window.Add(window)
	window.Run()
	window.Focus()
	return nil
}

// owner is checked under the mutex. A stale renderer, the main window, or an
// old closing window cannot claim or stop another window's preview.
func (s *LiveViewService) owner(ctx context.Context) (*liveViewWindow, error) {
	window, ok := ctx.Value(application.WindowKey).(application.Window)
	if s.closed || !ok || window == nil || s.view == nil || s.view.windowID != window.ID() {
		return nil, errors.New("this Live View window is no longer active")
	}
	return s.view, nil
}
func (s *LiveViewService) Load(ctx context.Context) (PreviewWindow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.owner(ctx)
	if e != nil {
		return PreviewWindow{}, e
	}
	detail, e := s.kit.PreviewDevice(v.site, v.device)
	return PreviewWindow{SiteID: v.site, DeviceID: v.device, StreamID: v.streamID, Theme: s.kit.Controller.Settings().Theme, Detail: detail}, e
}
func (s *LiveViewService) Start(ctx context.Context, o toolkit.PreviewOptions, username, password, onvifUsername, onvifPassword string, replaceCredentials bool) (streaming.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.owner(ctx)
	if e != nil {
		return streaming.Ticket{}, e
	}
	if o.SiteID != v.site || o.DeviceID != v.device {
		return streaming.Ticket{}, errors.New("select Live View on that camera to change devices")
	}
	if v.controls != nil {
		v.controls.close()
		v.controls = nil
	}
	if v.streamID != "" {
		o.StreamID = v.streamID
		o.Method = "hikvision"
	}
	if replaceCredentials {
		secret := vault.Secret{Username: username, Password: password}
		other := vault.Secret{Username: onvifUsername, Password: onvifPassword}
		if e = streaming.ValidateCredentials(secret); e != nil {
			return streaming.Ticket{}, e
		}
		if e = streaming.ValidateCredentials(other); e != nil {
			return streaming.Ticket{}, e
		}
		v.clear()
		v.secret = secret
		v.onvifSecret = other
	}
	ticket, e := s.kit.PreparePreview(s.manager, o, v.secret, v.onvifSecret)
	if e == nil {
		v.ticket = ticket.ID
		v.options = o
	}
	return ticket, e
}
func (s *LiveViewService) Stop(ctx context.Context, id string) {
	s.mu.Lock()
	v, e := s.owner(ctx)
	if e != nil || id == "" || v.ticket != id {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.StopCamera(ctx)
	s.manager.Stop(id)
}
func (s *LiveViewService) Status(ctx context.Context, id string) streaming.Status {
	s.mu.Lock()
	v, e := s.owner(ctx)
	if e != nil || id == "" || v.ticket != id {
		s.mu.Unlock()
		return streaming.Status{State: "stopped", Message: "Preview window closed"}
	}
	s.mu.Unlock()
	return s.manager.Status(id)
}
func (s *LiveViewService) Certificate(ctx context.Context, raw string, onvif bool) (streaming.Certificate, error) {
	s.mu.Lock()
	v, e := s.owner(ctx)
	if e != nil {
		s.mu.Unlock()
		return streaming.Certificate{}, e
	}
	site, id := v.site, v.device
	s.mu.Unlock()
	return s.kit.PreviewCertificate(site, id, raw, onvif)
}
func (s *LiveViewService) retire(windowID uint) {
	s.mu.Lock()
	v := s.view
	if v == nil || v.windowID != windowID {
		s.mu.Unlock()
		return
	}
	s.view = nil
	v.clear()
	ticket := v.ticket
	controls := v.controls
	v.controls = nil
	s.mu.Unlock()
	controls.close()
	if ticket != "" {
		s.manager.Stop(ticket)
	}
}
func (s *LiveViewService) closeWindow() {
	s.openMu.Lock()
	defer s.openMu.Unlock()
	s.mu.Lock()
	s.closed = true
	v := s.view
	s.mu.Unlock()
	if v != nil {
		s.retire(v.windowID)
		v.window.Close()
	}
}
func (s *LiveViewService) lockCredentials() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view != nil {
		s.view.clear()
		s.view.controls.close()
		s.view.controls = nil
	}
	s.manager.Stop("")
}
func (s *LiveViewService) ServiceShutdown() error {
	s.mu.Lock()
	s.closed = true
	if s.view != nil {
		s.view.clear()
		s.view.controls.close()
		s.view.controls = nil
		s.view = nil
	}
	s.mu.Unlock()
	s.manager.Close()
	return nil
}

// No listening socket: media rides the same origin-bound Wails channel as the
// application's IPC. A random, single-use capability is required before data.
func (s *LiveViewService) handle(conn *application.StreamConn) {
	defer conn.Close()
	timer := time.AfterFunc(5*time.Second, func() { _ = conn.Close() })
	defer timer.Stop()
	raw, e := conn.Receive()
	if e != nil || len(raw) > 256 {
		return
	}
	var hello struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &hello) != nil || len(hello.ID) != 64 {
		return
	}
	timer.Stop()
	s.mu.Lock()
	owned := s.view != nil && conn.Window() != nil && conn.Window().ID() == s.view.windowID && s.view.ticket == hello.ID && !s.closed
	s.mu.Unlock()
	if !owned {
		return
	}
	go func() {
		for {
			b, e := conn.Receive()
			if e != nil {
				return
			}
			if len(b) > 128 || (string(b) != "heartbeat" && string(b) != "playing") {
				_ = conn.Close()
				return
			}
			s.manager.Heartbeat(hello.ID, string(b) == "playing")
		}
	}()
	_ = s.manager.Consume(conn.Context(), hello.ID, func(p streaming.Packet) error {
		// Fragment MP4 byte streams to keep UI scheduling responsive. JPEG needs
		// one whole frame; its 4 MiB limit is below Wails' per-frame limit.
		chunk := len(p.Data)
		if p.Kind == 1 && chunk > 128<<10 {
			chunk = 128 << 10
		}
		for start := 0; start < len(p.Data); start += chunk {
			end := min(start+chunk, len(p.Data))
			frame := make([]byte, 1+end-start)
			frame[0] = p.Kind
			copy(frame[1:], p.Data[start:end])
			if e := conn.TrySend(frame); e != nil {
				return errors.New("player cannot keep up with this stream; select the sub stream or lower resolution")
			}
		}
		return nil
	})
}
