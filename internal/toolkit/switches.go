package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/switches"
)

func (m *Manager) PollSwitch(site, id string) error {
	assets, e := m.Store.Switches(site)
	if e != nil {
		return e
	}
	var selected *switches.Asset
	for _, a := range assets {
		if a.ID == id {
			v := a
			selected = &v
		}
	}
	if selected == nil {
		return errors.New("select a registered switch in this site")
	}
	a := *selected
	secret, e := m.Store.Secret(site, a.CredentialID)
	if e != nil {
		return e
	}
	view, e := m.Store.View(site)
	if e != nil {
		secret.Clear()
		return e
	}
	if view.Site.Archived {
		secret.Clear()
		return errors.New("unarchive the site before polling")
	}
	targets := []devices.Device{}
	for _, r := range view.Devices {
		if !r.Removed {
			d := r.Device
			d.ID = r.ID
			targets = append(targets, d)
		}
	}
	previous, e := m.Store.SwitchReports(site)
	if e != nil {
		secret.Clear()
		return e
	}
	err := m.start("Switch inspection", site, func(ctx context.Context) error {
		defer secret.Clear()
		report, pollErr := switches.Poll(ctx, a, secret, targets, func(message string) { m.progress(0, 0, message) })
		if pollErr != nil {
			report.Warnings = append(report.Warnings, "SNMP poll incomplete; credentials, routing, access restrictions or cancellation may be responsible")
		}
		for i, current := range report.Mappings {
			for _, old := range previous {
				if old.AssetID != a.ID {
					continue
				}
				for _, mapping := range old.Mappings {
					if current.MAC == mapping.MAC && current.Port != 0 && mapping.Port != 0 && current.Port != mapping.Port {
						report.Mappings[i].Confidence = "ambiguous"
						report.Mappings[i].Evidence = append(report.Mappings[i].Evidence, "Forwarding port differs from the previous poll; verify a MAC move, topology change or stale entry")
					}
				}
			}
		}
		data, _ := json.Marshal(report)
		outcome := "completed"
		if ctx.Err() != nil {
			outcome = "cancelled"
		} else if pollErr != nil {
			outcome = "incomplete"
		}
		run, e := m.Store.SaveRun(sites.Run{SiteID: site, Kind: "switch", Scope: a.Name + " · " + a.Host + " · read-only SNMPv" + a.Version, Outcome: outcome, Data: string(data)})
		if e != nil {
			return e
		}
		m.setRun(run.ID)
		m.progress(len(report.Ports), len(report.Ports), "Switch observations saved with confidence and timestamps")
		return pollErr
	})
	if err != nil {
		secret.Clear()
	}
	return err
}
