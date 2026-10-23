package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/audit"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"time"
)

func (m *Manager) AuditStatus() audit.Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	return audit.Clone(m.audit)
}
func (m *Manager) StartAudit(site string, ids []string, pins map[string]string) error {
	view, e := m.Store.View(site)
	if e != nil {
		return e
	}
	if view.Site.Archived {
		return errors.New("unarchive the site before auditing")
	}
	if len(ids) == 0 || len(ids) > 1000 {
		return errors.New("select 1–1000 saved devices for the audit")
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	records := []sites.Record{}
	for _, r := range view.Devices {
		if selected[r.ID] && !r.Removed {
			records = append(records, r)
			delete(selected, r.ID)
		}
	}
	if len(selected) > 0 {
		return errors.New("selected device was removed or no longer belongs to this site")
	}
	return m.start("Health audit", site, func(ctx context.Context) error {
		m.mu.Lock()
		m.audit = audit.Report{SiteID: site, SiteName: view.Site.Name, Started: time.Now().UTC().Format(time.RFC3339Nano), Total: len(records), Findings: []audit.Finding{}}
		m.mu.Unlock()
		engine := audit.New()
		engine.Secret = func(r sites.Record) (vault.Secret, string, error) { return m.Store.DeviceSecret(site, r.ID) }
		result := engine.Run(ctx, view.Site, records, view.Devices, pins, func(report audit.Report) {
			m.mu.Lock()
			m.audit = audit.Clone(report)
			m.mu.Unlock()
			m.progress(report.Done, report.Total, "Read-only health checks; unknown results require follow-up")
		})
		m.mu.Lock()
		m.audit = audit.Clone(result)
		m.mu.Unlock()
		data, _ := json.Marshal(result)
		outcome := "completed"
		if result.Cancelled {
			outcome = "cancelled"
		}
		run, e := m.Store.SaveRun(sites.Run{SiteID: site, Kind: "audit", Scope: "Explicitly selected saved devices; read-only TCP, HTTP/HTTPS and available ONVIF services", Outcome: outcome, Data: string(data)})
		if e != nil {
			return e
		}
		m.setRun(run.ID)
		return ctx.Err()
	})
}
