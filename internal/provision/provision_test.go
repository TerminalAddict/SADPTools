package provision

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"strings"
	"testing"
	"time"
)

// Synthetic devices only. These tests never open a SADP socket or write hardware.
func fixture(mac, ip string) devices.Device {
	active := true
	port := 8000
	d := devices.Device{MAC: mac, IPv4: ip, Serial: mac, Activated: &active, CommandPort: &port, Protocols: []string{"sadp"}, Interfaces: []devices.Interface{{Name: "fixture"}}, SubnetMask: "255.255.255.0", Gateway: "192.0.2.1"}
	d.ID = d.Key()
	return d
}
func environment() Environment {
	return Environment{Devices: []devices.Device{fixture("02:00:00:00:00:01", "192.0.2.100"), fixture("02:00:00:00:00:02", "192.0.2.101")}, Adapters: []platform.Adapter{{Name: "fixture", Suitable: true, UnicastSuitable: true, Addresses: []string{"192.0.2.2/24"}, Gateway: "192.0.2.1"}}, Reservations: map[string]string{"192.0.2.3": "synthetic reservation"}, Known: []ipscan.Known{{IP: "192.0.2.4", MAC: "02:00:00:00:00:09"}}, Scan: ipscan.Result{Hosts: []ipscan.Host{{IP: "192.0.2.5", Responding: true}}}}
}
func TestPreviewSkipsProtectedAndOccupied(t *testing.T) {
	env := environment()
	o := Options{IDs: []string{env.Devices[0].ID, env.Devices[1].ID}, Interface: "fixture", ConfigureNetwork: true, Start: "192.0.2.0", End: "192.0.2.10", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", TimeoutSeconds: 2}
	p, e := Build(o, env)
	if e != nil || p.Blocked || p.Items[0].Proposed.IPv4 != "192.0.2.6" || p.Items[1].Proposed.IPv4 != "192.0.2.7" {
		t.Fatal(p, e)
	}
	o.End = "192.0.2.5"
	p, e = Build(o, env)
	if e != nil || !p.Blocked {
		t.Fatal("exhausted range accepted", p, e)
	}
	env.Devices[0].Protocols = []string{"onvif"}
	o.End = "192.0.2.10"
	p, _ = Build(o, env)
	if !p.Blocked {
		t.Fatal("ONVIF-only device accepted")
	}
}

type writerFunc func(context.Context, configuration.Request) (configuration.Result, error)

func (f writerFunc) Execute(ctx context.Context, r configuration.Request) (configuration.Result, error) {
	return f(ctx, r)
}
func TestMixedQueueNoRetryAndStableIdentityVerification(t *testing.T) {
	before := fixture("02:00:00:00:00:01", "192.0.2.100")
	network := sadp.NetworkSettings{IPv4: "192.0.2.20", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", CommandPort: 8000}
	p := Plan{Options: Options{Interface: "fixture", TimeoutSeconds: 1}, Items: []Item{}}
	for i := range 4 {
		d := before
		d.MAC = []string{"02:00:00:00:00:01", "02:00:00:00:00:02", "02:00:00:00:00:03", "02:00:00:00:00:04"}[i]
		d.Serial = d.MAC
		d.ID = d.Key()
		p.Items = append(p.Items, Item{ID: d.ID, Before: d, Proposed: network, Actions: []sadp.Action{sadp.UpdateNetwork}, State: "pending"})
	}
	calls := map[string]int{}
	current := ""
	r := Runner{Check: func(context.Context, Item, sadp.Action) error { return nil }, Credential: func(Item, sadp.Action) (vault.Secret, error) {
		return vault.Secret{Password: "Synthetic-Password123"}, nil
	}, Writer: writerFunc(func(ctx context.Context, req configuration.Request) (configuration.Result, error) {
		calls[req.MAC]++
		current = req.MAC
		switch req.MAC {
		case p.Items[1].Before.MAC:
			return configuration.Result{Sent: true, Outcome: "rejected"}, configuration.ErrRejected
		case p.Items[2].Before.MAC:
			return configuration.Result{Sent: true, Outcome: "unconfirmed"}, configuration.ErrUnconfirmed
		}
		return configuration.Result{Sent: true, Outcome: "acknowledged"}, nil
	}), Rediscover: func(context.Context, string) ([]devices.Device, error) {
		d := before
		d.MAC = current
		d.Serial = current
		d.IPv4 = network.IPv4
		dhcp := false
		d.DHCP = &dhcp
		if current == p.Items[3].Before.MAC {
			d.MAC = "02:00:00:00:99:99"
		}
		return []devices.Device{d}, nil
	}}
	result := r.Run(context.Background(), p, nil)
	for i, want := range []string{"confirmed", "rejected", "unconfirmed", "unconfirmed"} {
		if result.Items[i].State != want || calls[p.Items[i].Before.MAC] != 1 {
			t.Fatalf("item %d %+v calls %v", i, result.Items[i], calls)
		}
	}
	if result.Items[0].Verified == nil || result.Items[3].Verified != nil {
		t.Fatal("verified wrong identity")
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "Synthetic-Password123") {
		t.Fatal("secret persisted")
	}
}
func TestActivationTimeoutStopsDependentNetworkAndCancellation(t *testing.T) {
	d := fixture("02:00:00:00:00:01", "192.0.2.100")
	active := false
	d.Activated = &active
	calls := 0
	p := Plan{Options: Options{Interface: "fixture", TimeoutSeconds: 1}, Items: []Item{{Before: d, Actions: []sadp.Action{sadp.Activate, sadp.UpdateNetwork}, State: "pending"}, {Before: d, Actions: []sadp.Action{sadp.Activate}, State: "pending"}}}
	ctx, cancel := context.WithCancel(context.Background())
	r := Runner{Check: func(context.Context, Item, sadp.Action) error { return nil }, Credential: func(Item, sadp.Action) (vault.Secret, error) { return vault.Secret{Password: "Synthetic-123"}, nil }, Writer: writerFunc(func(context.Context, configuration.Request) (configuration.Result, error) {
		calls++
		cancel()
		return configuration.Result{Sent: true, Outcome: "unconfirmed"}, context.Canceled
	}), Rediscover: func(context.Context, string) ([]devices.Device, error) {
		t.Fatal("retried unconfirmed activation")
		return nil, errors.New("unexpected")
	}}
	out := r.Run(ctx, p, nil)
	if calls != 1 || out.Items[0].State != "unconfirmed" || out.Items[1].State != "skipped" {
		t.Fatal(out, calls)
	}
	_ = time.Second
}
