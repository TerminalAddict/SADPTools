//go:build desktop

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type previewTestWindow struct {
	application.Window
	id uint
}

func (w previewTestWindow) ID() uint { return w.id }
func previewContext(id uint) context.Context {
	return context.WithValue(context.Background(), application.WindowKey, previewTestWindow{id: id})
}

func TestPreviewOwnershipCachedLoginAndClose(t *testing.T) {
	dir := t.TempDir()
	controller := app.New(filepath.Join(dir, "settings.json"), nil)
	defer controller.Close()
	kit, e := toolkit.New(filepath.Join(dir, "sites.sqlite"), controller, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer kit.Close()
	site, e := kit.Store.SaveSite(sites.Site{Name: "Synthetic viewer ownership"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = kit.Store.SaveObserved(site.ID, []devices.Device{{MAC: "02:00:00:00:00:71", Serial: "SYNTHETIC-VIEWER", IPv4: "127.0.0.1"}})
	if e != nil {
		t.Fatal(e)
	}
	rows, _ := kit.Store.Records(site.ID)
	manager := streaming.NewManager(nil)
	defer manager.Close()
	service := &LiveViewService{kit: kit, manager: manager, view: &liveViewWindow{windowID: 41, site: site.ID, device: rows[0].ID}}
	camera, e := testcamera.New(false, true, "")
	if e != nil {
		t.Fatal(e)
	}
	defer camera.Close()
	options := toolkit.PreviewOptions{SiteID: site.ID, DeviceID: rows[0].ID, Channel: 1, Method: "rtsp", URL: camera.URL}
	for _, ctx := range []context.Context{context.Background(), previewContext(42)} {
		if _, e = service.Load(ctx); e == nil {
			t.Fatal("another window read viewer state")
		}
		if _, e = service.Start(ctx, options, "", "", "", "", false); e == nil {
			t.Fatal("another window started preview")
		}
	}
	ctx := previewContext(41)
	for i := 0; i < 2; i++ {
		u, p := "", ""
		if i == 0 {
			u, p = "fixture", "synthetic-password"
		}
		ticket, e := service.Start(ctx, options, u, p, "", "", i == 0)
		if e != nil {
			t.Fatal(e)
		}
		service.Stop(previewContext(42), ticket.ID)
		if manager.Status(ticket.ID).State == "stopped" {
			t.Fatal("foreign window stopped preview")
		}
		mediaCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		got := false
		e = manager.Consume(mediaCtx, ticket.ID, func(packet streaming.Packet) error {
			if packet.Kind == 1 {
				got = true
				cancel()
			}
			return nil
		})
		cancel()
		if e != nil || !got {
			t.Fatal("cached camera credentials failed", e, manager.Status(ticket.ID))
		}
	}
	boot, e := service.Load(ctx)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(boot)
	if strings.Contains(string(raw), "synthetic-password") {
		t.Fatal("credentials exposed in viewer bootstrap")
	}
	old := service.view
	service.retire(40)
	if service.view != old {
		t.Fatal("stale close removed another viewer")
	}
	service.lockCredentials()
	if old.secret.Password != "" {
		t.Fatal("manual lock retained viewer credentials")
	}
	if _, e = service.Start(ctx, options, "fixture", "synthetic-password", "", "", true); e != nil {
		t.Fatal(e)
	}
	service.retire(41)
	if service.view != nil || old.secret.Password != "" {
		t.Fatal("closed viewer retained credentials")
	}
}
