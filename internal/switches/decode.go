package switches

import (
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/gosnmp/gosnmp"
	"net"
	"sort"
	"strconv"
	"strings"
)

func stringValue(p gosnmp.SnmpPDU) string {
	var s string
	switch v := p.Value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return ""
	}
	if len(s) > 1024 {
		s = s[:1024]
	}
	return strings.ToValidUTF8(s, "�")
}
func numberValue(p gosnmp.SnmpPDU) int {
	switch p.Value.(type) {
	case int, int32, int64, uint, uint32, uint64, uint16, byte:
		v := gosnmp.ToBigInt(p.Value)
		if v.IsInt64() {
			n := v.Int64()
			if n >= 0 && n <= 1<<53 {
				return int(n)
			}
		}
	}
	return -1
}
func indices(name, root string) []int {
	if !strings.HasPrefix(name, root+".") {
		return nil
	}
	out := []int{}
	for _, s := range strings.Split(strings.TrimPrefix(name, root+"."), ".") {
		n, e := strconv.Atoi(s)
		if e != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}
func suffix(i []int) string {
	a := []string{}
	for _, v := range i {
		a = append(a, strconv.Itoa(v))
	}
	return strings.Join(a, ".")
}
func column(rows map[string]gosnmp.SnmpPDU, root string) map[string]gosnmp.SnmpPDU {
	out := map[string]gosnmp.SnmpPDU{}
	for k, v := range rows {
		if i := indices(k, root); len(i) > 0 {
			out[suffix(i)] = v
		}
	}
	return out
}
func lookup(rows map[string]gosnmp.SnmpPDU, root, key string) gosnmp.SnmpPDU {
	return rows[root+"."+key]
}
func integers(s string) []int {
	out := []int{}
	for _, v := range strings.Split(s, ".") {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}
func macBytes(value any) string {
	b, ok := value.([]byte)
	if !ok || len(b) != 6 {
		return ""
	}
	return devices.NormalizeMAC(net.HardwareAddr(b).String())
}
func macIndex(i []int) string {
	if len(i) != 6 {
		return ""
	}
	b := make([]byte, 6)
	for n, v := range i {
		if v < 0 || v > 255 {
			return ""
		}
		b[n] = byte(v)
	}
	return macBytes(b)
}
func state(n int) string {
	switch n {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	case 4:
		return "unknown"
	case 5:
		return "dormant"
	case 6:
		return "not-present"
	case 7:
		return "lower-layer-down"
	}
	return "unknown"
}
func fdbState(n int) string {
	switch n {
	case 1:
		return "other"
	case 2:
		return "invalid"
	case 3:
		return "learned"
	case 4:
		return "self"
	case 5:
		return "management"
	}
	return "unknown"
}
func poeState(n int) string {
	switch n {
	case 1:
		return "disabled"
	case 2:
		return "searching"
	case 3:
		return "delivering power"
	case 4:
		return "fault"
	case 5:
		return "test"
	case 6:
		return "other fault"
	}
	return "unknown/unsupported"
}
func unique(values []int, n int) []int {
	for _, v := range values {
		if v == n {
			return values
		}
	}
	return append(values, n)
}
func Decode(a Asset, r Report, rows map[string]gosnmp.SnmpPDU, targets []devices.Device) Report {
	r.Ports = []Port{}
	r.FDB = []FDB{}
	r.Neighbours = []Neighbour{}
	r.Power = []Power{}
	r.Mappings = []Mapping{}
	r.Warnings = []string{}
	ports := map[int]*Port{}
	port := func(index int) *Port {
		if p := ports[index]; p != nil {
			return p
		}
		p := &Port{Index: index, Admin: "unknown", Operational: "unknown", PoE: "unknown/unsupported", VLANs: []int{}, UplinkEvidence: []string{}}
		ports[index] = p
		return p
	}
	for _, root := range []string{ifEntry + ".2", ifXEntry + ".1"} {
		for key := range column(rows, root) {
			i := integers(key)
			if len(i) != 1 || i[0] < 1 {
				continue
			}
			p := port(i[0])
			p.Description = stringValue(lookup(rows, ifEntry+".2", key))
			p.Name = stringValue(lookup(rows, ifXEntry+".1", key))
			if p.Name == "" {
				p.Name = p.Description
			}
			p.Alias = stringValue(lookup(rows, ifXEntry+".18", key))
			p.MAC = macBytes(lookup(rows, ifEntry+".6", key).Value)
			p.Admin = state(numberValue(lookup(rows, ifEntry+".7", key)))
			p.Operational = state(numberValue(lookup(rows, ifEntry+".8", key)))
			speed := numberValue(lookup(rows, ifXEntry+".15", key))
			if speed > 0 {
				v := float64(speed)
				p.SpeedMbps = &v
			} else {
				n := numberValue(lookup(rows, ifEntry+".5", key))
				if n > 0 && n < 4294967295 {
					v := float64(n) / 1000000
					p.SpeedMbps = &v
				}
			}
		}
	}
	bridge := map[int]int{}
	for k, v := range column(rows, bridgePorts) {
		idx := integers(k)
		n := numberValue(v)
		if len(idx) == 1 && n > 0 {
			bridge[idx[0]] = n
		}
	}
	fdbVLAN := map[int][]int{}
	for k, v := range column(rows, vlanEntry+".3") {
		idx := integers(k)
		n := numberValue(v)
		if len(idx) == 2 && n >= 0 && idx[1] >= 1 && idx[1] <= 4094 {
			fdbVLAN[n] = unique(fdbVLAN[n], idx[1])
		}
	}
	for k, v := range column(rows, vlanEntry+".4") {
		idx := integers(k)
		bits, ok := v.Value.([]byte)
		if len(idx) != 2 || !ok || idx[1] < 1 || idx[1] > 4094 {
			continue
		}
		for bp, ifIndex := range bridge {
			bit := bp - 1
			if bit >= 0 && bit/8 < len(bits) && bits[bit/8]&(1<<uint(7-bit%8)) != 0 {
				p := port(ifIndex)
				p.VLANs = unique(p.VLANs, idx[1])
			}
		}
	}
	for _, root := range []string{fdbEntry, qfdbEntry} {
		for key, v := range column(rows, root+".2") {
			idx := integers(key)
			fid := 0
			if root == qfdbEntry {
				if len(idx) != 7 {
					continue
				}
				fid = idx[0]
				idx = idx[1:]
			}
			mac := macIndex(idx)
			bp := numberValue(v)
			if mac == "" || bp < 0 {
				continue
			}
			r.FDB = append(r.FDB, FDB{MAC: mac, BridgePort: bp, IfIndex: bridge[bp], FDBID: fid, VLANs: append([]int{}, fdbVLAN[fid]...), State: fdbState(numberValue(lookup(rows, root+".3", key))), Source: root + ".2." + key})
		}
	}
	localMap := map[int]int{}
	for key, v := range column(rows, lldpLocal+".3") {
		idx := integers(key)
		if len(idx) != 1 {
			continue
		}
		subtype := numberValue(lookup(rows, lldpLocal+".2", key))
		value := stringValue(v)
		desc := stringValue(lookup(rows, lldpLocal+".4", key))
		matches := []int{}
		for index, p := range ports {
			match := subtype == 5 && value != "" && value == p.Name || subtype == 1 && value != "" && value == p.Alias || subtype == 3 && p.MAC != "" && macBytes(v.Value) == p.MAC || desc != "" && desc == p.Description
			if match {
				matches = append(matches, index)
			}
		}
		if len(matches) == 1 {
			localMap[idx[0]] = matches[0]
		}
	}
	for _, link := range a.PortLinks {
		if link.LLDPLocal > 0 {
			localMap[link.LLDPLocal] = link.IfIndex
		}
	}
	for key, v := range column(rows, lldpRemote+".5") {
		idx := integers(key)
		if len(idx) != 3 {
			continue
		}
		n := Neighbour{LocalPort: idx[1], IfIndex: localMap[idx[1]], Name: stringValue(lookup(rows, lldpRemote+".9", key)), PortID: stringValue(lookup(rows, lldpRemote+".7", key))}
		if numberValue(lookup(rows, lldpRemote+".4", key)) == 4 {
			n.ChassisMAC = macBytes(v.Value)
		}
		if caps, ok := lookup(rows, lldpRemote+".12", key).Value.([]byte); ok && len(caps) > 0 {
			n.Infrastructure = caps[0]&0x78 != 0
		}
		r.Neighbours = append(r.Neighbours, n)
		if n.Infrastructure && n.IfIndex > 0 {
			p := port(n.IfIndex)
			p.Uplink = true
			p.UplinkEvidence = append(p.UplinkEvidence, "LLDP neighbour advertises bridge/router/AP/repeater capabilities")
		}
	}
	physicalIf := map[int]int{}
	for key, v := range column(rows, entityAlias) {
		idx := integers(key)
		value := stringValue(v)
		mapped := indices(value, ifEntry+".1")
		if len(idx) == 2 && len(mapped) == 1 {
			if previous := physicalIf[idx[0]]; previous != 0 && previous != mapped[0] {
				physicalIf[idx[0]] = -1
			} else {
				physicalIf[idx[0]] = mapped[0]
			}
		}
	}
	powers := map[string]*Power{}
	for _, root := range []string{poeStatus, ciscoPower} {
		for key := range column(rows, root) {
			idx := integers(key)
			if len(idx) != 2 {
				continue
			}
			p := powers[key]
			if p == nil {
				p = &Power{Group: idx[0], Port: idx[1], State: poeState(numberValue(lookup(rows, poeStatus, key))), Source: poeStatus + "." + key}
				powers[key] = p
			}
			mw := numberValue(lookup(rows, ciscoPower, key))
			if mw >= 0 {
				watts := float64(mw) / 1000
				p.Watts = &watts
				p.Source += "; " + ciscoPower + "." + key + " (mW)"
			}
			physical := numberValue(lookup(rows, ciscoPhysical, key))
			if physicalIf[physical] > 0 {
				p.IfIndex = physicalIf[physical]
				p.Source += "; Cisco physical index → ENTITY-MIB ifIndex"
			}
		}
	}
	for _, link := range a.PortLinks {
		if p := powers[fmt.Sprintf("%d.%d", link.PoEGroup, link.PoEPort)]; p != nil {
			p.IfIndex = link.IfIndex
			p.Source += "; operator-confirmed PoE port mapping"
		}
	}
	for _, p := range powers {
		r.Power = append(r.Power, *p)
		if p.IfIndex > 0 {
			dst := port(p.IfIndex)
			dst.PoE = p.State
			dst.PowerWatts = p.Watts
			dst.PoESource = p.Source
		}
	}
	for _, index := range a.Uplinks {
		p := port(index)
		p.Uplink = true
		p.UplinkEvidence = append(p.UplinkEvidence, "Operator marked this interface as an uplink")
	}
	for _, p := range ports {
		sort.Ints(p.VLANs)
		if len(p.VLANs) > 1 {
			p.Uplink = true
			p.UplinkEvidence = append(p.UplinkEvidence, "Interface is a member of multiple VLANs; possible trunk")
		}
		r.Ports = append(r.Ports, *p)
	}
	sort.Slice(r.Ports, func(i, j int) bool { return r.Ports[i].Index < r.Ports[j].Index })
	sort.Slice(r.Power, func(i, j int) bool {
		if r.Power[i].Group == r.Power[j].Group {
			return r.Power[i].Port < r.Power[j].Port
		}
		return r.Power[i].Group < r.Power[j].Group
	})
	sort.Slice(r.FDB, func(i, j int) bool { return r.FDB[i].Source < r.FDB[j].Source })
	r.Mappings = Correlate(a, r, targets)
	if len(r.Ports) == 0 {
		r.Warnings = append(r.Warnings, "IF-MIB returned no usable interfaces")
	}
	if len(r.Power) == 0 {
		r.Warnings = append(r.Warnings, "PoE status/power unknown or unsupported; no zero-watt value inferred")
	}
	return r
}
func Correlate(a Asset, r Report, targets []devices.Device) []Mapping {
	out := []Mapping{}
	ports := map[int]Port{}
	for _, p := range r.Ports {
		ports[p.Index] = p
	}
	portMACs := map[int]map[string]bool{}
	for _, f := range r.FDB {
		if f.State == "invalid" || f.State == "self" {
			continue
		}
		if portMACs[f.IfIndex] == nil {
			portMACs[f.IfIndex] = map[string]bool{}
		}
		portMACs[f.IfIndex][f.MAC] = true
	}
	for _, d := range targets {
		mac := devices.NormalizeMAC(d.MAC)
		if mac == "" {
			continue
		}
		found := map[int][]FDB{}
		for _, f := range r.FDB {
			if f.MAC == mac && f.State != "invalid" && f.State != "self" {
				found[f.IfIndex] = append(found[f.IfIndex], f)
			}
		}
		if len(found) == 0 {
			out = append(out, Mapping{DeviceID: d.ID, IP: d.IPv4, MAC: mac, SwitchID: a.ID, SwitchName: a.Name, Confidence: "unknown", Evidence: []string{"No matching usable forwarding-table entry in this poll"}, At: r.At, PoE: "unknown/unsupported", VLANs: []int{}})
			continue
		}
		for index, entries := range found {
			p, known := ports[index]
			m := Mapping{DeviceID: d.ID, IP: d.IPv4, MAC: mac, SwitchID: a.ID, SwitchName: a.Name, Port: index, PortName: p.Name, Confidence: "likely", Evidence: []string{"FDB snapshot observed at poll time; individual MAC-entry age is unknown and may be stale"}, VLANs: []int{}, At: r.At, Link: p.Operational, SpeedMbps: p.SpeedMbps, PoE: p.PoE, PowerWatts: p.PowerWatts}
			learned := false
			for _, f := range entries {
				m.Evidence = append(m.Evidence, f.Source+" state="+f.State)
				if f.State == "learned" {
					learned = true
				}
				for _, v := range f.VLANs {
					m.VLANs = unique(m.VLANs, v)
				}
			}
			switch {
			case !known || index == 0:
				m.Confidence = "unknown"
				m.Evidence = append(m.Evidence, "Bridge port could not be mapped to IF-MIB; no physical interface inferred")
			case len(found) > 1 || p.Uplink || len(portMACs[index]) > 1 || d.IdentityConflict:
				m.Confidence = "ambiguous"
				m.Evidence = append(m.Evidence, "Multiple ports/MACs, conflicting identity or uplink/trunk evidence prevents a direct-port claim")
				m.Evidence = append(m.Evidence, p.UplinkEvidence...)
			case p.Operational != "up" || !learned:
				m.Confidence = "ambiguous"
				m.Evidence = append(m.Evidence, "Port is not observed up or FDB state is not learned")
			default:
				for _, n := range r.Neighbours {
					if n.IfIndex == index && n.ChassisMAC == mac && !n.Infrastructure {
						m.Confidence = "direct/strong"
						m.Evidence = append(m.Evidence, "Matching LLDP chassis MAC and a single learned MAC on an operational interface")
					}
				}
			}
			sort.Ints(m.VLANs)
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MAC == out[j].MAC {
			return out[i].Port < out[j].Port
		}
		return out[i].MAC < out[j].MAC
	})
	return out
}
