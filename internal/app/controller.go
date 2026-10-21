package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

type Scanner interface {
	Scan(context.Context, discovery.Options, func(devices.Device)) (discovery.Result, error)
}
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Details string `json:"details"`
}
type Snapshot struct {
	Devices     []devices.Device `json:"devices"`
	Configuring bool             `json:"configuring"`
	Scanning    bool             `json:"scanning"`
	LastScan    string           `json:"lastScan"`
	LastError   string           `json:"lastError"`
	Warnings    []string         `json:"warnings"`
	Logs        []LogEntry       `json:"logs"`
}

type Controller struct {
	mu              sync.Mutex
	logMu           sync.Mutex
	inventory       *devices.Inventory
	scanner         Scanner
	writer          configuration.Engine
	configuring     bool
	operationCancel context.CancelFunc
	log             *slog.Logger
	logs            []LogEntry
	settings        Settings
	path            string
	ctx             context.Context
	cancel          context.CancelFunc
	scanCancel      context.CancelFunc
	wg              sync.WaitGroup
	closed          bool
	scanning        bool
	lastScan        string
	lastError       string
	warnings        []string
	nextScan        time.Time
	notify          func()
}

func New(path string, notify func()) *Controller {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Controller{inventory: devices.NewInventory(), path: path, ctx: ctx, cancel: cancel, notify: notify, logs: []LogEntry{}, warnings: []string{}}
	c.log = slog.New(slog.NewJSONHandler(c, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c.scanner = discovery.New(c.log)
	c.writer = configuration.New(c.log)
	s, err := loadSettings(path)
	c.settings = s
	if err != nil {
		c.log.Warn("Could not load settings; using defaults", "error", err)
	}
	c.nextScan = time.Now().Add(time.Second)
	c.wg.Add(1)
	go c.automaticLoop()
	return c
}

// Write implements the bounded diagnostic sink used by slog, not a frontend method.
func (c *Controller) Write(p []byte) (int, error) {
	var record map[string]any
	if err := json.Unmarshal(p, &record); err != nil {
		return len(p), nil
	}
	entry := LogEntry{Time: fmt.Sprint(record["time"]), Level: fmt.Sprint(record["level"]), Message: fmt.Sprint(record["msg"])}
	delete(record, "time")
	delete(record, "level")
	delete(record, "msg")
	if len(record) > 0 {
		b, _ := json.Marshal(record)
		entry.Details = string(b)
	}
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if len(c.logs) == 500 {
		copy(c.logs, c.logs[1:])
		c.logs = c.logs[:499]
	}
	c.logs = append(c.logs, entry)
	return len(p), nil
}
func (c *Controller) changed() {
	if c.notify != nil {
		c.notify()
	}
}
func (c *Controller) Settings() Settings { c.mu.Lock(); defer c.mu.Unlock(); return c.settings }
func (c *Controller) SaveSettings(s Settings) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("application is closing")
	}
	if err := saveSettings(c.path, s); err != nil {
		return err
	}
	c.settings = s
	c.nextScan = time.Now().Add(time.Duration(s.IntervalSeconds) * time.Second)
	return nil
}
func (c *Controller) Interfaces() ([]platform.Adapter, error) {
	adapters, err := platform.Interfaces()
	if err != nil {
		c.log.Error("Interface enumeration failed", "error", err)
	} else {
		for _, a := range adapters {
			c.log.Debug("Network interface", "name", a.Name, "addresses", a.Addresses, "suitable", a.Suitable, "reason", a.Reason)
		}
	}
	return adapters, err
}
func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	s := Snapshot{Configuring: c.configuring, Scanning: c.scanning, LastScan: c.lastScan, LastError: c.lastError, Warnings: append([]string{}, c.warnings...)}
	c.mu.Unlock()
	s.Devices = c.inventory.Snapshot()
	c.logMu.Lock()
	s.Logs = append([]LogEntry{}, c.logs...)
	c.logMu.Unlock()
	return s
}
func (c *Controller) StartScan() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("application is closing")
	}
	if c.configuring {
		c.mu.Unlock()
		return configuration.ErrBusy
	}
	if c.scanning {
		c.mu.Unlock()
		return discovery.ErrBusy
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.scanCancel = cancel
	opts := discovery.Options{Interface: c.settings.Interface, Timeout: time.Duration(c.settings.TimeoutSeconds) * time.Second}
	c.scanning = true
	c.lastError = ""
	c.warnings = []string{}
	c.wg.Add(1)
	c.mu.Unlock()
	c.changed()
	go func() {
		defer c.wg.Done()
		defer cancel()
		result, err := c.scanner.Scan(ctx, opts, func(d devices.Device) { c.inventory.Upsert(d, time.Now()); c.changed() })
		c.mu.Lock()
		c.scanning = false
		c.scanCancel = nil
		c.lastScan = time.Now().Format(time.RFC3339)
		c.warnings = result.Warnings
		if err != nil && !errors.Is(err, context.Canceled) {
			c.lastError = err.Error()
		}
		c.nextScan = time.Now().Add(time.Duration(c.settings.IntervalSeconds) * time.Second)
		c.mu.Unlock()
		if errors.Is(err, context.Canceled) {
			c.log.Info("Discovery cancelled")
		} else if err != nil {
			c.log.Error("Discovery failed", "error", err)
		} else {
			c.log.Info("Discovery completed", "devices", len(result.Devices), "warnings", len(result.Warnings))
		}
		c.changed()
	}()
	return nil
}
func (c *Controller) CancelScan() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scanCancel != nil {
		c.scanCancel()
	}
}
func (c *Controller) automaticLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case now := <-ticker.C:
			c.mu.Lock()
			ready := c.settings.Automatic && !c.scanning && !c.configuring && !now.Before(c.nextScan)
			c.mu.Unlock()
			if ready {
				_ = c.StartScan()
			}
		}
	}
}
func (c *Controller) Close() { c.mu.Lock(); c.closed = true; c.cancel(); c.mu.Unlock(); c.wg.Wait() }
func (c *Controller) DeviceURL(id string) (string, error) {
	for _, d := range c.inventory.Snapshot() {
		if d.ID == id {
			if u := d.WebURL(); u != "" {
				return u, nil
			}
			return "", errors.New("device has no usable advertised HTTP address and port")
		}
	}
	return "", errors.New("device is no longer in the inventory")
}
