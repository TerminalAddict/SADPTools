package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type webDetectFunc func(context.Context, webinterface.Target) (webinterface.Result, error)

func (f webDetectFunc) Detect(ctx context.Context, t webinterface.Target) (webinterface.Result, error) {
	return f(ctx, t)
}

func webController(t *testing.T) (*Controller, string) {
	t.Helper()
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	t.Cleanup(c.Close)
	port := 80
	d, _ := c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", IPv4: "192.0.2.40", HTTPPort: &port}, time.Now())
	return c, d.ID
}
func syntheticEndpoint(address string) webinterface.Result {
	return webinterface.Result{Endpoint: webinterface.Endpoint{URL: "https://" + address + "/", Scheme: "https", Port: 443}}
}

func TestDeviceURLUsesDetectedEndpointAndInvalidatesCache(t *testing.T) {
	c, id := webController(t)
	calls := 0
	c.webDetector = webDetectFunc(func(_ context.Context, target webinterface.Target) (webinterface.Result, error) {
		calls++
		return syntheticEndpoint(target.Address), nil
	})
	for range 2 {
		url, err := c.DeviceURL(id)
		if err != nil || url != "https://192.0.2.40/" {
			t.Fatal(url, err)
		}
	}
	if calls != 1 {
		t.Fatal("browser ignored cached detection")
	}
	c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", IPv4: "192.0.2.41"}, time.Now())
	url, err := c.DeviceURL(id)
	if err != nil || url != "https://192.0.2.41/" || calls != 2 {
		t.Fatal(url, err, calls)
	}
	if _, err := c.CheckDeviceWeb(id, true); err != nil || calls != 3 {
		t.Fatal("forced recheck did not run", err)
	}
	c.webDetector = webDetectFunc(func(context.Context, webinterface.Target) (webinterface.Result, error) {
		return syntheticEndpoint("198.51.100.99"), nil
	})
	if _, err := c.CheckDeviceWeb(id, true); err == nil {
		t.Fatal("foreign address accepted")
	}
}

func TestWebProbeCancellationAndStaleAddress(t *testing.T) {
	for _, change := range []string{"address", "selection", "shutdown"} {
		t.Run(change, func(t *testing.T) {
			c, id := webController(t)
			started, release := make(chan struct{}), make(chan struct{})
			c.webDetector = webDetectFunc(func(ctx context.Context, target webinterface.Target) (webinterface.Result, error) {
				if target.Address == "192.0.2.40" {
					close(started)
					if change == "address" {
						<-release
					} else {
						<-ctx.Done()
					}
				}
				// Deliberately return a late success to exercise controller guards.
				return syntheticEndpoint(target.Address), nil
			})
			done := make(chan error, 1)
			go func() { _, err := c.CheckDeviceWeb(id, false); done <- err }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("probe not started")
			}
			switch change {
			case "address":
				c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", IPv4: "192.0.2.41"}, time.Now())
				close(release)
			case "selection":
				d, _ := c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:02", IPv4: "192.0.2.42"}, time.Now())
				if _, err := c.CheckDeviceWeb(d.ID, false); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				c.Close()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("stale success accepted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("probe was not joined")
			}
			c.mu.Lock()
			_, cached := c.webResults[id]
			c.mu.Unlock()
			if cached {
				t.Fatal("stale result cached")
			}
		})
	}
}

type scanFixture func(context.Context, discovery.Options, func(devices.Device)) (discovery.Result, error)

func (f scanFixture) Scan(ctx context.Context, o discovery.Options, fn func(devices.Device)) (discovery.Result, error) {
	return f(ctx, o, fn)
}

func TestScanFeedbackCountsCurrentUniqueDevicesAndRetainsInventory(t *testing.T) {
	c, _ := webController(t) // One previously discovered camera remains visible.
	for _, failure := range []error{nil, context.Canceled, errors.New("synthetic bind failure")} {
		started, release := make(chan struct{}), make(chan struct{})
		c.scanner = scanFixture(func(_ context.Context, _ discovery.Options, fn func(devices.Device)) (discovery.Result, error) {
			fn(devices.Device{MAC: "02:00:00:00:00:02", Model: "SYNTHETIC"})
			fn(devices.Device{MAC: "02:00:00:00:00:02", Model: "SYNTHETIC"})
			close(started)
			<-release
			return discovery.Result{}, failure
		})
		if err := c.StartScan(); err != nil {
			t.Fatal(err)
		}
		<-started
		s := c.Snapshot()
		if !s.Scanning || s.ScanStarted == "" || s.ScanSeconds != c.settings.TimeoutSeconds || s.ScanSeen != 1 || len(s.Devices) != 2 {
			t.Fatal(s)
		}
		wantNew := 0
		if failure == nil {
			wantNew = 1
		}
		if s.ScanNew != wantNew {
			t.Fatal("duplicate or retained camera counted as new", s.ScanNew)
		}
		close(release)
		waitFinished(t, c)
		wantOutcome := "completed"
		if failure == context.Canceled {
			wantOutcome = "cancelled"
		} else if failure != nil {
			wantOutcome = "failed"
		}
		if c.Snapshot().ScanOutcome != wantOutcome {
			t.Fatal(c.Snapshot())
		}
	}
}
