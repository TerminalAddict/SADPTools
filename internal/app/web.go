package app

import (
	"context"
	"errors"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type webRecord struct {
	target  webinterface.Target
	result  webinterface.Result
	checked time.Time
}

func (c *Controller) webTarget(id string) (webinterface.Target, error) {
	for _, d := range c.inventory.Snapshot() {
		if d.ID == id {
			t := webinterface.Target{Address: d.IPv4}
			if d.HTTPPort != nil {
				t.Port = *d.HTTPPort
			}
			if d.HTTPSPort != nil {
				t.HTTPSPort = *d.HTTPSPort
			}
			return t, t.Validate()
		}
	}
	return webinterface.Target{}, errors.New("device is no longer in the inventory")
}

// CheckDeviceWeb is read-only. Selection changes cancel the previous probe;
// cached results are tied to the current inventory address and advertised port.
func (c *Controller) CheckDeviceWeb(id string, refresh bool) (webinterface.Result, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return webinterface.Result{}, errors.New("application is closing")
	}
	target, err := c.webTarget(id)
	if err != nil {
		c.mu.Unlock()
		return webinterface.Result{}, err
	}
	if old, ok := c.webResults[id]; !refresh && ok && old.target == target && time.Since(old.checked) < time.Minute {
		c.mu.Unlock()
		return old.result, nil
	}
	if c.configuring {
		c.mu.Unlock()
		return webinterface.Result{}, configuration.ErrBusy
	}
	if c.webCancel != nil {
		c.webCancel()
	}
	c.webSequence++
	sequence := c.webSequence
	ctx, cancel := context.WithTimeout(c.ctx, 6*time.Second)
	c.webCancel = cancel
	c.wg.Add(1)
	c.mu.Unlock()
	defer c.wg.Done()
	defer cancel()
	result, err := c.webDetector.Detect(ctx, target)
	c.mu.Lock()
	defer c.mu.Unlock()
	current, currentErr := c.webTarget(id)
	if sequence != c.webSequence || c.closed || currentErr != nil || current != target {
		return webinterface.Result{}, errors.New("camera selection or address changed; check its web interface again")
	}
	c.webCancel = nil
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		if result.Endpoint.URL != "" && !result.Endpoint.ValidFor(target.Address) {
			return webinterface.Result{}, errors.New("camera returned an invalid web endpoint")
		}
		c.rememberWeb(id, target, result)
	}
	return result, err
}

// Caller holds c.mu. Certificate details are public metadata, never credentials.
func (c *Controller) rememberWeb(id string, target webinterface.Target, result webinterface.Result) {
	if len(c.webResults) >= 256 {
		clear(c.webResults)
	}
	c.webResults[id] = webRecord{target, result, time.Now()}
}

func (c *Controller) DeviceURL(id string) (string, error) {
	result, err := c.CheckDeviceWeb(id, false)
	if err != nil {
		return "", err
	}
	if result.Endpoint.URL == "" {
		return "", errors.New(result.Message)
	}
	return result.Endpoint.URL, nil
}
