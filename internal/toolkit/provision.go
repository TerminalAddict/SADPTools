package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
	"github.com/TerminalAddict/SADPTools/internal/provision"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"time"
)

func (m *Manager) provisionEnvironment(site string) (provision.Environment, error) {
	known, reserved, e := m.known(site)
	if e != nil {
		return provision.Environment{}, e
	}
	adapters, e := platform.Interfaces()
	if e != nil {
		return provision.Environment{}, e
	}
	return provision.Environment{Devices: m.Controller.Snapshot().Devices, Adapters: adapters, Known: known, Reservations: reserved, Scan: m.IPResult()}, nil
}
func (m *Manager) PreviewProvision(site string, o provision.Options) (provision.Plan, error) {
	env, e := m.provisionEnvironment(site)
	if e != nil {
		return provision.Plan{}, e
	}
	p, e := provision.Build(o, env)
	if e != nil {
		return p, e
	}
	p.SiteID = site
	p.ID = sites.ID()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job.Running {
		return p, errors.New("finish the current operation before creating a write plan")
	}
	m.plan = provision.Clone(p)
	m.planUsed = false
	return p, nil
}
func (m *Manager) ProvisionStatus() provision.Plan {
	m.mu.Lock()
	defer m.mu.Unlock()
	return provision.Clone(m.plan)
}
func sameAssignments(a, b provision.Plan) bool {
	if b.Blocked || len(a.Items) != len(b.Items) {
		return false
	}
	for i, x := range a.Items {
		y := b.Items[i]
		if x.ID != y.ID || x.Proposed != y.Proposed || !reflect.DeepEqual(x.Actions, y.Actions) || devices.ConflictingIdentity(x.Before, y.Before) || x.Before.IPv4 != y.Before.IPv4 || !reflect.DeepEqual(x.Before.Activated, y.Before.Activated) {
			return false
		}
	}
	return true
}
func (m *Manager) StartProvision(planID, confirmation, currentPassword, newPassword string) error {
	p := m.ProvisionStatus()
	if planID == "" || p.ID != planID || confirmation != "APPLY" || p.Blocked {
		return errors.New("review and explicitly approve the complete valid plan")
	}
	expires, e := time.Parse(time.RFC3339Nano, p.Expires)
	if e != nil || time.Now().After(expires) {
		return errors.New("plan expired; create and review a fresh preview")
	}
	env, e := m.provisionEnvironment(p.SiteID)
	if e != nil {
		return e
	}
	fresh, e := provision.Build(p.Options, env)
	if e != nil || !sameAssignments(p, fresh) {
		return errors.New("device identity, assignment or conflict evidence changed; create and review a new plan")
	}
	records := []sites.Record{}
	if p.SiteID != "" {
		records, e = m.Store.Records(p.SiteID)
		if e != nil {
			return e
		}
	}
	credential := func(item provision.Item, action sadp.Action) (vault.Secret, error) {
		if action == sadp.Activate || slices.Contains(item.Actions, sadp.Activate) {
			return vault.Secret{Username: "admin", Password: newPassword}, nil
		}
		if currentPassword != "" {
			return vault.Secret{Username: "admin", Password: currentPassword}, nil
		}
		id, ambiguous := sites.Match(records, item.Before)
		if id == "" || ambiguous {
			return vault.Secret{}, errors.New("no unambiguous saved credential assignment")
		}
		secret, _, e := m.Store.DeviceSecret(p.SiteID, id)
		if secret.Username != "" && secret.Username != "admin" {
			secret.Clear()
			return vault.Secret{}, errors.New("SADP network updates require the device admin password")
		}
		return secret, e
	}
	active := 0
	for _, item := range p.Items {
		for _, action := range item.Actions {
			active++
			secret, e := credential(item, action)
			if e != nil {
				return errors.New("required password unavailable; provide a batch password or unlock matching saved admin credentials")
			}
			req := configuration.Request{Action: action, MAC: item.Before.MAC, Interface: p.Options.Interface, TimeoutSeconds: p.Options.TimeoutSeconds, Password: secret.Password}
			if action == sadp.UpdateNetwork {
				req.Network = item.Proposed
			}
			e = req.Validate()
			req.Password = ""
			secret.Clear()
			if e != nil {
				return e
			}
		}
	}
	if active == 0 {
		return errors.New("plan contains no actions")
	}
	// Consume the reviewed plan before scheduling it; it cannot be submitted twice.
	m.mu.Lock()
	if m.plan.ID != planID || m.planUsed {
		m.mu.Unlock()
		return errors.New("write plan changed")
	}
	m.plan.Expires = time.Now().UTC().Format(time.RFC3339Nano)
	m.planUsed = true
	m.mu.Unlock()
	err := m.start("Provisioning", p.SiteID, func(ctx context.Context) error {
		defer func() { currentPassword = ""; newPassword = "" }()
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		rediscover := func(ctx context.Context, adapter string) ([]devices.Device, error) {
			r, e := discovery.New(logger).Scan(ctx, discovery.Options{Interface: adapter, Timeout: 5 * time.Second}, nil)
			for _, d := range r.Devices {
				m.Controller.Observe(d)
			}
			return r.Devices, e
		}
		runner := provision.Runner{Writer: configuration.New(logger), Credential: credential, Rediscover: rediscover, Check: func(ctx context.Context, item provision.Item, action sadp.Action) error {
			rows, e := rediscover(ctx, p.Options.Interface)
			if e != nil {
				return errors.New("pre-write rediscovery failed; no write sent")
			}
			observed, ok := provision.MatchingObservation(item.Before, rows)
			if !ok || !observed.SADPCompatible() || observed.Activated == nil {
				return errors.New("device identity or activation state could not be revalidated; no write sent")
			}
			if action == sadp.Activate && *observed.Activated || action == sadp.UpdateNetwork && !*observed.Activated {
				return errors.New("activation state changed; review a new plan")
			}
			baseline := item.Before
			if item.Verified != nil {
				baseline = *item.Verified
			}
			changed := baseline.IPv4 != "" && observed.IPv4 != "" && baseline.IPv4 != observed.IPv4 || baseline.SubnetMask != "" && observed.SubnetMask != "" && baseline.SubnetMask != observed.SubnetMask || baseline.Gateway != "" && observed.Gateway != "" && baseline.Gateway != observed.Gateway || baseline.CommandPort != nil && observed.CommandPort != nil && *baseline.CommandPort != *observed.CommandPort
			if changed {
				return errors.New("observed network settings changed after preview; create a new plan")
			}
			if action == sadp.UpdateNetwork {
				known, reserved, e := m.known(p.SiteID)
				if e != nil {
					return e
				}
				for _, d := range rows {
					if d.IPv4 == item.Proposed.IPv4 && d.MAC != item.Before.MAC {
						return errors.New("target IPv4 now belongs to another discovered device")
					}
				}
				if _, ok := reserved[item.Proposed.IPv4]; ok {
					return errors.New("target address was reserved after preview")
				}
				for _, k := range known {
					if k.IP == item.Proposed.IPv4 && k.MAC != item.Before.MAC {
						return errors.New("target address has another saved/live owner")
					}
				}
				if observed.IPv4 != item.Proposed.IPv4 {
					scan, e := ipscan.New().Scan(ctx, ipscan.Options{Interface: p.Options.Interface, Start: item.Proposed.IPv4, End: item.Proposed.IPv4, ICMP: true, TCPPorts: []int{80, 443, 554, 8000}, Workers: 1, Rate: 10, TimeoutMS: 350, ConfirmScope: true}, nil, reserved, nil)
					if e != nil || len(scan.Hosts) != 1 {
						return errors.New("target occupancy recheck failed; no write sent")
					}
					h := scan.Hosts[0]
					if h.Responding || h.Known || h.Conflict || h.Reserved != "" {
						return errors.New("target address has response, neighbour or reserved evidence; review before assigning")
					}
				}
			}
			return nil
		}}
		result := runner.Run(ctx, p, func(update provision.Plan) {
			m.mu.Lock()
			m.plan = provision.Clone(update)
			m.mu.Unlock()
			done := 0
			for _, item := range update.Items {
				if item.State != "pending" && item.State != "in-progress" {
					done++
				}
			}
			m.progress(done, len(update.Items), "Serial queue; confirmed results require rediscovery. No automatic write retries.")
		})
		m.mu.Lock()
		m.plan = provision.Clone(result)
		m.plan.Expires = time.Now().UTC().Format(time.RFC3339Nano)
		m.mu.Unlock()
		if p.SiteID != "" {
			data, _ := json.Marshal(result)
			outcome := "completed"
			for _, item := range result.Items {
				if item.State == "unconfirmed" || item.State == "rejected" || item.State == "skipped" && len(item.Actions) > 0 {
					outcome = "review-required"
				}
			}
			if ctx.Err() != nil {
				outcome = "cancelled"
			}
			run, e := m.Store.SaveRun(sites.Run{SiteID: p.SiteID, Kind: "provision", Scope: p.Options.Interface, Outcome: outcome, Data: string(data)})
			if e != nil {
				return e
			}
			m.setRun(run.ID)
			if p.Options.SaveVerified {
				verified := []devices.Device{}
				for _, item := range result.Items {
					if item.State == "confirmed" && item.Verified != nil {
						verified = append(verified, *item.Verified)
					}
				}
				if len(verified) > 0 {
					if e = m.Store.SaveProvisioned(p.SiteID, verified); e != nil {
						return e
					}
				}
			}
		}
		return ctx.Err()
	})
	if err != nil {
		currentPassword = ""
		newPassword = ""
	}
	return err
}
