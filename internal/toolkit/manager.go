// Package toolkit coordinates site/network workflows independently of Wails.
package toolkit

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/audit"
	"github.com/TerminalAddict/SADPTools/internal/cameraip"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/provision"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"sync"
	"time"
)

type Job struct {
	RunID     string `json:"runID"`
	Kind      string `json:"kind"`
	SiteID    string `json:"siteID"`
	Running   bool   `json:"running"`
	Started   string `json:"started"`
	Completed string `json:"completed"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
	Error     string `json:"error"`
	Cancelled bool   `json:"cancelled"`
}
type Manager struct {
	cameraIP      *pendingCameraIP
	cameraIPProbe func(context.Context, cameraip.Options, vault.Secret, func(string)) (cameraip.Result, error)
	walk          WalkTest
	audit         audit.Report
	planUsed      bool
	plan          provision.Plan
	ipResult      ipscan.Result
	Store         *sites.Store
	Controller    *app.Controller
	mu            sync.Mutex
	job           Job
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	closed        bool
	notify        func()
}

func New(path string, c *app.Controller, notify func()) (*Manager, error) {
	store, e := sites.Open(path)
	if e != nil {
		return nil, e
	}
	return &Manager{Store: store, Controller: c, notify: notify}, nil
}
func (m *Manager) changed() {
	if m.notify != nil {
		m.notify()
	}
}
func (m *Manager) Job() Job { m.mu.Lock(); defer m.mu.Unlock(); return m.job }
func (m *Manager) progress(done, total int, message string) {
	m.mu.Lock()
	m.job.Done = done
	m.job.Total = total
	m.job.Message = message
	m.mu.Unlock()
	m.changed()
}
func (m *Manager) start(kind, site string, fn func(context.Context) error) error {
	m.mu.Lock()
	if m.closed || m.job.Running {
		m.mu.Unlock()
		return errors.New("another toolkit operation is running or the app is closing")
	}
	ctx, finish, e := m.Controller.AcquireTask()
	if e != nil {
		m.mu.Unlock()
		return e
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.job = Job{Kind: kind, SiteID: site, Running: true, Started: time.Now().UTC().Format(time.RFC3339)}
	m.wg.Add(1)
	m.mu.Unlock()
	m.changed()
	go func() {
		defer m.wg.Done()
		defer finish()
		defer cancel()
		err := fn(ctx)
		m.mu.Lock()
		m.job.Running = false
		m.job.Completed = time.Now().UTC().Format(time.RFC3339)
		m.job.Cancelled = errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled)
		if err != nil && !m.job.Cancelled {
			m.job.Error = err.Error()
		}
		m.cancel = nil
		m.mu.Unlock()
		m.changed()
	}()
	return nil
}
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.Store.Close()
}
