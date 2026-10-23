package audit

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
	"testing"
	"time"
)

func TestEvidenceAuthenticationPartialAndCancellation(t *testing.T) {
	port := 80
	record := sites.Record{ID: "fixture", Device: devices.Device{IPv4: "192.0.2.10", Model: "Synthetic", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", HTTPPort: &port, XAddrs: []string{"http://192.0.2.10/onvif/device_service"}}}
	for _, tc := range []struct {
		name    string
		failure error
		want    string
	}{{"success", nil, "pass"}, {"denied", onvif.ErrAuthentication, "fail"}, {"timeout", context.DeadlineExceeded, "unknown"}, {"unsupported", onvif.ErrUnsupported, "unknown"}} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{TCP: func(context.Context, string, int) (bool, string) { return true, "synthetic TCP accepted" }, Web: func(context.Context, devices.Device) (webinterface.Result, error) {
				return webinterface.Result{}, errors.New("synthetic missing web")
			}, Secret: func(sites.Record) (vault.Secret, string, error) {
				return vault.Secret{Username: "fixture", Password: "synthetic"}, "synthetic profile", nil
			}, Read: func(context.Context, devices.Device, vault.Secret, string) (Remote, error) {
				return Remote{Configuration: onvif.Configuration{DNS: []string{"192.0.2.53"}, UTC: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), Errors: map[string]string{"ntp": "synthetic unsupported"}}}, tc.failure
			}}
			r := e.Run(context.Background(), sites.Site{ID: "fixture"}, []sites.Record{record}, []sites.Record{record, record}, nil, nil)
			status := map[string]string{}
			for _, f := range r.Findings {
				status[f.Check] = f.Status
				if f.Source == "" || f.At == "" {
					t.Fatal("missing evidence provenance")
				}
			}
			if status["Authentication"] != tc.want || status["Duplicate IPv4"] != "fail" || status["Web interface"] != "unknown" {
				t.Fatal(status)
			}
			if tc.failure == nil && (status["Clock skew"] != "warning" || status["NTP"] != "unknown") {
				t.Fatal(status)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := New()
	r := e.Run(ctx, sites.Site{}, []sites.Record{record}, nil, nil, nil)
	if !r.Cancelled || r.Done != 0 {
		t.Fatal(r)
	}
}
