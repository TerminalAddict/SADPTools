// Package provision plans and executes explicit, serial SADP writes. Plans and
// reports contain no credentials. Unconfirmed writes are never retried.
package provision

import (
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
	"net"
	"net/netip"
	"slices"
	"time"
)

type Options struct {
	IDs              []string `json:"ids"`
	Interface        string   `json:"interface"`
	Activate         bool     `json:"activate"`
	ConfigureNetwork bool     `json:"configureNetwork"`
	DHCP             bool     `json:"dhcp"`
	Start            string   `json:"start"`
	End              string   `json:"end"`
	SubnetMask       string   `json:"subnetMask"`
	Gateway          string   `json:"gateway"`
	CommandPort      int      `json:"commandPort"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
	SaveVerified     bool     `json:"saveVerified"`
}
type Item struct {
	ID       string               `json:"id"`
	Before   devices.Device       `json:"before"`
	Proposed sadp.NetworkSettings `json:"proposed"`
	Actions  []sadp.Action        `json:"actions"`
	State    string               `json:"state"`
	Message  string               `json:"message"`
	Verified *devices.Device      `json:"verified"`
	Steps    []Step               `json:"steps"`
}
type Step struct {
	Action  sadp.Action `json:"action"`
	Outcome string      `json:"outcome"`
	Sent    bool        `json:"sent"`
	At      string      `json:"at"`
	Message string      `json:"message"`
}
type Plan struct {
	ID       string   `json:"id"`
	SiteID   string   `json:"siteID"`
	Created  string   `json:"created"`
	Expires  string   `json:"expires"`
	Options  Options  `json:"options"`
	Items    []Item   `json:"items"`
	Warnings []string `json:"warnings"`
	Blocked  bool     `json:"blocked"`
}
type Environment struct {
	Devices      []devices.Device
	Adapters     []platform.Adapter
	Known        []ipscan.Known
	Reservations map[string]string
	Scan         ipscan.Result
}

func Build(o Options, env Environment) (Plan, error) {
	p := Plan{Options: o, Items: []Item{}, Warnings: []string{"No response never proves an address is free. Targets are checked again before writes.", "Commands use the existing SADP firmware-dependent protocol. There is no batch rollback; cancellation cannot undo a sent request."}, Created: time.Now().UTC().Format(time.RFC3339Nano), Expires: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339Nano)}
	if len(o.IDs) == 0 || len(o.IDs) > 64 || !o.Activate && !o.ConfigureNetwork {
		return p, errors.New("select 1–64 devices and an operation")
	}
	if o.TimeoutSeconds < 1 || o.TimeoutSeconds > 300 {
		return p, errors.New("write timeout must be 1–300 seconds")
	}
	adapters, e := platform.Select(env.Adapters, o.Interface)
	if e != nil || len(adapters) != 1 || o.Interface == "" {
		return p, errors.New("select exactly one available multicast adapter")
	}
	excluded := map[string][]string{}
	for ip, note := range env.Reservations {
		excluded[ip] = []string{"reserved: " + note}
	}
	for _, a := range env.Adapters {
		for _, raw := range a.Addresses {
			if prefix, e := netip.ParsePrefix(raw); e == nil {
				excluded[prefix.Addr().String()] = []string{"local adapter address"}
			}
		}
		if a.Gateway != "" {
			excluded[a.Gateway] = []string{"adapter gateway"}
		}
	}
	if o.Gateway != "" && o.Gateway != "0.0.0.0" {
		excluded[o.Gateway] = []string{"planned gateway"}
	}
	occupied := map[string][]string{}
	for _, d := range env.Devices {
		occupied[d.IPv4] = append(occupied[d.IPv4], devices.NormalizeMAC(d.MAC))
	}
	for _, k := range env.Known {
		occupied[k.IP] = append(occupied[k.IP], devices.NormalizeMAC(k.MAC))
	}
	for _, h := range env.Scan.Hosts {
		if h.Reserved != "" {
			excluded[h.IP] = []string{h.Reserved}
		}
		if h.Conflict {
			excluded[h.IP] = []string{"scanner identity conflict"}
		}
		if h.Responding || h.Known {
			if len(h.MACs) == 0 {
				occupied[h.IP] = append(occupied[h.IP], "")
			} else {
				occupied[h.IP] = append(occupied[h.IP], h.MACs...)
			}
		}
	}
	var next, end netip.Addr
	var prefix netip.Prefix
	if o.ConfigureNetwork {
		next, e = netip.ParseAddr(o.Start)
		if e != nil || !next.Is4() {
			return p, errors.New("enter a starting IPv4 address")
		}
		end, e = netip.ParseAddr(o.End)
		if e != nil || !end.Is4() || end.Less(next) {
			return p, errors.New("enter an ending IPv4 address at or after start")
		}
		mask := net.ParseIP(o.SubnetMask).To4()
		if mask == nil {
			return p, errors.New("invalid subnet mask")
		}
		ones, bits := net.IPMask(mask).Size()
		if bits != 32 || ones < 1 || ones > 30 {
			return p, errors.New("existing SADP engine supports subnet masks /1 through /30")
		}
		prefix = netip.PrefixFrom(next, ones).Masked()
		if !prefix.Contains(end) {
			return p, errors.New("allocation range must stay in one configured subnet")
		}
		if next.IsMulticast() || next.IsLoopback() || next.IsUnspecified() || end.IsMulticast() {
			return p, errors.New("unusable address range")
		}
	}
	seen := map[string]bool{}
	skipped := 0
	for _, id := range o.IDs {
		if seen[id] {
			return p, errors.New("select each device once")
		}
		seen[id] = true
		item := Item{ID: id, State: "pending", Actions: []sadp.Action{}, Steps: []Step{}}
		found := false
		for _, d := range env.Devices {
			if d.ID == id {
				item.Before = d
				found = true
				break
			}
		}
		d := item.Before
		invalid := ""
		switch {
		case !found:
			invalid = "Device is not in live discovery; rediscover saved devices before provisioning"
		case !d.SADPCompatible() || !slices.Contains(d.Protocols, "sadp") || d.MAC == "":
			invalid = "Requires an unambiguous SADP identity and MAC"
		case d.Activated == nil:
			invalid = "Activation status is unknown; use the single-device workflow"
		default:
			route := false
			for _, a := range d.Interfaces {
				if a.Name == o.Interface {
					route = true
				}
			}
			if !route {
				invalid = "Device was not discovered on the selected adapter"
			}
		}
		if invalid == "" {
			if !*d.Activated {
				if !o.Activate {
					invalid = "Inactive device: include activation first"
				} else {
					item.Actions = append(item.Actions, sadp.Activate)
				}
			}
		}
		if invalid == "" && o.ConfigureNetwork {
			port := o.CommandPort
			if port == 0 && d.CommandPort != nil {
				port = *d.CommandPort
			}
			if port < 1 || port > 65535 {
				invalid = "SDK port is unknown; explicitly provide it"
			} else {
				allocated := false
				attempts := 0
				for next.IsValid() && !end.Less(next) && attempts < 65536 {
					candidate := next
					next = next.Next()
					attempts++
					n := sadp.NetworkSettings{IPv4: candidate.String(), SubnetMask: o.SubnetMask, Gateway: o.Gateway, CommandPort: port, DHCP: o.DHCP}
					if e := configuration.ValidateNetwork(n); e != nil {
						skipped++
						continue
					}
					if _, blocked := excluded[n.IPv4]; blocked {
						skipped++
						continue
					}
					blocked := false
					for _, mac := range occupied[n.IPv4] {
						if mac == "" || mac != d.MAC {
							blocked = true
						}
					}
					if blocked {
						skipped++
						continue
					}
					item.Proposed = n
					item.Actions = append(item.Actions, sadp.UpdateNetwork)
					occupied[n.IPv4] = []string{"allocated by this plan"}
					allocated = true
					break
				}
				if !allocated {
					invalid = "No suitable address remains in the selected range"
				}
			}
		}
		if invalid != "" {
			item.State = "skipped"
			item.Message = invalid
			p.Blocked = true
		} else if len(item.Actions) == 0 {
			item.State = "skipped"
			item.Message = "Already active; no network change requested"
		}
		p.Items = append(p.Items, item)
	}
	if skipped > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("Skipped %d protected, occupied or invalid candidate addresses.", skipped))
	}
	if o.DHCP && o.ConfigureNetwork {
		p.Warnings = append(p.Warnings, "The assigned static address is a fallback when DHCP is enabled. Verification requires observed DHCP=true; the leased address may differ.")
	}
	return p, nil
}
