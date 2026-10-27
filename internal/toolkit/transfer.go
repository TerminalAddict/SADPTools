package toolkit

import (
	"errors"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/sites"
)

type pendingSiteImport struct {
	data    *sites.PreparedImport
	token   string
	expires time.Time
	timer   *time.Timer
}

func (m *Manager) clearSiteImport() {
	if m.siteImport != nil {
		m.siteImport.timer.Stop()
		m.siteImport.data.Clear()
		m.siteImport = nil
	}
}

func (m *Manager) PrepareSiteImport(data []byte, passphrase string) (sites.ImportPreview, error) {
	p, err := sites.PrepareImport(data, passphrase)
	if err != nil {
		return sites.ImportPreview{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		p.Clear()
		return sites.ImportPreview{}, errors.New("site storage is closing")
	}
	m.clearSiteImport()
	token := sites.ID()
	pending := &pendingSiteImport{data: p, token: token, expires: time.Now().Add(10 * time.Minute)}
	pending.timer = time.AfterFunc(10*time.Minute, func() { m.CancelSiteImport(token) })
	m.siteImport = pending
	preview := p.Preview()
	preview.Token = token
	return preview, nil
}

func (m *Manager) CancelSiteImport(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.siteImport != nil && m.siteImport.token == token {
		m.clearSiteImport()
	}
}

func (m *Manager) CommitSiteImport(token, name string) (sites.Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.siteImport
	if m.closed || p == nil || p.token != token {
		return sites.Site{}, errors.New("choose and review the site file again")
	}
	if time.Now().After(p.expires) {
		m.clearSiteImport()
		return sites.Site{}, errors.New("site import expired; choose the file again")
	}
	site, err := m.Store.ImportSite(p.data, name)
	if err == nil {
		m.clearSiteImport()
	}
	return site, err
}
