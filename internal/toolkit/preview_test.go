package toolkit

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

func TestPreviewProfileSelection(t *testing.T) {
	p := []onvif.MediaProfile{{ID: "unknown", Codec: "H264"}, {ID: "jpeg", Codec: "JPEG", Width: 20, Height: 20}, {ID: "main", Codec: "H265", Width: 1920, Height: 1080}, {ID: "sub", Codec: "H264", Width: 640, Height: 360}}
	if choosePreviewProfile(p, "sub", false) != "sub" || choosePreviewProfile(p, "main", false) != "main" || choosePreviewProfile(p, "sub", true) != "jpeg" {
		t.Fatal("profile choice")
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
