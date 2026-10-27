package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type State struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	CurrentVersion string `json:"currentVersion"`
	Version        string `json:"version"`
	ReleaseURL     string `json:"releaseURL"`
	Downloaded     int64  `json:"downloaded"`
	Size           int64  `json:"size"`
	Supported      bool   `json:"supported"`
	Platform       string `json:"platform"`
}

type Manager struct {
	mu          sync.Mutex
	wg          sync.WaitGroup
	client      *Client
	state       State
	offer       *Offer
	path, cache string
	channel     string
	cancel      context.CancelFunc
	closed      bool
	notify      func()
	launch      func(string) error
}

func NewManager(current, cache string, notify func()) *Manager {
	m := &Manager{client: NewClient(), cache: cache, notify: notify, launch: launchInstaller}
	m.state = State{Status: "idle", CurrentVersion: current, ReleaseURL: ReleasesURL,
		Platform:  runtime.GOOS,
		Supported: runtime.GOOS == "windows" && runtime.GOARCH == "amd64"}
	// A launched installer must outlive us. Only prune our old private directories
	// on a later launch, never a currently downloaded or running installer.
	entries, _ := os.ReadDir(cache)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "update-") {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
				_ = os.RemoveAll(filepath.Join(cache, e.Name()))
			}
		}
	}
	return m
}

func (m *Manager) Snapshot() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *Manager) changed() {
	if m.notify != nil {
		m.notify()
	}
}
func (m *Manager) busy() bool {
	return m.closed || m.state.Status == "checking" || m.state.Status == "downloading" || m.state.Status == "installing"
}

func (m *Manager) Check(channel string) error {
	m.mu.Lock()
	if m.busy() {
		m.mu.Unlock()
		return errors.New("an update action is already in progress; cancel or finish it first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	oldPath := m.path
	m.path, m.channel = "", channel
	m.state.Status, m.state.Message = "checking", "Checking GitHub releases…"
	m.state.Version, m.state.ReleaseURL, m.state.Size, m.state.Downloaded = "", ReleasesURL, 0, 0
	m.offer = nil
	current := m.state.CurrentVersion
	m.wg.Add(1)
	m.mu.Unlock()
	if oldPath != "" {
		_ = os.RemoveAll(filepath.Dir(oldPath))
	}
	m.changed()
	go func() {
		defer m.wg.Done()
		defer cancel()
		offer, err := m.client.Check(ctx, current, channel)
		m.mu.Lock()
		m.cancel = nil
		switch {
		case errors.Is(err, context.Canceled):
			m.state.Status, m.state.Message = "idle", "Update check cancelled."
		case err != nil:
			m.state.Status, m.state.Message = "error", err.Error()
		case offer == nil:
			m.state.Status, m.state.Message = "current", "You are up to date on this update channel."
		default:
			m.offer = offer
			m.state.Status, m.state.Message = "available", "A new version is available."
			m.state.Version, m.state.ReleaseURL, m.state.Size = offer.Version, offer.ReleaseURL, offer.Size
			if !m.state.Supported {
				m.state.Message = "A new version is available for this platform. Open the release page to download and install it manually."
			}
		}
		m.mu.Unlock()
		m.changed()
	}()
	return nil
}

func (m *Manager) Download(channel string) error {
	m.mu.Lock()
	if m.channel != channel {
		m.mu.Unlock()
		return errors.New("update channel changed; check for updates again")
	}
	if m.busy() || m.offer == nil || !m.state.Supported || m.state.Status == "ready" {
		m.mu.Unlock()
		return errors.New("no Windows AMD64 update is available to download")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	offer := *m.offer
	m.state.Status, m.state.Message, m.state.Downloaded = "downloading", "Downloading and verifying the installer…", 0
	m.wg.Add(1)
	m.mu.Unlock()
	m.changed()
	go func() {
		defer m.wg.Done()
		defer cancel()
		last := time.Time{}
		path, err := m.client.Download(ctx, offer, m.cache, func(n int64) {
			m.mu.Lock()
			m.state.Downloaded = n
			m.mu.Unlock()
			if time.Since(last) > 200*time.Millisecond {
				last = time.Now()
				m.changed()
			}
		})
		m.mu.Lock()
		m.cancel = nil
		switch {
		case errors.Is(err, context.Canceled):
			m.state.Status, m.state.Message = "available", "Download cancelled."
		case err != nil:
			m.state.Status, m.state.Message = "available", "Download failed: "+err.Error()
		default:
			m.path = path
			m.state.Status, m.state.Message = "ready", "Download verified. Ready to install."
		}
		m.mu.Unlock()
		m.changed()
	}()
	return nil
}

func (m *Manager) Cancel() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	path := ""
	if m.state.Status == "ready" {
		path, m.path = m.path, ""
		m.state.Status, m.state.Message, m.state.Downloaded = "available", "Downloaded installer discarded.", 0
	}
	m.mu.Unlock()
	if path != "" {
		_ = os.RemoveAll(filepath.Dir(path))
	}
	m.changed()
}

// Install starts a verified installer, never a shell command. Its /WAITPID
// protocol waits for our exit before touching the installed application.
// The caller must exclude device operations and quit only after success.
func (m *Manager) Install(channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.channel != channel {
		return errors.New("update channel changed; check for updates again")
	}
	if m.busy() || m.state.Status != "ready" || m.offer == nil || !m.state.Supported {
		return errors.New("download and verify an update before installing")
	}
	if err := VerifyFile(m.path, *m.offer); err != nil {
		_ = os.RemoveAll(filepath.Dir(m.path))
		m.path = ""
		m.state.Status, m.state.Message = "available", err.Error()
		return err
	}
	if err := m.launch(m.path); err != nil {
		return err
	}
	m.state.Status, m.state.Message = "installing", "Closing SADPTools and opening setup…"
	return nil
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()
	path := m.path
	if m.state.Status == "installing" {
		path = ""
	}
	m.mu.Unlock()
	if path != "" {
		_ = os.RemoveAll(filepath.Dir(path))
	}
}
