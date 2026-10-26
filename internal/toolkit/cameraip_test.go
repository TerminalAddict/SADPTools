package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/cameraip"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"strings"
	"testing"
	"time"
)

func fixtureIPRequest(site string) CameraIPRequest {
	return CameraIPRequest{SiteID: site, Target: cameraip.Options{Address: "198.51.100.10", Method: "auto", TimeoutSeconds: 5}}
}
func finishCameraJob(t *testing.T, m *Manager) {
	t.Helper()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("job did not join")
	}
}
func TestCameraIPReviewSaveIsSingleUseAndSiteScoped(t *testing.T) {
	m, site, _ := advancedFixture(t)
	other, e := m.Store.SaveSite(sites.Site{Name: "SYNTHETIC other"})
	if e != nil {
		t.Fatal(e)
	}
	m.cameraIPProbe = func(_ context.Context, o cameraip.Options, s vault.Secret, progress func(string)) (cameraip.Result, error) {
		progress("Synthetic check")
		return cameraip.Result{Identified: true, Device: devices.Device{IPv4: o.Address, Serial: "SYNTHETIC-REMOTE", Model: "SYNTHETIC", Protocols: []string{"direct-ip", "isapi"}}}, nil
	}
	c, e := m.CheckCameraIP(context.Background(), fixtureIPRequest(site.ID), vault.Secret{Username: "fixture", Password: "SYNTHETIC-ONE-TIME"})
	if e != nil || c.Token == "" {
		t.Fatal(c, e)
	}
	finishCameraJob(t, m)
	rows, _ := m.Store.Records(site.ID)
	if len(rows) != 1 {
		t.Fatal("check saved before review")
	}
	for _, bad := range []CameraIPSave{{Token: c.Token, SiteID: other.ID}, {Token: "wrong", SiteID: site.ID}} {
		if _, e = m.SaveCameraIP(bad); e == nil {
			t.Fatal("wrong token/site accepted")
		}
	}
	saved, e := m.SaveCameraIP(CameraIPSave{Token: c.Token, SiteID: site.ID, Name: "Saved SYNTHETIC camera", Location: "Remote test fixture"})
	if e != nil || saved.Record.CredentialMode != "none" {
		t.Fatal(saved, e)
	}
	if _, e = m.SaveCameraIP(CameraIPSave{Token: c.Token, SiteID: site.ID}); e == nil {
		t.Fatal("token replay accepted")
	}
	v, _ := m.Store.View(site.ID)
	history, _ := m.Store.History(site.ID, saved.Record.ID)
	data, _ := json.Marshal([]any{v, history, c, m.Job()})
	if len(v.Profiles) != 0 || strings.Contains(string(data), "SYNTHETIC-ONE-TIME") {
		t.Fatal("one-time secret persisted")
	}
	if len(m.Controller.Snapshot().Devices) != 0 {
		t.Fatal("direct camera merged into local broadcast discovery")
	}
}
func TestCameraIPUnverifiedAndExpiredResult(t *testing.T) {
	m, site, _ := advancedFixture(t)
	m.cameraIPProbe = func(_ context.Context, o cameraip.Options, _ vault.Secret, _ func(string)) (cameraip.Result, error) {
		return cameraip.Result{Device: devices.Device{IPv4: o.Address, Protocols: []string{"direct-ip"}}}, nil
	}
	c, e := m.CheckCameraIP(context.Background(), fixtureIPRequest(site.ID), vault.Secret{})
	if e != nil {
		t.Fatal(e)
	}
	finishCameraJob(t, m)
	save := CameraIPSave{Token: c.Token, SiteID: site.ID}
	if _, e = m.SaveCameraIP(save); e == nil {
		t.Fatal("unverified address saved without explicit consent")
	}
	save.AllowUnverified = true
	m.mu.Lock()
	m.cameraIP.Expires = time.Now().Add(-time.Second)
	m.mu.Unlock()
	if _, e = m.SaveCameraIP(save); e == nil {
		t.Fatal("expired check accepted")
	}
	c, e = m.CheckCameraIP(context.Background(), fixtureIPRequest(site.ID), vault.Secret{})
	if e != nil {
		t.Fatal(e)
	}
	finishCameraJob(t, m)
	old := save
	save.Token = c.Token
	if _, e = m.SaveCameraIP(old); e == nil {
		t.Fatal("old result reused after recheck")
	}
	saved, e := m.SaveCameraIP(save)
	if e != nil || saved.Record.Device.ID != "" || saved.Record.Device.MAC != "" {
		t.Fatal(saved, e)
	}
}
func TestCameraIPSavedProfileAndCancellation(t *testing.T) {
	m, site, _ := advancedFixture(t)
	if e := m.Store.Unlock("Synthetic fixture passphrase"); e != nil {
		t.Fatal(e)
	}
	p, e := m.Store.SaveProfile(sites.Profile{SiteID: site.ID, Label: "SYNTHETIC profile"}, vault.Secret{Username: "fixture", Password: "SYNTHETIC-SAVED"})
	if e != nil {
		t.Fatal(e)
	}
	req := fixtureIPRequest(site.ID)
	req.CredentialID = p.ID
	started := make(chan struct{})
	joined := make(chan struct{})
	m.cameraIPProbe = func(ctx context.Context, _ cameraip.Options, s vault.Secret, _ func(string)) (cameraip.Result, error) {
		if s.Username != "fixture" || s.Password != "SYNTHETIC-SAVED" {
			t.Error("wrong credential")
		}
		close(started)
		<-ctx.Done()
		close(joined)
		return cameraip.Result{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.CheckCameraIP(ctx, req, vault.Secret{}); done <- err }()
	<-started
	cancel()
	select {
	case e = <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("binding context did not cancel check")
	}
	<-joined
	finishCameraJob(t, m)
	if m.Job().Running || !m.Job().Cancelled || m.Controller.Snapshot().Configuring {
		t.Fatal("cancelled task not released", m.Job())
	}
	if _, e = m.SaveCameraIP(CameraIPSave{SiteID: site.ID, AllowUnverified: true}); e == nil {
		t.Fatal("cancelled check saved")
	}
	if _, e = m.CheckCameraIP(context.Background(), req, vault.Secret{Username: "mix", Password: "not allowed"}); e == nil {
		t.Fatal("mixed credential sources accepted")
	}
	m.cameraIPProbe = func(_ context.Context, o cameraip.Options, _ vault.Secret, _ func(string)) (cameraip.Result, error) {
		return cameraip.Result{Identified: true, Device: devices.Device{IPv4: o.Address, Model: "SYNTHETIC", Serial: "SYNTHETIC-PROFILE", Protocols: []string{"direct-ip"}}}, nil
	}
	c, e := m.CheckCameraIP(context.Background(), req, vault.Secret{})
	if e != nil {
		t.Fatal(e)
	}
	finishCameraJob(t, m)
	saved, e := m.SaveCameraIP(CameraIPSave{Token: c.Token, SiteID: site.ID})
	if e != nil || saved.Record.CredentialID != p.ID {
		t.Fatal("selected profile was not assigned", e)
	}
}
func TestCameraIPCloseCancelsPendingCheck(t *testing.T) {
	m, site, _ := advancedFixture(t)
	started := make(chan struct{})
	m.cameraIPProbe = func(ctx context.Context, _ cameraip.Options, _ vault.Secret, _ func(string)) (cameraip.Result, error) {
		close(started)
		<-ctx.Done()
		return cameraip.Result{}, ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, e := m.CheckCameraIP(context.Background(), fixtureIPRequest(site.ID), vault.Secret{})
		done <- e
	}()
	<-started
	m.Close()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not release check")
	}
}
