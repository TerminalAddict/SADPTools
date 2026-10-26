package cameraip

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/isapi"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

// All addresses and identities in this file are synthetic documentation fixtures.
type fakeWeb func(context.Context, webinterface.Target) (webinterface.Result, error)

func (f fakeWeb) Detect(ctx context.Context, target webinterface.Target) (webinterface.Result, error) {
	return f(ctx, target)
}
func syntheticOptions() Options {
	return Options{Address: "192.0.2.10", Method: "auto", TimeoutSeconds: 5}
}
func syntheticClient(t *testing.T) *Client {
	t.Helper()
	return &Client{
		web: fakeWeb(func(_ context.Context, target webinterface.Target) (webinterface.Result, error) {
			if target.Source != "" {
				t.Error("routed check bound to a local discovery adapter")
			}
			return webinterface.Result{Endpoint: webinterface.Endpoint{URL: "http://192.0.2.10/", Scheme: "http", Port: 80}, Message: "Synthetic HTTP response"}, nil
		}),
		inspect: func(_ context.Context, target onvif.Target) (onvif.Endpoint, error) {
			u, _ := url.Parse(target.URL)
			return onvif.Endpoint{URL: target.URL, Scheme: u.Scheme}, nil
		},
		onvif: func(context.Context, onvif.Target, devices.Device, vault.Secret) (onvif.Information, error) {
			return onvif.Information{}, onvif.ErrUnsupported
		},
		isapi: func(context.Context, string, string, string, vault.Secret) (devices.Device, error) {
			return devices.Device{}, isapi.ErrUnsupported
		},
		rtsp: func(context.Context, string, int) bool { return false },
	}
}
func TestOptionsRejectInvalidTargetsBeforeNetwork(t *testing.T) {
	for _, address := range []string{"", "camera.example", "127.0.0.1", "0.0.0.0", "239.255.255.250", "255.255.255.255", "http://192.0.2.10/", "192.0.2.0/24", "192.0.2.10:80", "::1"} {
		o := syntheticOptions()
		o.Address = address
		if e := o.Validate(); e == nil {
			t.Errorf("accepted %q", address)
		}
	}
	for _, mutate := range []func(*Options){func(o *Options) { o.WebPort = -1 }, func(o *Options) { o.HTTPSPort = 65536 }, func(o *Options) { o.TimeoutSeconds = 4 }, func(o *Options) { o.TimeoutSeconds = 61 }, func(o *Options) { o.Method = "write" }, func(o *Options) { o.WebPin = "bad" }, func(o *Options) { o.ONVIFURL = "http://198.51.100.1/onvif/device_service" }, func(o *Options) { o.ONVIFURL = "http://user:secret@192.0.2.10/onvif/device_service" }, func(o *Options) { o.ONVIFURL = "http://192.0.2.10/onvif/device_service?password=secret" }} {
		o := syntheticOptions()
		mutate(&o)
		if e := o.Validate(); e == nil {
			t.Errorf("accepted invalid options: %+v", o)
		}
	}
	o := syntheticOptions()
	o.ONVIFURL = "https://192.0.2.10:8443/onvif/device_service"
	o.WebPin = strings.Repeat("ab", 32)
	if e := o.Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestRoutedONVIFIdentityAndUnknownProperties(t *testing.T) {
	c := syntheticClient(t)
	o := syntheticOptions()
	o.ONVIFURL = "http://192.0.2.10:8080/onvif/device_service"
	o.RTSPPort = 8554
	c.rtsp = func(_ context.Context, ip string, port int) bool {
		if ip != o.Address || port != 8554 {
			t.Error("wrong RTSP target")
		}
		return true
	}
	c.onvif = func(_ context.Context, target onvif.Target, base devices.Device, s vault.Secret) (onvif.Information, error) {
		if target.URL != o.ONVIFURL || target.Address != o.Address {
			t.Error("wrong ONVIF target")
		}
		base.Model = "SYNTHETIC camera"
		base.Serial = "SYNTHETIC-001"
		base.MAC = "02:00:00:00:00:10"
		return onvif.Information{Device: base}, nil
	}
	c.isapi = func(context.Context, string, string, string, vault.Secret) (devices.Device, error) {
		t.Error("unnecessary fallback after identity")
		return devices.Device{}, nil
	}
	r, e := c.Probe(context.Background(), o, vault.Secret{}, nil)
	if e != nil || !r.Identified || r.Device.ID != "mac:02:00:00:00:00:10" || r.Device.RTSPPort == nil || *r.Device.RTSPPort != 8554 {
		t.Fatal(r, e)
	}
	if r.Device.Activated != nil || r.Device.DHCP != nil || r.Device.SubnetMask != "" || len(r.Device.Interfaces) != 0 || r.Device.SADPCompatible() || r.Device.Vendor != "" {
		t.Fatal("invented discovery, activation or vendor evidence", r)
	}
	if len(r.Device.XAddrs) != 1 || r.Device.XAddrs[0] != o.ONVIFURL {
		t.Fatal(r)
	}
}
func TestFallbackAndAuthenticationDoNotGuessOrRetry(t *testing.T) {
	for _, auth := range []bool{false, true} {
		t.Run(map[bool]string{false: "anonymous", true: "authenticated"}[auth], func(t *testing.T) {
			c := syntheticClient(t)
			secret := vault.Secret{}
			if auth {
				secret = vault.Secret{Username: "fixture", Password: "SYNTHETIC-PASSWORD"}
			}
			c.onvif = func(context.Context, onvif.Target, devices.Device, vault.Secret) (onvif.Information, error) {
				return onvif.Information{}, onvif.ErrAuthentication
			}
			calls := 0
			c.isapi = func(context.Context, string, string, string, vault.Secret) (devices.Device, error) {
				calls++
				return devices.Device{Model: "SYNTHETIC", Serial: "SYNTHETIC-002"}, nil
			}
			r, e := c.Probe(context.Background(), syntheticOptions(), secret, nil)
			if e != nil {
				t.Fatal(e)
			}
			if auth && (calls != 0 || r.Identified) || !auth && (calls != 1 || !r.Identified) {
				t.Fatal("incorrect fallback", calls, r)
			}
			if r.Device.Activated != nil || r.Device.Vendor != "" {
				t.Fatal("ISAPI implies neither activation nor vendor")
			}
		})
	}
}
func TestCertificatesBlockCredentialsAndPreventDowngrade(t *testing.T) {
	fingerprint := strings.Repeat("ab", 32)
	for _, tc := range []struct {
		name, pin                      string
		inspectError, changed, webHTTP bool
		wantRead                       bool
	}{
		{name: "untrusted"}, {name: "approved", pin: fingerprint, wantRead: true}, {name: "changed", pin: fingerprint, changed: true}, {name: "HTTP fallback", webHTTP: true}, {name: "inspection failure", webHTTP: true, inspectError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := syntheticClient(t)
			o := syntheticOptions()
			o.ONVIFURL = "https://192.0.2.10:8443/onvif/device_service"
			o.ONVIFPin = tc.pin
			if !tc.webHTTP {
				c.web = fakeWeb(func(context.Context, webinterface.Target) (webinterface.Result, error) {
					return webinterface.Result{Endpoint: webinterface.Endpoint{URL: "https://192.0.2.10:8443/", Scheme: "https", Port: 8443, CertificateSHA256: fingerprint}}, nil
				})
			}
			c.inspect = func(_ context.Context, target onvif.Target) (onvif.Endpoint, error) {
				if tc.inspectError {
					return onvif.Endpoint{}, errors.New("synthetic TLS failure")
				}
				fp := fingerprint
				if tc.changed {
					fp = strings.Repeat("cd", 32)
				}
				return onvif.Endpoint{URL: target.URL, Scheme: "https", CertificateSHA256: fp}, nil
			}
			reads := 0
			c.onvif = func(_ context.Context, target onvif.Target, base devices.Device, s vault.Secret) (onvif.Information, error) {
				reads++
				if target.CertificateSHA256 != fingerprint {
					t.Error("pin not passed to verified reader")
				}
				base.Model = "SYNTHETIC"
				return onvif.Information{Device: base}, nil
			}
			c.isapi = func(context.Context, string, string, string, vault.Secret) (devices.Device, error) {
				t.Error("credential downgrade/fallback")
				return devices.Device{}, nil
			}
			r, e := c.Probe(context.Background(), o, vault.Secret{Username: "fixture", Password: "SYNTHETIC-PASSWORD"}, nil)
			if e != nil || r.Identified != tc.wantRead || (reads == 1) != tc.wantRead {
				t.Fatal(reads, r, e)
			}
		})
	}
}
func TestUnreachablePreservesUnknownsAndRedactsReflectedPassword(t *testing.T) {
	c := syntheticClient(t)
	c.web = fakeWeb(func(context.Context, webinterface.Target) (webinterface.Result, error) {
		return webinterface.Result{}, nil
	})
	r, e := c.Probe(context.Background(), syntheticOptions(), vault.Secret{}, nil)
	if e != nil || r.Identified || r.Device.ID != "" || r.Device.MAC != "" || r.Device.HTTPPort != nil || r.Device.RTSPPort != nil || !r.Device.LastSeen.IsZero() {
		t.Fatal(r, e)
	}
	c = syntheticClient(t)
	c.onvif = func(_ context.Context, _ onvif.Target, base devices.Device, s vault.Secret) (onvif.Information, error) {
		base.Model = s.Password
		return onvif.Information{Device: base}, nil
	}
	r, e = c.Probe(context.Background(), syntheticOptions(), vault.Secret{Username: "fixture", Password: "SYNTHETIC-SECRET"}, nil)
	b, _ := json.Marshal(r)
	if e != nil || r.Identified || strings.Contains(string(b), "SYNTHETIC-SECRET") {
		t.Fatal("reflected credential escaped identity cleaning", e)
	}
}
func TestCancellationAndDeadlineJoinProbes(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			c := syntheticClient(t)
			webDone, rtspDone := make(chan struct{}), make(chan struct{})
			c.web = fakeWeb(func(ctx context.Context, _ webinterface.Target) (webinterface.Result, error) {
				defer close(webDone)
				<-ctx.Done()
				return webinterface.Result{}, ctx.Err()
			})
			c.rtsp = func(ctx context.Context, _ string, _ int) bool { defer close(rtspDone); <-ctx.Done(); return false }
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
			} else {
				cancel()
			}
			defer cancel()
			_, e := c.Probe(ctx, syntheticOptions(), vault.Secret{}, nil)
			if !errors.Is(e, context.Canceled) && !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal(e)
			}
			for _, ch := range []chan struct{}{webDone, rtspDone} {
				select {
				case <-ch:
				default:
					t.Fatal("probe outlived operation")
				}
			}
		})
	}
}
