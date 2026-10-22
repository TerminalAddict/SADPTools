package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

// Configure operates on an inventory identity, never an arbitrary webview-supplied MAC.
// The synchronous result is not persisted in snapshots: it may contain recovery data.
func (c *Controller) Configure(id string, r configuration.Request) (configuration.Result, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return configuration.Result{}, errors.New("application is closing")
	}
	if c.scanning || c.configuring {
		c.mu.Unlock()
		return configuration.Result{}, configuration.ErrBusy
	}
	var device *devices.Device
	for _, d := range c.inventory.Snapshot() {
		if d.ID == id {
			device = &d
			break
		}
	}
	if device == nil || device.MAC == "" {
		c.mu.Unlock()
		return configuration.Result{}, errors.New("select a discovered device with a valid MAC address")
	}
	if r.MAC != "" && devices.NormalizeMAC(r.MAC) != device.MAC {
		c.mu.Unlock()
		return configuration.Result{}, errors.New("selected device identity changed; reopen the operation")
	}
	adapter, err := operationInterface(*device, r.Interface, c.settings.Interface)
	if err != nil {
		c.mu.Unlock()
		return configuration.Result{}, err
	}
	r.Interface = adapter
	if r.Action == sadp.Activate && device.Activated != nil && *device.Activated {
		c.mu.Unlock()
		return configuration.Result{}, errors.New("device already reports active status")
	}
	if r.Action == sadp.UpdateNetwork && device.Activated != nil && !*device.Activated {
		c.mu.Unlock()
		return configuration.Result{}, errors.New("activate this device before changing network settings")
	}
	r.MAC = device.MAC
	if err := r.Validate(); err != nil {
		c.mu.Unlock()
		return configuration.Result{}, err
	}
	c.configuring = true
	ctx, cancel := context.WithCancel(c.ctx)
	c.operationCancel = cancel
	c.wg.Add(1)
	c.mu.Unlock()
	c.changed()
	defer func() {
		cancel()
		c.mu.Lock()
		c.configuring = false
		c.operationCancel = nil
		c.nextScan = time.Now().Add(time.Duration(c.settings.IntervalSeconds) * time.Second)
		c.mu.Unlock()
		c.changed()
		c.wg.Done()
	}()
	result, err := c.writer.Execute(ctx, r)
	// No optimistic inventory changes: the next inquiry must actually observe them.
	c.log.Info("Configuration finished", "action", r.Action, "mac", device.MAC, "outcome", result.Outcome)
	if err != nil && result.Sent {
		// Wails methods with an error discard the return value. Deliver uncertain/rejected
		// outcomes as structured results so the UI can explain what was (not) confirmed.
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("operation not sent: %w", err)
	}
	return result, nil
}

// An empty selection uses a route observed during discovery. Prefer the scan
// adapter when it saw this device; otherwise use the first recorded route.
// Resolve exactly one adapter before sending, without retries on other routes.
func operationInterface(device devices.Device, requested, preferred string) (string, error) {
	first, match := "", ""
	for _, a := range device.Interfaces {
		if a.Name == "" {
			continue
		}
		if requested != "" && a.Name == requested {
			return requested, nil
		}
		if first == "" {
			first = a.Name
		}
		if a.Name == preferred {
			match = a.Name
		}
	}
	if requested != "" {
		return "", errors.New("select an adapter on which this device was discovered")
	}
	if match != "" {
		return match, nil
	}
	if first != "" {
		return first, nil
	}
	return "", errors.New("no discovered connection is available for this device; rediscover before trying again")
}

func (c *Controller) CancelOperation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operationCancel != nil {
		c.operationCancel()
	}
}

// RecoveryExport is our own support-data format, not Hikvision's encrypted XML file.
func (c *Controller) RecoveryExport(id string, r configuration.Result) ([]byte, error) {
	if !r.Action.ReadOnly() || r.Outcome != "challenge" || r.Challenge == "" || len(r.Challenge) > 4096 {
		return nil, errors.New("no recovery data is available to export")
	}
	for _, d := range c.inventory.Snapshot() {
		if d.ID == id && d.MAC == r.MAC {
			return json.MarshalIndent(struct {
				Format    string      `json:"format"`
				Exported  string      `json:"exported"`
				MAC       string      `json:"mac"`
				Serial    string      `json:"serial"`
				Model     string      `json:"model"`
				Method    sadp.Action `json:"method"`
				Challenge string      `json:"challenge"`
				Note      string      `json:"note"`
			}{"sadptools-recovery-v1", time.Now().UTC().Format(time.RFC3339), d.MAC, d.Serial, d.Model, r.Action, r.Challenge, "Private device recovery data. This is not a Hikvision SADP XML export; confirm the accepted format with your authorised support provider."}, "", "  ")
		}
	}
	return nil, errors.New("recovery data does not match the selected device")
}
