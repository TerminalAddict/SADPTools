// Package cameraip performs explicit, read-only checks of one routed IPv4 camera.
// It does not require multicast, discover adapters, save credentials or configure devices.
package cameraip

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/isapi"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type Options struct {
	Address        string `json:"address"`
	WebPort        int    `json:"webPort"`
	HTTPSPort      int    `json:"httpsPort"`
	RTSPPort       int    `json:"rtspPort"`
	ONVIFURL       string `json:"onvifURL"`
	Method         string `json:"method"`
	WebPin         string `json:"webPin"`
	ONVIFPin       string `json:"onvifPin"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}
type Evidence struct {
	Service string `json:"service"`
	State   string `json:"state"`
	Message string `json:"message"`
}
type Result struct {
	Device     devices.Device        `json:"device"`
	Web        webinterface.Endpoint `json:"web"`
	ONVIF      onvif.Endpoint        `json:"onvif"`
	Identified bool                  `json:"identified"`
	Evidence   []Evidence            `json:"evidence"`
	At         string                `json:"at"`
}

func (o Options) Validate() error {
	if !webinterface.UsableAddress(o.Address) {
		return errors.New("enter one camera IPv4 address, without a URL, port or subnet")
	}
	for _, p := range []int{o.WebPort, o.HTTPSPort, o.RTSPPort} {
		if p < 0 || p > 65535 {
			return errors.New("ports must be 0 (default) or 1–65535")
		}
	}
	if o.TimeoutSeconds < 5 || o.TimeoutSeconds > 60 {
		return errors.New("camera check timeout must be 5–60 seconds")
	}
	if o.Method != "auto" && o.Method != "onvif" && o.Method != "isapi" {
		return errors.New("choose automatic, ONVIF or ISAPI identification")
	}
	if o.ONVIFURL != "" && !onvif.ValidXAddr(o.ONVIFURL, o.Address) {
		return errors.New("ONVIF URL must use this exact camera IP and an /onvif/ path, without credentials or query parameters")
	}
	for _, p := range []string{o.WebPin, o.ONVIFPin} {
		if p != "" {
			b, e := hex.DecodeString(p)
			if e != nil || len(b) != 32 {
				return errors.New("certificate fingerprint must be 64 hexadecimal characters")
			}
		}
	}
	return nil
}

// Dependencies are small seams for synthetic tests. Production readers use the
// existing verified HTTP clients; they never follow redirects or use a proxy.
type Client struct {
	web     webinterface.Detector
	inspect func(context.Context, onvif.Target) (onvif.Endpoint, error)
	onvif   func(context.Context, onvif.Target, devices.Device, vault.Secret) (onvif.Information, error)
	isapi   func(context.Context, string, string, string, vault.Secret) (devices.Device, error)
	rtsp    func(context.Context, string, int) bool
}

func New() *Client {
	return &Client{web: webinterface.New(), inspect: onvif.InspectEndpoint, rtsp: probeRTSP,
		onvif: func(ctx context.Context, t onvif.Target, d devices.Device, s vault.Secret) (onvif.Information, error) {
			c, e := onvif.NewClient(t)
			if e != nil {
				return onvif.Information{}, e
			}
			return c.Information(ctx, d, s)
		},
		isapi: func(ctx context.Context, ip, base, pin string, s vault.Secret) (devices.Device, error) {
			c, e := isapi.New(ip, base, pin)
			if e != nil {
				return devices.Device{}, e
			}
			return c.DeviceInformation(ctx, s)
		},
	}
}
func (c *Client) Probe(parent context.Context, o Options, secret vault.Secret, progress func(string)) (Result, error) {
	defer secret.Clear()
	out := Result{Device: devices.Device{IPv4: o.Address, Protocols: []string{"direct-ip"}, Interfaces: []devices.Interface{}, XAddrs: []string{}}, Evidence: []Evidence{}, At: time.Now().UTC().Format(time.RFC3339Nano)}
	if e := o.Validate(); e != nil {
		return out, e
	}
	if e := streaming.ValidateCredentials(secret); e != nil {
		return out, e
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(o.TimeoutSeconds)*time.Second)
	defer cancel()
	note := func(service, state, message string) {
		out.Evidence = append(out.Evidence, Evidence{service, state, message})
	}
	update := func(message string) {
		if progress != nil {
			progress(message)
		}
	}
	update("Checking HTTP/HTTPS and RTSP through the system route")
	port := o.RTSPPort
	if port == 0 {
		port = 554
	}
	rtspDone := make(chan bool, 1)
	go func() { rtspDone <- c.rtsp(ctx, o.Address, port) }()
	web, e := c.web.Detect(ctx, webinterface.Target{Address: o.Address, Port: o.WebPort, HTTPSPort: o.HTTPSPort})
	hasRTSP := <-rtspDone // Join the bounded probe even on cancellation.
	if e != nil && ctx.Err() == nil {
		note("Web", "unavailable", "HTTP/HTTPS detection could not complete")
	}
	if web.Endpoint.ValidFor(o.Address) {
		out.Web = web.Endpoint
		p := out.Web.Port
		if out.Web.Scheme == "https" {
			out.Device.HTTPSPort = &p
		} else {
			out.Device.HTTPPort = &p
		}
		note("Web", "responded", web.Message)
	} else {
		note("Web", "no-response", "No HTTP/HTTPS response; check routing, firewall or custom ports")
	}
	if hasRTSP {
		out.Device.RTSPPort = &port
		note("RTSP", "responded", "RTSP protocol answered; no video or authentication was requested")
	} else {
		note("RTSP", "no-response", "No RTSP reply on port "+strconv.Itoa(port)+"; streaming may use a different port")
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if out.Web.URL != "" || hasRTSP {
		out.Device.FirstSeen = time.Now().UTC()
		out.Device.LastSeen = out.Device.FirstSeen
	}
	// Preserve an explicitly approved pin even for a now system-trusted endpoint:
	// the exact certificate still has to match, and is verified again by the reader.
	trusted := func(ep onvif.Endpoint, pin string) bool {
		return ep.Scheme != "https" || (pin != "" && strings.EqualFold(pin, ep.CertificateSHA256)) || (pin == "" && ep.CertificateTrusted)
	}
	stopFallback := false
	if o.Method != "isapi" {
		raw := o.ONVIFURL
		if raw == "" {
			base := out.Web.URL
			if base == "" {
				p := o.WebPort
				if p == 0 {
					p = 80
				}
				base = "http://" + o.Address + ":" + strconv.Itoa(p) + "/"
			}
			u, _ := url.Parse(base)
			u.Path = "/onvif/device_service"
			raw = u.String()
		}
		target := onvif.Target{Address: o.Address, URL: raw}
		update("Checking the camera's ONVIF device service")
		ep, err := c.inspect(ctx, target)
		out.ONVIF = ep
		pin := o.ONVIFPin
		if sameOrigin(raw, out.Web.URL) && pin == "" {
			pin = o.WebPin
		}
		if err != nil {
			// A failed HTTPS inspection must not downgrade an authenticated
			// attempt to a different (potentially HTTP) API.
			stopFallback = secret.Username != "" && strings.HasPrefix(raw, "https://")
			note("ONVIF", "unavailable", "Could not inspect the selected ONVIF endpoint")
		} else if !trusted(ep, pin) {
			stopFallback = secret.Username != ""
			note("ONVIF", "certificate-review", "Verify the displayed HTTPS fingerprint before reading camera information")
		} else {
			target.CertificateSHA256 = pin
			info, err := c.onvif(ctx, target, out.Device, secret)
			if err == nil && hasIdentity(info.Device) {
				out.Device = info.Device
				out.Device.IPv4 = o.Address
				out.Device.Protocols = []string{"direct-ip", "onvif"}
				out.Device.XAddrs = []string{raw}
				out.Identified = true
				note("ONVIF", "identified", "Device information read using ONVIF")
				for _, warning := range info.Warnings {
					note("ONVIF", "partial", sanitize.Text(warning))
				}
			} else {
				if err == nil {
					err = errors.New("camera returned no usable identity fields")
				}
				stopFallback = secret.Username != "" && (errors.Is(err, onvif.ErrAuthentication) || (ep.Scheme == "https" && out.Web.Scheme != "https"))
				note("ONVIF", readState(err), safeReadMessage(err, "ONVIF", secret.Username != ""))
			}
		}
	}
	if !out.Identified && o.Method != "onvif" && !stopFallback && ctx.Err() == nil && out.Web.URL != "" {
		update("Checking the camera's ISAPI device information")
		ep := onvif.Endpoint{URL: out.Web.URL, Scheme: out.Web.Scheme, CertificateTrusted: out.Web.CertificateTrusted, CertificateSHA256: out.Web.CertificateSHA256}
		if !trusted(ep, o.WebPin) {
			note("ISAPI", "certificate-review", "Verify the displayed HTTPS fingerprint before reading camera information")
		} else {
			d, err := c.isapi(ctx, o.Address, out.Web.URL, o.WebPin, secret)
			if err == nil && hasIdentity(d) {
				out.Device.Vendor = d.Vendor
				out.Device.Model = d.Model
				out.Device.Serial = d.Serial
				out.Device.MAC = d.MAC
				out.Device.Firmware = d.Firmware
				out.Device.Name = d.Name
				out.Device.DeviceType = d.DeviceType
				out.Device.Protocols = []string{"direct-ip", "isapi"}
				out.Identified = true
				note("ISAPI", "identified", "Device information read using ISAPI")
			} else {
				if err == nil {
					err = errors.New("camera returned no usable identity fields")
				}
				note("ISAPI", readState(err), safeReadMessage(err, "ISAPI", secret.Username != ""))
			}
		}
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if out.Identified {
		out.Device.FirstSeen = time.Now().UTC()
		out.Device.LastSeen = out.Device.FirstSeen
	}
	cleanDevice(&out.Device, secret)
	out.Identified = out.Identified && hasIdentity(out.Device)
	out.Device.ID = out.Device.Key()
	if !out.Identified {
		note("Identity", "unknown", "Camera identity was not verified. You may explicitly save the address as an unverified device")
	}
	return out, nil
}
func hasIdentity(d devices.Device) bool {
	return d.Model != "" || d.Serial != "" || d.MAC != "" || d.Vendor != ""
}
func sameOrigin(a, b string) bool {
	x, e := url.Parse(a)
	y, f := url.Parse(b)
	return e == nil && f == nil && x.Scheme == y.Scheme && x.Host == y.Host
}
func readState(e error) string {
	if errors.Is(e, onvif.ErrAuthentication) || errors.Is(e, isapi.ErrAuthentication) {
		return "authentication"
	}
	if errors.Is(e, onvif.ErrUnsupported) || errors.Is(e, isapi.ErrUnsupported) {
		return "unsupported"
	}
	return "unavailable"
}
func safeReadMessage(e error, service string, auth bool) string {
	switch readState(e) {
	case "authentication":
		if auth {
			return service + " rejected the account or permissions; no alternate service login was attempted"
		}
		return service + " requires authentication; enable the service and use an account with read permission"
	case "unsupported":
		return service + " endpoint is unsupported or disabled"
	default:
		return service + " did not return valid device information; check its port, service settings and network connection"
	}
}
func cleanDevice(d *devices.Device, s vault.Secret) {
	for _, p := range []*string{&d.Vendor, &d.Model, &d.Serial, &d.Firmware, &d.Name, &d.DeviceType} {
		*p = sanitize.Label(*p)
		for _, secret := range []string{s.Password, s.PrivacyPassword} {
			if secret != "" && strings.Contains(*p, secret) {
				*p = ""
			}
		}
	}
	d.MAC = devices.NormalizeMAC(d.MAC)
}
