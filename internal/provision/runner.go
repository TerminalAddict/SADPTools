package provision

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"time"
)

type Runner struct {
	Writer     configuration.Engine
	Rediscover func(context.Context, string) ([]devices.Device, error)
	Check      func(context.Context, Item, sadp.Action) error
	Credential func(Item, sadp.Action) (vault.Secret, error)
}

func MatchingObservation(before devices.Device, rows []devices.Device) (devices.Device, bool) {
	var found devices.Device
	count := 0
	for _, d := range rows {
		if d.MAC == before.MAC && d.MAC != "" {
			if devices.ConflictingIdentity(before, d) || d.IdentityConflict {
				return devices.Device{}, false
			}
			found = d
			count++
		}
	}
	return found, count == 1
}
func verified(action sadp.Action, n sadp.NetworkSettings, d devices.Device) bool {
	if action == sadp.Activate {
		return d.Activated != nil && *d.Activated
	}
	if action == sadp.UpdateNetwork {
		if d.DHCP == nil || *d.DHCP != n.DHCP || d.CommandPort == nil || *d.CommandPort != n.CommandPort {
			return false
		}
		if n.DHCP {
			return d.IPv4 != ""
		}
		return d.IPv4 == n.IPv4 && d.SubnetMask == n.SubnetMask && d.Gateway == n.Gateway
	}
	return false
}
func (r Runner) Run(ctx context.Context, p Plan, progress func(Plan)) Plan {
	emit := func() {
		if progress != nil {
			progress(Clone(p))
		}
	}
	if p.Blocked {
		for i := range p.Items {
			p.Items[i].State = "skipped"
			p.Items[i].Message = "Plan has validation errors; no writes sent"
		}
		emit()
		return p
	}
	for i := range p.Items {
		item := &p.Items[i]
		if item.State == "skipped" {
			continue
		}
		if ctx.Err() != nil {
			item.State = "skipped"
			item.Message = "Cancelled before this device"
			emit()
			continue
		}
		item.State = "in-progress"
		emit()

		for _, action := range item.Actions {
			if ctx.Err() != nil {
				item.State = "skipped"
				item.Message = "Cancelled before the next action; inspect any earlier confirmed steps"
				break
			}
			if e := r.Check(ctx, *item, action); e != nil {
				item.State = "skipped"
				item.Message = e.Error()
				break
			}
			secret, e := r.Credential(*item, action)
			if e != nil {
				item.State = "skipped"
				item.Message = "Required credential unavailable; unlock the vault and review assignments"
				break
			}
			req := configuration.Request{Action: action, MAC: item.Before.MAC, Interface: p.Options.Interface, TimeoutSeconds: p.Options.TimeoutSeconds, Password: secret.Password}
			if action == sadp.UpdateNetwork {
				req.Network = item.Proposed
			}
			if e = req.Validate(); e != nil {
				req.Password = ""
				secret.Clear()
				item.State = "skipped"
				item.Message = "Credential or request validation failed; no action sent"
				break
			}
			result, writeErr := r.Writer.Execute(ctx, req)
			req.Password = ""
			secret.Clear()
			step := Step{Action: action, Outcome: result.Outcome, Sent: result.Sent, At: time.Now().UTC().Format(time.RFC3339Nano)}
			switch {
			case result.Outcome == "rejected" || errors.Is(writeErr, configuration.ErrRejected):
				item.State = "rejected"
				step.Message = "Device rejected the action; check credentials and firmware compatibility"
			case result.Sent && (writeErr != nil || result.Outcome != "acknowledged"):
				item.State = "unconfirmed"
				step.Message = "Request may have applied. No retry or dependent write will be attempted; verify manually."
			case !result.Sent:
				item.State = "skipped"
				step.Message = "Action was not sent; check adapter and request validation"
			default:
				rows, e := r.Rediscover(ctx, p.Options.Interface)
				observed, ok := MatchingObservation(item.Before, rows)
				if e != nil || !ok || !verified(action, item.Proposed, observed) {
					item.State = "unconfirmed"
					step.Message = "Acknowledged, but stable-identity rediscovery did not verify every requested setting; manual verification required"
				} else {
					item.State = "confirmed"
					item.Verified = &observed
					step.Message = "Acknowledged and verified by stable-identity SADP rediscovery"
				}
			}
			item.Message = step.Message
			item.Steps = append(item.Steps, step)
			emit()
			if item.State != "confirmed" {
				break
			}
			if action != item.Actions[len(item.Actions)-1] {
				item.State = "in-progress"
				emit()
			}
		}
		emit()
	}
	return p
}

// Clone prevents GUI polling from sharing writable slices with the runner.
func Clone(p Plan) Plan {
	p.Items = append([]Item{}, p.Items...)
	p.Warnings = append([]string{}, p.Warnings...)
	p.Options.IDs = append([]string{}, p.Options.IDs...)
	for i := range p.Items {
		p.Items[i].Steps = append([]Step{}, p.Items[i].Steps...)
		p.Items[i].Actions = append([]sadp.Action{}, p.Items[i].Actions...)
	}
	return p
}
