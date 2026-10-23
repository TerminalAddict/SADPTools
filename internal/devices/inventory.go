package devices

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Inventory struct {
	mu    sync.RWMutex
	items map[string]Device
}

func NewInventory() *Inventory { return &Inventory{items: make(map[string]Device)} }

func clone(d Device) Device {
	d.Interfaces = append([]Interface{}, d.Interfaces...)
	d.Protocols = append([]string{}, d.Protocols...)
	d.Capabilities = append([]string{}, d.Capabilities...)
	d.XAddrs = append([]string{}, d.XAddrs...)
	if d.HTTPSPort != nil {
		v := *d.HTTPSPort
		d.HTTPSPort = &v
	}
	if d.RTSPPort != nil {
		v := *d.RTSPPort
		d.RTSPPort = &v
	}
	if d.Activated != nil {
		v := *d.Activated
		d.Activated = &v
	}
	if d.DHCP != nil {
		v := *d.DHCP
		d.DHCP = &v
	}
	if d.HTTPPort != nil {
		v := *d.HTTPPort
		d.HTTPPort = &v
	}
	if d.CommandPort != nil {
		v := *d.CommandPort
		d.CommandPort = &v
	}
	return d
}

// Upsert preserves absent fields and promotes serial-only identities when a MAC
// becomes available. Distinct MACs are never merged.
func (i *Inventory) Upsert(d Device, now time.Time) (Device, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	d = clone(d)
	d.MAC = NormalizeMAC(d.MAC)
	key := d.Key()
	if key == "" {
		return Device{}, false
	}

	if old, ok := i.items[key]; ok && ConflictingIdentity(old, d) {
		old.IdentityConflict = true
		i.items[key] = old
		d.IdentityConflict = true
		hash := sha256.Sum256([]byte(d.MAC + "\x00" + d.Serial + "\x00" + d.UUID + "\x00" + d.Vendor))
		key += fmt.Sprintf(":conflict:%x", hash[:8])
	}
	// Join additional serial/UUID observations only when all known identities agree.
	// This promotes an ONVIF UUID after an authenticated MAC read and never uses IP.
	candidates := []string{}
	for k, old := range i.items {
		shared := d.MAC != "" && d.MAC == old.MAC || d.Serial != "" && d.Serial == old.Serial || d.UUID != "" && d.UUID == old.UUID
		if shared && !ConflictingIdentity(old, d) {
			candidates = append(candidates, k)
		}
	}
	compatible := true
	for _, a := range candidates {
		for _, b := range candidates {
			if ConflictingIdentity(i.items[a], i.items[b]) {
				compatible = false
			}
		}
	}
	if compatible && len(candidates) > 0 {
		sort.Slice(candidates, func(a, b int) bool { return i.items[candidates[a]].LastSeen.Before(i.items[candidates[b]].LastSeen) })
		combined := Device{}
		for _, k := range candidates {
			old := i.items[k]
			first := combined.FirstSeen
			combined = mergeFields(combined, old)
			if first.IsZero() || old.FirstSeen.Before(first) {
				first = old.FirstSeen
			}
			combined.FirstSeen = first
			if old.LastSeen.After(combined.LastSeen) {
				combined.LastSeen = old.LastSeen
			}
			delete(i.items, k)
		}
		if d.MAC == "" && combined.MAC != "" {
			key = combined.Key()
		}
		if d.MAC == "" && d.Serial == "" && combined.Serial != "" {
			key = combined.Key()
		}
		i.items[key] = combined
	} else if !compatible {
		d.IdentityConflict = true
	}

	old, exists := i.items[key]
	old = mergeFields(old, d)
	old.ID = key
	if old.FirstSeen.IsZero() {
		old.FirstSeen = now
	}
	old.LastSeen = now
	i.items[key] = old
	return clone(old), !exists
}

func (i *Inventory) Snapshot() []Device {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make([]Device, 0, len(i.items))
	for _, d := range i.items {
		result = append(result, clone(d))
	}
	sort.Slice(result, func(a, b int) bool { return result[a].ID < result[b].ID })
	return result
}

// mergeFields prefers observed nonempty fields while retaining every adapter sighting.
func mergeFields(old, d Device) Device {
	old.IdentityConflict = old.IdentityConflict || d.IdentityConflict
	merge := func(dst *string, src string) {
		if src != "" {
			*dst = src
		}
	}
	merge(&old.Model, d.Model)
	merge(&old.UUID, d.UUID)
	merge(&old.Vendor, d.Vendor)
	union := func(a, b []string) []string {
		for _, s := range b {
			found := false
			for _, x := range a {
				if x == s {
					found = true
					break
				}
			}
			if !found {
				a = append(a, s)
			}
		}
		return a
	}
	old.Protocols = union(old.Protocols, d.Protocols)
	old.Capabilities = union(old.Capabilities, d.Capabilities)
	old.XAddrs = union(old.XAddrs, d.XAddrs)
	if d.HTTPSPort != nil {
		old.HTTPSPort = d.HTTPSPort
	}
	if d.RTSPPort != nil {
		old.RTSPPort = d.RTSPPort
	}
	merge(&old.DeviceType, d.DeviceType)
	merge(&old.IPv4, d.IPv4)
	merge(&old.SubnetMask, d.SubnetMask)
	merge(&old.Gateway, d.Gateway)
	merge(&old.MAC, d.MAC)
	merge(&old.Serial, d.Serial)
	merge(&old.Firmware, d.Firmware)
	merge(&old.Name, d.Name)
	merge(&old.PasswordResetAbility, d.PasswordResetAbility)
	merge(&old.PasswordResetMode, d.PasswordResetMode)
	if d.Activated != nil {
		old.Activated = d.Activated
	}
	if d.DHCP != nil {
		old.DHCP = d.DHCP
	}
	if d.HTTPPort != nil {
		old.HTTPPort = d.HTTPPort
	}
	if d.CommandPort != nil {
		old.CommandPort = d.CommandPort
	}
	for _, found := range d.Interfaces {
		seen := false
		for _, previous := range old.Interfaces {
			if found == previous {
				seen = true
				break
			}
		}
		if !seen {
			old.Interfaces = append(old.Interfaces, found)
		}
	}
	sort.Slice(old.Interfaces, func(a, b int) bool {
		if old.Interfaces[a].Index != old.Interfaces[b].Index {
			return old.Interfaces[a].Index < old.Interfaces[b].Index
		}
		return old.Interfaces[a].Address < old.Interfaces[b].Address
	})
	return old
}
