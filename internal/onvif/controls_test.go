package onvif

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestControlIdentityBoundsAndUnconfirmedFailures(t *testing.T) {
	for _, mode := range []string{"ok", "wrong-device", "disconnect", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device"})
			writes := 0
			c.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
				raw, _ := io.ReadAll(r.Body)
				body := string(raw)
				var reply string
				if strings.Contains(body, "GetDeviceInformation") {
					serial := "synthetic-serial"
					if mode == "wrong-device" {
						serial = "different-device"
					}
					reply = soap("GetDeviceInformation", "<SerialNumber>"+serial+"</SerialNumber>")
				} else if strings.Contains(body, "ContinuousMove") {
					writes++
					if !strings.Contains(body, "PT0.5S") || !strings.Contains(body, "synthetic-profile") {
						t.Fatal("unbounded or wrong-profile movement")
					}
					if mode == "disconnect" {
						return nil, errors.New("synthetic loss")
					}
					reply = soap("ContinuousMove", "")
					if mode == "malformed" {
						reply = "<broken"
					}
				} else {
					t.Fatal("unexpected operation")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply))}, nil
			})
			ctrl := &Control{client: c, serial: "synthetic-serial", profile: "synthetic-profile", ptzEndpoint: "http://192.0.2.3/onvif/ptz", caps: ControlCapabilities{PanTilt: true}, secret: vault.Secret{Username: "fixture", Password: "synthetic"}}
			e := ctrl.Move(context.Background(), .2, 0, 0)
			switch mode {
			case "ok":
				if e != nil || writes != 1 {
					t.Fatal(e, writes)
				}
			case "wrong-device":
				if e == nil || writes != 0 {
					t.Fatal("identity guard failed")
				}
			default:
				if !errors.Is(e, ErrOutcomeUnconfirmed) || writes != 1 {
					t.Fatal("write retried or certainty lost", e, writes)
				}
			}
		})
	}
}
func TestFixedCameraAndStalePreset(t *testing.T) {
	c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device"})
	catalog := MediaCatalog{Profiles: []MediaProfile{{ID: "fixed", token: "fixed"}}}
	ctrl, e := c.Controls(context.Background(), catalog, "fixed", "serial", vault.Secret{})
	if e != nil || ctrl.Capabilities().PanTilt || ctrl.Capabilities().RelativeFocus {
		t.Fatal(ctrl, e)
	}
	if !errors.Is(ctrl.Move(context.Background(), .1, 0, 0), ErrUnsupported) {
		t.Fatal("fixed camera moved")
	}
	if _, e = c.Controls(context.Background(), catalog, "missing", "serial", vault.Secret{}); e == nil {
		t.Fatal("missing profile accepted")
	}
	ctrl.ptzEndpoint = "http://192.0.2.3/ptz"
	ctrl.caps.Presets = true
	c.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(soap("GetPresets", ""))), Header: make(http.Header)}, nil
	})
	if _, e = ctrl.preset(context.Background(), "stale-id"); e == nil {
		t.Fatal("stale preset accepted")
	}
}
