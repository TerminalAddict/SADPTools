//go:build linux

package ipscan

import (
	"bufio"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"os"
	"strings"
)

func neighbours() ([]Neighbour, error) {
	f, e := os.Open("/proc/net/arp")
	if e != nil {
		return nil, e
	}
	defer f.Close()
	out := []Neighbour{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		v := strings.Fields(s.Text())
		if len(v) < 6 || v[2] != "0x2" || devices.NormalizeMAC(v[3]) == "" {
			continue
		}
		out = append(out, Neighbour{IP: v[0], MAC: devices.NormalizeMAC(v[3]), Interface: v[5], Source: "ARP cache (age unknown)"})
	}
	return out, s.Err()
}
