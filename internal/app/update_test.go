package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateSettingsMigrateOldFilesAndPreserveOptOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"light","timeoutSeconds":5,"intervalSeconds":30}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := loadSettings(path)
	if err != nil || !s.CheckUpdates || s.UpdateChannel == "" {
		t.Fatalf("%+v %v", s, err)
	}
	s.CheckUpdates = false
	s.UpdateChannel = "stable"
	if err := saveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(path)
	if err != nil || got.CheckUpdates || got.UpdateChannel != "stable" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestUpdateNeverInterruptsDeviceOperation(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	defer c.Close()
	c.mu.Lock()
	c.configuring = true
	c.mu.Unlock()
	called := false
	err := c.LaunchUpdate(func(Settings) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("update launched during a device operation")
	}
}

func TestFailedUpdateLaunchLeavesAppUsable(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	defer c.Close()
	if err := c.LaunchUpdate(func(Settings) error { return errors.New("synthetic failure") }); err == nil {
		t.Fatal("missing error")
	}
	if c.closed {
		t.Fatal("failed launch closed controller")
	}
	if err := c.SaveSettings(DefaultSettings()); err != nil {
		t.Fatal(err)
	}
}

func TestSuccessfulUpdateStopsDiscoveryAndPreventsNewWork(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	defer c.Close()
	scanner := &blockingScanner{started: make(chan struct{}, 1)}
	c.scanner = scanner
	if err := c.StartScan(); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	if err := c.LaunchUpdate(func(settings Settings) error {
		if settings.UpdateChannel == "" {
			t.Fatal("missing settings")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Scanning {
		t.Fatal("scan survived update handoff")
	}
	if err := c.StartScan(); err == nil {
		t.Fatal("new work accepted after handoff")
	}
}
