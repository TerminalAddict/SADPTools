// Package ptz bounds operator-requested movement and always attempts a final
// Stop. It contains no GUI or vendor dependencies.
package ptz

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Provider interface {
	Move(context.Context, float64, float64, float64) error
	Stop(context.Context) error
	Recall(context.Context, string) error
}
type Controller struct {
	mu         sync.Mutex
	command    sync.Mutex
	provider   Provider
	cancel     context.CancelFunc
	timer      *time.Timer
	closed     bool
	last       time.Time
	generation uint64
	stopError  string
}

func New(p Provider) *Controller { return &Controller{provider: p} }
func (c *Controller) Pulse(ctx context.Context, x, y, z float64) error {
	return c.run(ctx, 800*time.Millisecond, func(ctx context.Context) error { return c.provider.Move(ctx, x, y, z) })
}
func (c *Controller) Recall(ctx context.Context, id string) error {
	return c.run(ctx, 15*time.Second, func(ctx context.Context) error { return c.provider.Recall(ctx, id) })
}
func (c *Controller) run(ctx context.Context, limit time.Duration, fn func(context.Context) error) error {
	c.command.Lock()
	defer c.command.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("camera controls are closed")
	}
	if time.Since(c.last) < 300*time.Millisecond {
		c.mu.Unlock()
		return errors.New("movement rate limit; release the control before another step")
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.generation++
	generation := c.generation
	c.last = time.Now()
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	c.cancel = cancel
	c.mu.Unlock()
	err := fn(work)
	cancel()
	c.mu.Lock()
	superseded := c.closed || c.generation != generation
	if !superseded && err == nil {
		c.timer = time.AfterFunc(limit, func() { c.stopGeneration(generation) })
	}
	c.mu.Unlock()
	if err != nil || superseded {
		c.attemptStop()
	}
	return err
}
func (c *Controller) stopGeneration(generation uint64) {
	c.mu.Lock()
	if generation != c.generation {
		c.mu.Unlock()
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	c.command.Lock()
	defer c.command.Unlock()
	c.mu.Lock()
	current := generation == c.generation
	c.mu.Unlock()
	if current {
		c.attemptStop()
	}
}
func (c *Controller) attemptStop() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	e := c.provider.Stop(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if e != nil {
		c.stopError = "Stop outcome unconfirmed — verify camera movement"
	} else {
		c.stopError = ""
	}
}

// Action applies the same release/failsafe rules to focus and other bounded
// movements implemented by a capability-checked provider.
func (c *Controller) Action(ctx context.Context, fn func(context.Context) error) error {
	return c.run(ctx, time.Second, fn)
}
func (c *Controller) Stop() { c.mu.Lock(); g := c.generation; c.mu.Unlock(); c.stopGeneration(g) }
func (c *Controller) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.Stop()
	c.mu.Lock()
	c.generation++
	c.mu.Unlock()
}
func (c *Controller) StopError() string { c.mu.Lock(); defer c.mu.Unlock(); return c.stopError }
