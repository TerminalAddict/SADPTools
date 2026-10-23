//go:build linux

package platform

import (
	"bufio"
	"encoding/binary"
	"net"
	"os"
	"strconv"
	"strings"
)

func gateway(name string) string {
	f, e := os.Open("/proc/net/route")
	if e != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		v := strings.Fields(s.Text())
		if len(v) < 4 || v[0] != name || v[1] != "00000000" {
			continue
		}
		n, e := strconv.ParseUint(v[2], 16, 32)
		if e != nil || n == 0 {
			continue
		}
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(n))
		return net.IP(b[:]).String()
	}
	return ""
}
