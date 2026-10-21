package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

type writerFunc func(context.Context, configuration.Request) (configuration.Result, error)

func (f writerFunc) Execute(ctx context.Context, r configuration.Request) (configuration.Result, error) {
	return f(ctx, r)
}
func configController(t *testing.T) (*Controller, string, configuration.Request) {
	t.Helper()
	c := New(filepath.Join(t.TempDir(), "settings.json"), nil)
	t.Cleanup(c.Close)
	inactive := false
	port := 8123
	d, _ := c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", Activated: &inactive, CommandPort: &port, Interfaces: []devices.Interface{{Index: 3, Name: "Ethernet Test"}}}, time.Now())
	r := configuration.Request{Action: sadp.Activate, MAC: d.MAC, Interface: "Ethernet Test", TimeoutSeconds: 2, Password: "Synthetic-pass1"}
	return c, d.ID, r
}
func TestControllerOperationLifecycleAndNoOptimisticUpdate(t *testing.T) {
	c, id, r := configController(t)
	started := make(chan struct{})
	var calls atomic.Int32
	c.writer = writerFunc(func(ctx context.Context, r configuration.Request) (configuration.Result, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return configuration.Result{Action: r.Action, MAC: r.MAC, Sent: true, Outcome: "unconfirmed", Message: "Synthetic unconfirmed operation"}, errors.Join(configuration.ErrUnconfirmed, ctx.Err())
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := c.Configure(id, r)
		if err != nil || result.Outcome != "unconfirmed" {
			t.Error(result, err)
		}
	}()
	<-started
	if !c.Snapshot().Configuring {
		t.Fatal("operation state not visible")
	}
	if err := c.StartScan(); !errors.Is(err, configuration.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := c.Configure(id, r); !errors.Is(err, configuration.ErrBusy) {
		t.Fatal(err)
	}
	c.CancelOperation()
	<-done
	if c.Snapshot().Configuring || calls.Load() != 1 {
		t.Fatal("operation did not stop")
	}
	if *c.Snapshot().Devices[0].Activated {
		t.Fatal("invented activation success")
	}
	data, _ := json.Marshal(c.Snapshot())
	if strings.Contains(string(data), r.Password) {
		t.Fatal("password in snapshot")
	}
}
func TestControllerChecksIdentityAdapterAndActivation(t *testing.T) {
	for _, kind := range []string{"unknown", "mac", "adapter", "already-active", "inactive-network"} {
		t.Run(kind, func(t *testing.T) {
			c, id, r := configController(t)
			c.writer = writerFunc(func(context.Context, configuration.Request) (configuration.Result, error) {
				t.Fatal("invalid command sent")
				return configuration.Result{}, nil
			})
			switch kind {
			case "unknown":
				id = "missing"
			case "mac":
				r.MAC = "02:00:00:00:00:02"
			case "adapter":
				r.Interface = "Other adapter"
			case "already-active":
				active := true
				c.inventory.Upsert(devices.Device{MAC: r.MAC, Activated: &active}, time.Now())
			case "inactive-network":
				r.Action = sadp.UpdateNetwork
			}
			if _, err := c.Configure(id, r); err == nil {
				t.Fatal("bad selection accepted")
			}
		})
	}
}
func TestConfigurationBlockedByScanAndShutdown(t *testing.T) {
	c, id, r := configController(t)
	scanner := &blockingScanner{started: make(chan struct{}, 1)}
	c.scanner = scanner
	if err := c.StartScan(); err != nil {
		t.Fatal(err)
	}
	<-scanner.started
	if _, err := c.Configure(id, r); !errors.Is(err, configuration.ErrBusy) {
		t.Fatal(err)
	}
	c.CancelScan()
	waitFinished(t, c)
	entered := make(chan struct{})
	c.writer = writerFunc(func(ctx context.Context, r configuration.Request) (configuration.Result, error) {
		close(entered)
		<-ctx.Done()
		return configuration.Result{Sent: true, Outcome: "unconfirmed"}, ctx.Err()
	})
	done := make(chan struct{})
	go func() { defer close(done); _, _ = c.Configure(id, r) }()
	<-entered
	c.Close()
	<-done
	if c.Snapshot().Configuring {
		t.Fatal("shutdown did not join writer")
	}
	if _, err := c.Configure(id, r); err == nil {
		t.Fatal("write allowed after close")
	}
}
func TestRecoveryDataStaysOutOfSnapshotsAndRequiresMatchingDevice(t *testing.T) {
	c, id, r := configController(t)
	r.Action = sadp.ExchangeCode
	r.Password = ""
	c.writer = writerFunc(func(_ context.Context, r configuration.Request) (configuration.Result, error) {
		return configuration.Result{MAC: r.MAC, Action: r.Action, Outcome: "challenge", Challenge: "SYNTHETIC-PRIVATE-CHALLENGE", Sent: true}, nil
	})
	result, err := c.Configure(id, r)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(c.Snapshot())
	if strings.Contains(string(snapshot), result.Challenge) {
		t.Fatal("challenge retained in diagnostics")
	}
	data, err := c.RecoveryExport(id, result)
	if err != nil || !strings.Contains(string(data), result.Challenge) {
		t.Fatal(string(data), err)
	}
	result.MAC = "02:00:00:00:00:02"
	if _, err := c.RecoveryExport(id, result); err == nil {
		t.Fatal("wrong target exported")
	}
}

func TestManagementFieldsInCSVExport(t *testing.T) {
	c, id, _ := configController(t)
	data, err := c.Export([]string{id}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[0]) != 18 || rows[0][15] != "SDK command port" || rows[1][15] != "8123" || rows[1][16] != "" || rows[1][17] != "" {
		t.Fatal("missing advertised port or invented recovery properties", rows)
	}
}
