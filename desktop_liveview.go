//go:build desktop

package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type LiveViewService struct {
	manager *streaming.Manager
	kit     *toolkit.Manager
}

func (s *LiveViewService) Device(site, id string) (toolkit.PreviewDevice, error) {
	if s.kit == nil {
		return toolkit.PreviewDevice{}, errors.New("device storage is unavailable")
	}
	return s.kit.PreviewDevice(site, id)
}
func (s *LiveViewService) Start(o toolkit.PreviewOptions, username, password, onvifUsername, onvifPassword string) (streaming.Ticket, error) {
	if s.kit == nil {
		return streaming.Ticket{}, errors.New("device storage is unavailable")
	}
	secret := vault.Secret{Username: username, Password: password}
	other := vault.Secret{Username: onvifUsername, Password: onvifPassword}
	defer secret.Clear()
	defer other.Clear()
	return s.kit.PreparePreview(s.manager, o, secret, other)
}
func (s *LiveViewService) Stop(id string)                    { s.manager.Stop(id) }
func (s *LiveViewService) Status(id string) streaming.Status { return s.manager.Status(id) }
func (s *LiveViewService) Certificate(site, id, raw string, onvif bool) (streaming.Certificate, error) {
	if s.kit == nil {
		return streaming.Certificate{}, errors.New("device storage is unavailable")
	}
	return s.kit.PreviewCertificate(site, id, raw, onvif)
}
func (s *LiveViewService) ServiceShutdown() error { s.manager.Close(); return nil }

// No listening socket: media rides the same origin-bound Wails channel as the
// application's IPC. A random, single-use capability is required before data.
func (s *LiveViewService) handle(conn *application.StreamConn) {
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
	defer conn.Close()
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
