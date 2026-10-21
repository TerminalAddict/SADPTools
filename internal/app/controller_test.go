package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
)

type blockingScanner struct {
	started chan struct{}
	calls   atomic.Int32
}

func (s *blockingScanner) Scan(ctx context.Context, _ discovery.Options, onDevice func(devices.Device)) (discovery.Result, error) {
	s.calls.Add(1)
	onDevice(devices.Device{MAC: "02:00:00:00:00:01", Model: "SYNTHETIC"})
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return discovery.Result{Devices: []devices.Device{}, Warnings: []string{}}, ctx.Err()
}
func waitFinished(t *testing.T, c *Controller) {
	t.Helper()
	limit := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if !c.Snapshot().Scanning {
			return
		}
		select {
		case <-ticker.C:
		case <-limit:
			t.Fatal("scan did not finish")
		}
	}
}
func TestControllerCancelRetainsInventoryAndCloseWaits(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	defer c.Close()
	scanner := &blockingScanner{started: make(chan struct{}, 2)}
	c.scanner = scanner
	if err := c.StartScan(); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	if err := c.StartScan(); !errors.Is(err, discovery.ErrBusy) {
		t.Fatal(err)
	}
	c.CancelScan()
	waitFinished(t, c)
	if len(c.Snapshot().Devices) != 1 || c.Snapshot().LastError != "" {
		t.Fatal(c.Snapshot())
	}
	if err := c.StartScan(); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	c.Close()
	if c.Snapshot().Scanning || scanner.calls.Load() != 2 {
		t.Fatal("close did not join scan")
	}
	if err := c.StartScan(); err == nil {
		t.Fatal("scan allowed after shutdown")
	}
}
func TestAutomaticDiscoveryDoesNotOverlap(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	defer c.Close()
	scanner := &blockingScanner{started: make(chan struct{}, 2)}
	c.scanner = scanner
	c.mu.Lock()
	c.settings.Automatic = true
	c.nextScan = time.Now().Add(-time.Second)
	c.mu.Unlock()
	select {
	case <-scanner.started:
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("automatic scan did not start")
	}
	// Force the schedule due while a scan is active; a second tick must not overlap.
	time.Sleep(1100 * time.Millisecond)
	if scanner.calls.Load() != 1 {
		t.Fatal("overlapping automatic scans")
	}
	c.CancelScan()
	waitFinished(t, c)
	c.mu.Lock()
	next := c.nextScan
	c.mu.Unlock()
	if !next.After(time.Now()) {
		t.Fatal("interval not scheduled from completion")
	}
}
func TestSettingsRoundTripAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	s := DefaultSettings()
	s.Theme = "light"
	s.Interface = "Ethernet 2"
	s.Automatic = true
	if err := saveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(path)
	if err != nil || got != s {
		t.Fatal(got, err)
	}
	invalid := s
	invalid.TimeoutSeconds = 0
	if err := saveSettings(path, invalid); err == nil {
		t.Fatal("invalid timeout saved")
	}
	got, _ = loadSettings(path)
	if got != s {
		t.Fatal("bad save damaged settings")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = loadSettings(path)
	if err == nil || got != DefaultSettings() {
		t.Fatal("bad settings not recovered")
	}
}
func TestBoundedLogsAndExport(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	defer c.Close()
	for n := 0; n < 600; n++ {
		c.log.Debug("synthetic log", "n", n)
	}
	if len(c.Snapshot().Logs) != 500 {
		t.Fatal("unbounded logs")
	}
	d, _ := c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", Name: " =HYPERLINK(\"test\")", Serial: "synthetic,with,commas"}, time.Now())
	data, err := c.Export([]string{d.ID}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1][4] != "synthetic,with,commas" || !strings.HasPrefix(records[1][11], "'") {
		t.Fatal(records)
	}
	data, err = c.Export([]string{d.ID}, "json")
	var list []devices.Device
	if err != nil || json.Unmarshal(data, &list) != nil || len(list) != 1 {
		t.Fatal(string(data), err)
	}
	if _, err = c.Export([]string{"unknown"}, "json"); err == nil {
		t.Fatal("unknown export ID accepted")
	}
	if _, err = c.Export(nil, "exe"); err == nil {
		t.Fatal("unknown export format accepted")
	}
	if _, err = c.DeviceURL(d.ID); err == nil {
		t.Fatal("guessed web URL")
	}
}
