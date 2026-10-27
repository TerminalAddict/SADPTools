package toolkit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

func TestPreviewProfileSelection(t *testing.T) {
	p := []onvif.MediaProfile{{ID: "unknown", Codec: "H264"}, {ID: "jpeg", Codec: "JPEG", Width: 20, Height: 20}, {ID: "main", Codec: "H265", Width: 1920, Height: 1080}, {ID: "sub", Codec: "H264", Width: 640, Height: 360}}
	if choosePreviewProfile(p, "sub", false) != "sub" || choosePreviewProfile(p, "main", false) != "main" || choosePreviewProfile(p, "sub", true) != "jpeg" {
		t.Fatal("profile choice")
	}
}

func TestImportedCameraPreviewUsesSavedEndpointsAndCredentials(t *testing.T) {
	camera, err := testcamera.New(false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer camera.Close()
	dir := t.TempDir()
	controller := app.New(filepath.Join(dir, "settings.json"), nil)
	defer controller.Close()
	m, err := New(filepath.Join(dir, "sites.sqlite"), controller, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Store.Unlock("Synthetic preview passphrase"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"summary": map[string]any{"passwords_revealed": true}, "cameras": []any{map[string]any{"ip": "127.0.0.1", "vendor": "GENERIC_RTSP", "username": "fixture", "password": "synthetic-password", "has_password": true, "credential_status": "recovered", "stream_urls": map[string]string{"1": camera.URL + "?main", "2": camera.URL + "?sub"}}}})
	p, err := sites.PrepareImport(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Clear()
	site, err := m.Store.ImportSite(p, "Synthetic imported camera")
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := m.Store.Records(site.ID)
	detail, err := m.PreviewDevice(site.ID, rows[0].ID)
	if err != nil || !detail.SavedPreview || len(detail.Credentials) != 1 {
		t.Fatal("imported Live View selection", err)
	}
	j, _ := json.Marshal(detail)
	if strings.Contains(string(j), "rtsp://") || strings.Contains(string(j), "synthetic-password") {
		t.Fatal("private data crossed desktop boundary")
	}
	player := streaming.NewManager(nil)
	defer player.Close()
	o := PreviewOptions{SiteID: site.ID, DeviceID: rows[0].ID, Method: "saved", Channel: 1, Profile: "sub", CredentialSite: site.ID, CredentialRecord: rows[0].ID}
	ticket, err := m.PreparePreview(player, o, vault.Secret{}, vault.Secret{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	packets := 0
	_ = player.Consume(ctx, ticket.ID, func(p streaming.Packet) error {
		if p.Kind == 1 {
			packets++
			if packets >= 3 {
				cancel()
			}
		}
		return nil
	})
	status := player.Status(ticket.ID)
	if packets < 3 || len(status.Profiles) != 2 || status.Info.Codec != "H.264" {
		t.Fatalf("imported stream did not play: %s (%d packets, %d profiles, codec %q)", status.Message, packets, len(status.Profiles), status.Info.Codec)
	}
	m.Store.Lock()
	// Protected endpoints require the vault even when using session credentials.
	o.CredentialSite, o.CredentialRecord = "", ""
	if _, err = m.PreparePreview(player, o, vault.Secret{}, vault.Secret{}); err == nil {
		t.Fatal("locked stream addresses used")
	}
}

func TestPreviewIdentityRejectsContradictions(t *testing.T) {
	a := devices.Device{MAC: "02:00:00:00:00:01", Serial: "SYNTHETIC-1"}
	for _, b := range []devices.Device{{MAC: a.MAC, Serial: "OTHER"}, {MAC: "02:00:00:00:00:02", Serial: a.Serial}, {IdentityConflict: true, MAC: a.MAC}, {MAC: "invalid"}} {
		if samePreviewDevice(a, b) {
			t.Fatal("contradictory identity accepted")
		}
	}
	if !samePreviewDevice(a, devices.Device{MAC: a.MAC}) {
		t.Fatal("valid stable identity rejected")
	}
}

type previewEngine struct{ entered chan struct{} }

func (e previewEngine) Play(ctx context.Context, _ streaming.Source, _ streaming.Sink) error {
	close(e.entered)
	<-ctx.Done()
	return ctx.Err()
}
func TestPreviewVaultAssignmentLockAndTaskIndependence(t *testing.T) {
	dir := t.TempDir()
	controller := app.New(filepath.Join(dir, "settings.json"), nil)
	defer controller.Close()
	m, e := New(filepath.Join(dir, "sites.sqlite"), controller, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	site, e := m.Store.SaveSite(sites.Site{Name: "SYNTHETIC", Networks: []string{"192.0.2.0/24"}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = m.Store.SaveObserved(site.ID, []devices.Device{{MAC: "02:00:00:00:00:01", Serial: "SYNTHETIC-1", IPv4: "192.0.2.3"}})
	if e != nil {
		t.Fatal(e)
	}
	rows, _ := m.Store.Records(site.ID)
	record := rows[0]
	if e = m.Store.Unlock("synthetic-vault-test-passphrase"); e != nil {
		t.Fatal(e)
	}
	profile, e := m.Store.SaveProfile(sites.Profile{SiteID: site.ID, Label: "synthetic"}, vault.Secret{Username: "fixture", Password: "synthetic-password"})
	if e != nil {
		t.Fatal(e)
	}
	site.DefaultCredential = profile.ID
	if _, e = m.Store.SaveSite(site); e != nil {
		t.Fatal(e)
	}
	detail, e := m.PreviewDevice(site.ID, record.ID)
	if e != nil || len(detail.Credentials) != 1 {
		t.Fatal("saved credential selection", e)
	}
	engine := previewEngine{entered: make(chan struct{})}
	player := streaming.NewManager(engine)
	defer player.Close()
	options := PreviewOptions{SiteID: site.ID, DeviceID: record.ID, Method: "hikvision", Channel: 1, Profile: "sub", CredentialSite: site.ID, CredentialRecord: record.ID}
	ticket, e := m.PreparePreview(player, options, vault.Secret{}, vault.Secret{})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() {
		_ = player.Consume(context.Background(), ticket.ID, func(streaming.Packet) error { return nil })
		close(done)
	}()
	select {
	case <-engine.entered:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}
	if controller.Snapshot().Configuring || m.Job().Running {
		t.Fatal("preview blocked discovery/configuration")
	}
	m.Store.Lock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("vault lock left stream running")
	}
	if _, e = m.PreparePreview(player, options, vault.Secret{}, vault.Secret{}); e == nil {
		t.Fatal("locked vault accepted")
	}
	options.CredentialRecord = "wrong-record"
	if _, e = m.PreparePreview(player, options, vault.Secret{}, vault.Secret{}); e == nil {
		t.Fatal("wrong credential record accepted")
	}
}
