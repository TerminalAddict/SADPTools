package diagnostics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var dnsTarget = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?\.?$`)

type Options struct {
	Target           string   `json:"target"`
	Interface        string   `json:"interface"`
	TimeoutMS        int      `json:"timeoutMS"`
	Ports            []int    `json:"ports"`
	HTTPPorts        []int    `json:"httpPorts"`
	HTTPSPorts       []int    `json:"httpsPorts"`
	ExpectedNetworks []string `json:"expectedNetworks"`
	ICMP             bool     `json:"icmp"`
}

func (o Options) Validate() error {
	if !dnsTarget.MatchString(o.Target) || len(o.Target) > 253 || strings.Contains(o.Target, "..") {
		return errors.New("enter one IPv4 address or DNS hostname, without URL, credentials or port")
	}
	if o.TimeoutMS < 100 || o.TimeoutMS > 10000 || len(o.Ports) == 0 || len(o.Ports) > 12 {
		return errors.New("choose 1–12 ports and a 100–10000 ms probe timeout")
	}
	ports := map[int]bool{}
	for _, p := range o.Ports {
		if p < 1 || p > 65535 || ports[p] {
			return errors.New("ports must be unique numbers from 1 to 65535")
		}
		ports[p] = true
	}
	for _, p := range append(append([]int{}, o.HTTPPorts...), o.HTTPSPorts...) {
		if !ports[p] {
			return errors.New("HTTP/HTTPS ports must be included in the selected port list")
		}
	}
	return nil
}

type Network struct {
	RouteDetail func(context.Context, string, string) (platform.RouteDetail, error)
	Interfaces  func() ([]platform.Adapter, error)
	Route       func(string, int) (string, error)
	Resolver    func(context.Context, string) ([]net.IP, error)
	Dial        func(context.Context, string, string, time.Duration) (net.Conn, error)
	Probes      *ipscan.Engine
}

func NewNetwork() *Network {
	return &Network{RouteDetail: platform.DestinationRoute, Interfaces: platform.Interfaces, Route: platform.IPv4RouteSource, Resolver: func(ctx context.Context, s string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip4", s)
	}, Probes: ipscan.New(), Dial: func(ctx context.Context, dest, source string, t time.Duration) (net.Conn, error) {
		d := net.Dialer{Timeout: t}
		if source != "" {
			d.LocalAddr = &net.TCPAddr{IP: net.ParseIP(source)}
		}
		return d.DialContext(ctx, "tcp4", dest)
	}}
}
func usable(ip netip.Addr) bool {
	return ip.Is4() && !ip.IsUnspecified() && !ip.IsMulticast() && ip.String() != "255.255.255.255"
}
func (n *Network) Run(ctx context.Context, o Options, emit func(Step)) (r Run, err error) {
	r = NewRun("connectivity", o.Target)
	if err = o.Validate(); err != nil {
		return r, err
	}
	defer func() {
		r.Completed = time.Now().UTC().Format(time.RFC3339Nano)
		r.State = "completed"
		if ctx.Err() != nil {
			r.State = "cancelled"
			err = ctx.Err()
		}
	}()
	add := func(test, protocol, status, evidence, next string, start time.Time, code int) {
		s := Step{Category: "connectivity", Test: test, Protocol: protocol, Status: status, Evidence: evidence, Next: next, Code: code, DurationMS: time.Since(start).Milliseconds(), At: time.Now().UTC().Format(time.RFC3339Nano)}
		r.Steps = append(r.Steps, s)
		if emit != nil {
			emit(s)
		}
	}
	timeout := time.Duration(o.TimeoutMS) * time.Millisecond
	start := time.Now()
	ip, e := netip.ParseAddr(o.Target)
	if e != nil {
		sub, cancel := context.WithTimeout(ctx, timeout)
		ips, err := n.Resolver(sub, o.Target)
		cancel()
		if err != nil || len(ips) == 0 {
			add("DNS resolution", "dns", "failed", "No IPv4 answer; IP transport has not been tested", "Check the hostname and workstation DNS settings", start, 0)
			return r, nil
		}
		for _, v := range ips {
			if a, ok := netip.AddrFromSlice(v.To4()); ok && usable(a) {
				ip = a
				break
			}
		}
		add("DNS resolution", "dns", "verified", "Selected IPv4 "+ip.String()+"; subsequent probes stay on this address", "", start, 0)
	}
	if !usable(ip) {
		return r, errors.New("target must resolve to a usable unicast IPv4 address")
	}
	address := ip.String()
	r.Target = address
	start = time.Now()
	source, e := n.Route(address, o.Ports[0])
	adapters, ae := n.Interfaces()
	if e != nil {
		add("OS route selection", "route", "failed", "Operating system could not select a source route", "Check route table, VPN and adapter state", start, 0)
		return r, nil
	}
	var selected platform.Adapter
	for _, a := range adapters {
		for _, prefix := range a.Addresses {
			p, _ := netip.ParsePrefix(prefix)
			if p.IsValid() && p.Addr().String() == source {
				selected = a
			}
		}
	}
	if o.Interface != "" {
		found := false
		for _, a := range adapters {
			if a.Name == o.Interface && a.UnicastSuitable {
				selected = a
				source = a.Address()
				found = true
			}
		}
		if !found {
			return r, errors.New("selected IPv4 adapter unavailable")
		}
	}
	r.Interface = selected.Name
	r.Source = source
	detail := "OS-selected source " + source
	if o.Interface != "" {
		detail = "Explicit source binding " + source + "; OS routing still determines the path"
	}
	if ae == nil {
		detail += "; adapter " + selected.Name + "; prefixes " + strings.Join(selected.Addresses, ", ") + "; adapter default gateway " + selected.Gateway + " (destination next-hop not inferred)"
	}
	add("OS route selection", "route", "observed", detail, "", start, 0)
	if n.RouteDetail != nil {
		start = time.Now()
		rd, err := n.RouteDetail(ctx, address, source)
		if err != nil {
			add("Destination route", "route", "unknown", "Destination next-hop detail unavailable; source route is still known", "Inspect the OS routing table if TCP fails", start, 0)
		} else {
			gateway := rd.Gateway
			if gateway == "" {
				gateway = "on-link / no gateway"
			}
			add("Destination route", "route", "observed", rd.Evidence+"; interface "+rd.Interface+"; prefix "+rd.Prefix+"; next hop "+gateway, "", start, 0)
		}
	}

	onlink := false
	for _, s := range selected.Addresses {
		p, _ := netip.ParsePrefix(s)
		onlink = onlink || p.Contains(ip)
	}
	start = time.Now()
	neighbours, ne := n.Probes.Neighbours()
	matches := []string{}
	if onlink {
		for _, v := range neighbours {
			if v.IP == address && v.Interface == selected.Name {
				matches = append(matches, v.MAC+" (cached)")
			}
		}
	}
	switch {
	case !onlink:
		add("Neighbour evidence", "arp", "not-applicable", "Routed destination; the camera MAC is not expected in the local ARP table", "", start, 0)
	case ne != nil:
		add("Neighbour evidence", "arp", "unknown", "Neighbour cache unavailable", "ICMP/TCP probes remain useful", start, 0)
	case len(matches) == 0:
		add("Neighbour evidence", "arp", "unknown", "No cached neighbour entry; this does not prove that the address is unused", "Check cabling/VLAN; examine active TCP results", start, 0)
	default:
		add("Neighbour evidence", "arp", "observed", strings.Join(matches, ", "), "Cached information is not proof of present connectivity", start, 0)
	}
	if len(o.ExpectedNetworks) > 0 {
		match := false
		for _, v := range o.ExpectedNetworks {
			p, e := netip.ParsePrefix(v)
			if e == nil && p.Contains(ip) {
				match = true
			}
		}
		status := "observed"
		msg := "Target belongs to a recorded site network"
		if !match {
			status = "unknown"
			msg = "Target is outside recorded site networks; routing may still make it reachable"
		}
		add("Expected site network", "inventory", status, msg, "", time.Now(), 0)
	}
	if o.ICMP && ctx.Err() == nil {
		start = time.Now()
		v := n.Probes.Ping(ctx, address, source, timeout)
		status := "unknown"
		if v.State == "responded" {
			status = "verified"
		}
		if v.State == "unsupported" {
			status = "unsupported"
		}
		add("ICMP echo", "icmp", status, v.Detail, "A missing echo response does not prove the host is offline or unused", start, 0)
	}
	contains := func(list []int, p int) bool {
		for _, v := range list {
			if v == p {
				return true
			}
		}
		return false
	}
	for _, p := range o.Ports {
		if ctx.Err() != nil {
			break
		}
		start = time.Now()
		dest := net.JoinHostPort(address, strconv.Itoa(p))
		conn, e := n.Dial(ctx, dest, source, timeout)
		if e != nil {
			msg, next := connectionFailure(e)
			add(fmt.Sprintf("TCP %d", p), "tcp", "failed", msg, next, start, 0)
			continue
		}
		conn.Close()
		add(fmt.Sprintf("TCP %d", p), "tcp", "verified", "Connection accepted", "An open port alone does not identify its service", start, 0)
		if contains(o.HTTPPorts, p) || contains(o.HTTPSPorts, p) {
			start = time.Now()
			scheme := "http"
			if contains(o.HTTPSPorts, p) {
				scheme = "https"
			}
			host := o.Target
			if net.ParseIP(host) != nil {
				host = address
			}
			transport := &http.Transport{Proxy: nil, DialContext: func(c context.Context, _, _ string) (net.Conn, error) { return n.Dial(c, dest, source, timeout) }, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout}
			client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			req, requestErr := http.NewRequestWithContext(ctx, "GET", scheme+"://"+net.JoinHostPort(host, strconv.Itoa(p))+"/", nil)
			if requestErr != nil {
				transport.CloseIdleConnections()
				return r, errors.New("invalid web probe target")
			}
			resp, e := client.Do(req)
			if e != nil {
				msg, next := connectionFailure(e)
				add(fmt.Sprintf("%s %d", strings.ToUpper(scheme), p), scheme, "failed", msg, next, start, 0)
			} else {
				resp.Body.Close()
				msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
				status := "observed"
				next := ""
				if resp.StatusCode == 401 || resp.StatusCode == 403 {
					msg += " — authentication/permission required; connectivity works"
					next = "Assign the correct camera account; no default passwords were attempted"
				}
				if resp.StatusCode >= 300 && resp.StatusCode < 400 {
					msg += " — redirect refused; no credentials or follow-up requests sent"
				}
				if h := resp.Header.Get("WWW-Authenticate"); h != "" {
					for _, mechanism := range []string{"Basic", "Digest", "Bearer"} {
						if strings.Contains(strings.ToLower(h), strings.ToLower(mechanism)) {
							msg += "; advertises " + mechanism
						}
					}
				}
				if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
					cert := resp.TLS.PeerCertificates[0]
					msg += "; TLS certificate verified; expires " + cert.NotAfter.UTC().Format(time.RFC3339)
				}
				add(fmt.Sprintf("%s %d", strings.ToUpper(scheme), p), scheme, status, msg, next, start, resp.StatusCode)
			}
			transport.CloseIdleConnections()
		}
	}
	return r, nil
}
func connectionFailure(e error) (string, string) {
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var unknown x509.UnknownAuthorityError
	var ne net.Error
	switch {
	case errors.As(e, &invalid) && invalid.Reason == x509.Expired:
		return "TLS certificate expired or not yet valid", "Check camera/workstation clocks and certificate validity"
	case errors.As(e, &host):
		return "TLS certificate hostname/IP mismatch", "Verify the intended endpoint and certificate before trusting it"
	case errors.As(e, &unknown):
		return "TLS certificate authority is not trusted", "Inspect and explicitly approve the exact fingerprint in camera settings"
	case errors.Is(e, syscall.ECONNREFUSED):
		return "TCP connection refused", "Host responded; confirm service enablement and selected port"
	case errors.Is(e, syscall.EHOSTUNREACH) || errors.Is(e, syscall.ENETUNREACH):
		return "Host or network unreachable", "Check routes, VLAN and gateway reachability"
	case errors.As(e, &ne) && ne.Timeout():
		return "Connection or response timed out", "Check firewall, routing, service and device power; timeout does not prove absence"
	case errors.Is(e, context.Canceled):
		return "Cancelled", ""
	default:
		return "Connection or TLS handshake failed", "Check protocol, port, routing and certificate trust"
	}
}
