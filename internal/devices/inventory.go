package devices

import (
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
	if d.Serial != "" {
		if d.MAC != "" {
			serialKey := "serial:" + d.Serial
			if old, ok := i.items[serialKey]; ok {
				if current, exists := i.items[key]; exists {
					first := old.FirstSeen
					if current.FirstSeen.Before(first) {
						first = current.FirstSeen
					}
					if current.LastSeen.After(old.LastSeen) {
						old = mergeFields(old, current)
					} else {
						old = mergeFields(current, old)
					}
					old.FirstSeen = first
				}
				i.items[key] = old
				delete(i.items, serialKey)
			}
		} else {
			match := ""
			for k, old := range i.items {
				if old.Serial == d.Serial && old.MAC != "" {
					if match != "" {
						match = ""
						break
					}
					match = k
				}
			}
			if match != "" {
				key = match
			}
		}
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
	merge := func(dst *string, src string) {
		if src != "" {
			*dst = src
		}
	}
	merge(&old.Model, d.Model)
	merge(&old.DeviceType, d.DeviceType)
	merge(&old.IPv4, d.IPv4)
	merge(&old.SubnetMask, d.SubnetMask)
	merge(&old.Gateway, d.Gateway)
	merge(&old.MAC, d.MAC)
	merge(&old.Serial, d.Serial)
	merge(&old.Firmware, d.Firmware)
	merge(&old.Name, d.Name)
	if d.Activated != nil {
		old.Activated = d.Activated
	}
	if d.DHCP != nil {
		old.DHCP = d.DHCP
	}
	if d.HTTPPort != nil {
		old.HTTPPort = d.HTTPPort
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
