package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
)

func (c *Controller) Export(ids []string, format string) ([]byte, error) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	rows := []devices.Device{}
	for _, d := range c.inventory.Snapshot() {
		if selected[d.ID] {
			rows = append(rows, d)
			delete(selected, d.ID)
		}
	}
	if len(selected) > 0 {
		return nil, fmt.Errorf("an exported device is no longer in the inventory")
	}
	switch format {
	case "json":
		return json.MarshalIndent(rows, "", "  ")
	case "csv":
		var b bytes.Buffer
		w := csv.NewWriter(&b)
		if err := w.Write([]string{"IP address", "Model", "Device type", "MAC", "Serial number", "Firmware", "Activated", "DHCP", "HTTP port", "Subnet mask", "Gateway", "Device name", "Interfaces", "First seen", "Last seen"}); err != nil {
			return nil, err
		}
		boolean := func(v *bool) string {
			if v == nil {
				return ""
			}
			return strconv.FormatBool(*v)
		}
		for _, d := range rows {
			interfaces := []string{}
			for _, a := range d.Interfaces {
				interfaces = append(interfaces, a.Name+" ("+a.Address+")")
			}
			port := ""
			if d.HTTPPort != nil {
				port = strconv.Itoa(*d.HTTPPort)
			}
			record := []string{d.IPv4, d.Model, d.DeviceType, d.MAC, d.Serial, d.Firmware, boolean(d.Activated), boolean(d.DHCP), port, d.SubnetMask, d.Gateway, d.Name, strings.Join(interfaces, "; "), d.FirstSeen.Format(time.RFC3339), d.LastSeen.Format(time.RFC3339)}
			for i, v := range record {
				record[i] = csvCell(v)
			}
			if err := w.Write(record); err != nil {
				return nil, err
			}
		}
		w.Flush()
		return b.Bytes(), w.Error()
	default:
		return nil, fmt.Errorf("export format must be csv or json")
	}
}
func csvCell(s string) string {
	if trimmed := strings.TrimLeft(s, " \t\r\n"); trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + s
	}
	return s
}
