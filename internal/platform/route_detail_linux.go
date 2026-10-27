//go:build linux

package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os/exec"
	"time"
)

func DestinationRoute(ctx context.Context, address, source string) (RouteDetail, error) {
	if ip, e := netip.ParseAddr(address); e != nil || !ip.Is4() {
		return RouteDetail{}, errors.New("invalid route target")
	}
	if ip, e := netip.ParseAddr(source); e != nil || !ip.Is4() {
		return RouteDetail{}, errors.New("invalid route source")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	data, e := exec.CommandContext(ctx, "ip", "-j", "route", "get", address, "from", source).Output()
	if e != nil || len(data) > 64<<10 {
		return RouteDetail{}, errors.New("destination route detail unavailable; iproute2/permissions may be unavailable")
	}
	var rows []struct {
		Dst     string `json:"dst"`
		Gateway string `json:"gateway"`
		Dev     string `json:"dev"`
		Prefsrc string `json:"prefsrc"`
	}
	if json.Unmarshal(data, &rows) != nil || len(rows) != 1 {
		return RouteDetail{}, errors.New("ambiguous route details")
	}
	r := rows[0]
	return RouteDetail{Source: source, Interface: r.Dev, Gateway: r.Gateway, Prefix: r.Dst, Evidence: "Linux ip route get for destination and source"}, nil
}
