package app

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"time"
)

// AcquireTask shares the same exclusion/cancellation boundary as existing device
// operations. Toolkit work cannot race a write, discovery or update installation.
func (c *Controller) AcquireTask() (context.Context, func(), error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, nil, errors.New("application is closing")
	}
	if c.scanning || c.configuring {
		c.mu.Unlock()
		return nil, nil, configuration.ErrBusy
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.configuring = true
	c.operationCancel = cancel
	if c.webCancel != nil {
		c.webCancel()
	}
	c.wg.Add(1)
	c.mu.Unlock()
	c.changed()
	return ctx, func() {
		cancel()
		c.mu.Lock()
		c.configuring = false
		c.operationCancel = nil
		c.nextScan = time.Now().Add(time.Duration(c.settings.IntervalSeconds) * time.Second)
		c.mu.Unlock()
		c.changed()
		c.wg.Done()
	}, nil
}
func (c *Controller) Observe(d devices.Device) { c.inventory.Upsert(d, time.Now()); c.changed() }
