// Package audit performs bounded, read-only checks. Saved configuration and
// fresh network evidence are labelled separately; unknown is never a clean pass.
package audit

import (
	"context"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type Finding struct {
	RecordID string `json:"recordID"`
	IP       string `json:"ip"`
	Model    string `json:"model"`
	MAC      string `json:"mac"`
	Check    string `json:"check"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Source   string `json:"source"`
	At       string `json:"at"`
}
type Report struct {
	SiteID    string    `json:"siteID"`
	SiteName  string    `json:"siteName"`
	Started   string    `json:"started"`
	Finished  string    `json:"finished"`
	Done      int       `json:"done"`
	Total     int       `json:"total"`
	Cancelled bool      `json:"cancelled"`
	Findings  []Finding `json:"findings"`
}
type Remote struct {
	Device        devices.Device
	Configuration onvif.Configuration
	Warnings      []string
}
type Engine struct {
	TCP    func(context.Context, string, int) (bool, string)
	Web    func(context.Context, devices.Device) (webinterface.Result, error)
	Read   func(context.Context, devices.Device, vault.Secret, string) (Remote, error)
	Secret func(sites.Record) (vault.Secret, string, error)
}

func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func New() *Engine {
	return &Engine{TCP: tcp, Web: func(ctx context.Context, d devices.Device) (webinterface.Result, error) {
		port := 0
		if d.HTTPPort != nil {
			port = *d.HTTPPort
		}
		source, _ := platform.IPv4RouteSource(d.IPv4, 80)
		target := webinterface.Target{Address: d.IPv4, Port: port, Source: source}
		if d.HTTPSPort != nil {
			target.HTTPSPort = *d.HTTPSPort
		}
		return webinterface.New().Detect(ctx, target)
	}, Read: readONVIF}
}
func tcp(ctx context.Context, address string, port int) (bool, string) {
	dialer := net.Dialer{Timeout: 800 * time.Millisecond}
	c, e := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(address, strconv.Itoa(port)))
	if e != nil {
		return false, "No accepted TCP connection; firewall, service configuration or routing may be responsible"
	}
	c.Close()
	return true, "TCP connection accepted; this does not establish application health"
}
func readONVIF(ctx context.Context, d devices.Device, secret vault.Secret, pin string) (Remote, error) {
	out := Remote{}
	endpoint := ""
	for _, raw := range d.XAddrs {
		if onvif.ValidXAddr(raw, d.IPv4) {
			if endpoint == "" {
				endpoint = raw
			}
			if strings.HasPrefix(raw, "https://") {
				endpoint = raw
				break
			}
		}
	}
	if endpoint == "" {
		return out, errors.New("no usable ONVIF service discovered; enable ONVIF discovery for authenticated checks")
	}
	source, _ := platform.IPv4RouteSource(d.IPv4, 80)
	client, e := onvif.NewClient(onvif.Target{Address: d.IPv4, URL: endpoint, Source: source, CertificateSHA256: pin})
	if e != nil {
		return out, e
	}
	info, e := client.Information(ctx, d, secret)
	if e != nil {
		return out, e
	}
	out.Device = info.Device
	out.Warnings = info.Warnings
	out.Configuration = client.Configuration(ctx, secret)
	return out, nil
}
func (e *Engine) Run(ctx context.Context, site sites.Site, records, all []sites.Record, pins map[string]string, progress func(Report)) Report {
	report := Report{SiteID: site.ID, SiteName: site.Name, Started: stamp(), Total: len(records), Findings: []Finding{}}
	appendFinding := func(r sites.Record, check, status, evidence, source string) {
		report.Findings = append(report.Findings, Finding{r.ID, r.Device.IPv4, r.Device.Model, r.Device.MAC, check, status, evidence, source, stamp()})
	}
	owners := map[string][]string{}
	for _, r := range all {
		if !r.Removed && r.Device.IPv4 != "" {
			owners[r.Device.IPv4] = append(owners[r.Device.IPv4], r.ID)
		}
	}
	for _, r := range records {
		if ctx.Err() != nil {
			break
		}
		d := r.Device
		add := func(check, status, evidence, source string) { appendFinding(r, check, status, evidence, source) }
		if d.IPv4 == "" || d.SubnetMask == "" || d.Gateway == "" {
			add("Network consistency", "unknown", "Saved IPv4 address, mask or gateway is missing", "saved inventory")
		} else {
			n := configuration.NetworkSettings{IPv4: d.IPv4, SubnetMask: d.SubnetMask, Gateway: d.Gateway, CommandPort: 1}
			if err := configuration.ValidateNetwork(n); err != nil {
				add("Network consistency", "warning", err.Error(), "saved inventory")
			} else {
				add("Network consistency", "pass", "Saved IPv4, subnet mask and gateway are internally consistent", "saved inventory")
			}
		}
		if len(owners[d.IPv4]) > 1 {
			add("Duplicate IPv4", "fail", fmt.Sprintf("%d active saved records share this address; verify MAC identities and assignments", len(owners[d.IPv4])), "saved inventory")
		} else if d.IPv4 == "" {
			add("Duplicate IPv4", "unknown", "No saved IPv4 address", "saved inventory")
		} else {
			add("Duplicate IPv4", "pass", "No duplicate in this site's saved inventory; this does not rule out an unsaved network host", "saved inventory")
		}
		if d.Firmware == "" {
			add("Firmware version", "unknown", "No firmware version recorded", "saved inventory")
		} else {
			add("Firmware version", "pass", "Observed version: "+d.Firmware+". Update availability not assessed: no approved vendor/model/region baseline supplied.", "saved inventory")
		}
		ip, err := netip.ParseAddr(d.IPv4)
		usable := err == nil && ip.Is4() && !ip.IsUnspecified() && !ip.IsLoopback() && !ip.IsMulticast() && d.IPv4 != "255.255.255.255"
		for _, port := range []struct {
			name   string
			number *int
		}{{"HTTP", d.HTTPPort}, {"HTTPS", d.HTTPSPort}, {"SDK", d.CommandPort}, {"RTSP", d.RTSPPort}} {
			if ctx.Err() != nil {
				break
			}
			if port.number == nil || *port.number < 1 || *port.number > 65535 {
				add(port.name+" reachability", "not-checked", "Port was not advertised or recorded", "saved inventory")
				continue
			}
			if !usable {
				add(port.name+" reachability", "unknown", "No usable target IPv4 address", "saved inventory")
				continue
			}
			ok, detail := e.TCP(ctx, d.IPv4, *port.number)
			status := "unknown"
			if ok {
				status = "pass"
			}
			add(port.name+" reachability", status, detail, "TCP connect port "+strconv.Itoa(*port.number))
		}
		if usable && ctx.Err() == nil {
			web, err := e.Web(ctx, d)
			if err != nil || web.Endpoint.URL == "" {
				add("Web interface", "unknown", "HTTP/HTTPS detection did not establish a usable web interface", "bounded unauthenticated HTTP/HTTPS detection")
			} else {
				add("Web interface", "pass", web.Endpoint.URL, "HTTP/HTTPS response")
				if web.Endpoint.Scheme == "https" {
					status := "pass"
					detail := "Certificate trusted by this host"
					if !web.Endpoint.CertificateTrusted {
						status = "warning"
						detail = "Certificate is not trusted by this host; verify its SHA-256 fingerprint before authenticated access: " + web.Endpoint.CertificateSHA256
					}
					add("HTTPS certificate", status, detail, "TLS handshake")
				} else {
					add("HTTPS certificate", "warning", "Detected web interface uses HTTP; transport encryption was not established", "HTTP/HTTPS detection")
				}
			}
		}
		skipReason := "No ONVIF endpoint was discovered; authenticated health checks are unavailable"
		credentialSource := "none"
		var secret vault.Secret
		if !usable {
			skipReason = "No usable target IPv4 address"
		} else if len(d.XAddrs) > 0 && e.Secret != nil {
			var err error
			secret, credentialSource, err = e.Secret(r)
			if err != nil {
				skipReason = "Credential vault is locked or the assigned profile is unavailable"
			} else if secret.Username != "" && secret.Password != "" {
				skipReason = ""
			} else {
				skipReason = "No credentials assigned; configure an ONVIF credential profile"
			}
		}
		if skipReason != "" {
			for _, check := range []string{"Authentication", "DNS", "NTP", "Clock skew"} {
				add(check, "not-checked", skipReason, "ONVIF")
			}
		} else if ctx.Err() == nil {
			remote, err := e.Read(ctx, d, secret, pins[r.ID])
			secret.Clear()
			if err != nil {
				status := "unknown"
				if errors.Is(err, onvif.ErrAuthentication) {
					status = "fail"
				}
				add("Authentication", status, err.Error(), "ONVIF using "+credentialSource)
				for _, check := range []string{"DNS", "NTP", "Clock skew"} {
					add(check, "unknown", "Authenticated read did not complete; no configuration state inferred", "ONVIF")
				}
			}
			if err == nil {
				add("Authentication", "pass", "Read-only ONVIF device-information request succeeded", "ONVIF using "+credentialSource)
				if remote.Device.Firmware != "" {
					add("Current firmware", "pass", "Device reports "+remote.Device.Firmware+"; upgrade suitability was not assessed", "ONVIF GetDeviceInformation")
				}
				for _, w := range remote.Warnings {
					add("ONVIF partial data", "unknown", w, "ONVIF")
				}
				for _, n := range []struct {
					key, label string
					values     []string
				}{{"dns", "DNS", remote.Configuration.DNS}, {"ntp", "NTP", remote.Configuration.NTP}} {
					if err := remote.Configuration.Errors[n.key]; err != "" {
						add(n.label, "unknown", err, "ONVIF")
					} else if len(n.values) == 0 {
						add(n.label, "unknown", "Device returned no usable server values; unsupported/empty configuration cannot be distinguished", "ONVIF")
					} else {
						add(n.label, "pass", "Configured server(s): "+strings.Join(n.values, ", ")+". Server reachability was not tested.", "ONVIF")
					}
				}
				remoteTime, err := time.Parse(time.RFC3339, remote.Configuration.UTC)
				if err != nil {
					detail := remote.Configuration.Errors["clock"]
					if detail == "" {
						detail = "Device did not provide valid UTC time"
					}
					add("Clock skew", "unknown", detail, "ONVIF GetSystemDateAndTime")
				} else {
					delta := time.Since(remoteTime)
					if delta < 0 {
						delta = -delta
					}
					status := "pass"
					if delta > 2*time.Minute {
						status = "warning"
					}
					add("Clock skew", status, fmt.Sprintf("Approximately %.0f seconds from this PC's clock; confirm the PC clock is correct", delta.Seconds()), "ONVIF GetSystemDateAndTime")
				}
			}
		}
		secret.Clear()
		add("Uptime / storage health", "not-checked", "No supported device-specific health API has been established for this device", "capability limitation")
		report.Done++
		if progress != nil {
			progress(Clone(report))
		}
	}
	report.Cancelled = ctx.Err() != nil
	report.Finished = stamp()
	return report
}
func Clone(r Report) Report { r.Findings = append([]Finding{}, r.Findings...); return r }
