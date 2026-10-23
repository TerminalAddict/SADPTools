package toolkit

import (
	"context"
	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"path/filepath"
	"testing"
	"time"
)

func TestSharedCancellationAndShutdown(t *testing.T) {
	dir := t.TempDir()
	controller := app.New(filepath.Join(dir, "settings.json"), nil)
	defer controller.Close()
	m, e := New(filepath.Join(dir, "sites.sqlite"), controller, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	started := make(chan struct{})
	if e = m.start("synthetic task", "", func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }); e != nil {
		t.Fatal(e)
	}
	<-started
	if !controller.Snapshot().Configuring || !m.Job().Running {
		t.Fatal("task exclusion not shared")
	}
	if e = m.start("duplicate", "", func(context.Context) error { return nil }); e == nil {
		t.Fatal("concurrent task allowed")
	}
	m.Cancel()
	finished := make(chan struct{})
	go func() { m.Close(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel/join")
	}
	if controller.Snapshot().Configuring || m.Job().Running || !m.Job().Cancelled {
		t.Fatal("task state not released", m.Job())
	}
}
func TestSiteScopeRequiresSavedAdapterAndNetworks(t *testing.T) {
	a := platform.Adapter{Name: "Synthetic Ethernet", Addresses: []string{"192.0.2.2/24"}, Suitable: true}
	site := sites.Site{ID: "synthetic", Name: "Synthetic site", Interface: a.Name, Networks: []string{"192.0.2.0/24"}, Protocols: []string{"sadp", "onvif"}}
	scope, e := PrepareSiteScope(site, []platform.Adapter{a}, 5)
	if e != nil || scope.Token == "" {
		t.Fatal(e)
	}
	a.Addresses = []string{"198.51.100.2/24"}
	changed, e := PrepareSiteScope(site, []platform.Adapter{a}, 5)
	if e != nil || changed.Token == scope.Token {
		t.Fatal("adapter change did not invalidate approval")
	}
	site.Interface = ""
	if _, e = PrepareSiteScope(site, []platform.Adapter{a}, 5); e == nil {
		t.Fatal("all-interface site scan implicitly allowed")
	}
}
