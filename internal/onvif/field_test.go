package onvif

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/recordings"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

// Synthetic protocol fixtures only. No physical devices or credentials are used.
func TestImagingCapabilitiesIdentityBoundsAndUnconfirmedWrite(t *testing.T) {
	for _, mode := range []string{"ok", "nested", "wrong-device", "stale", "range", "timeout", "readback", "unsupported", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device"})
			writes := 0
			value := "40"
			c.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
				raw, _ := io.ReadAll(r.Body)
				body := string(raw)
				var reply string
				switch {
				case strings.Contains(body, "GetServices"):
					reply = soap("GetServices", `<Service><Namespace>`+imagingNS+`</Namespace><XAddr>http://192.0.2.3/onvif/imaging</XAddr></Service>`)
				case strings.Contains(body, "GetDeviceInformation"):
					serial := "synthetic-serial"
					if mode == "wrong-device" {
						serial = "other-serial"
					}
					reply = soap("GetDeviceInformation", "<SerialNumber>"+serial+"</SerialNumber>")
				case strings.Contains(body, "GetOptions"):
					opts := `<Brightness><Min>0</Min><Max>100</Max></Brightness><IrCutFilter>AUTO</IrCutFilter><IrCutFilter>ON</IrCutFilter><WideDynamicRange><Mode>ON</Mode><Mode>OFF</Mode><Level><Min>0</Min><Max>100</Max></Level></WideDynamicRange>`
					if mode == "unsupported" {
						opts = ""
					}
					reply = soap("GetOptions", "<ImagingOptions>"+opts+"</ImagingOptions>")
				case strings.Contains(body, "GetImagingSettings"):
					reply = soap("GetImagingSettings", "<ImagingSettings><Brightness>"+value+"</Brightness><IrCutFilter>AUTO</IrCutFilter><WideDynamicRange><Mode>ON</Mode><Level>"+value+"</Level></WideDynamicRange></ImagingSettings>")
				case strings.Contains(body, "SetImagingSettings"):
					writes++
					if r.GetBody != nil {
						t.Fatal("write must not be transparently replayable")
					}
					expectedXML := "<tt:Brightness>50</tt:Brightness>"
					if mode == "nested" {
						expectedXML = "<tt:WideDynamicRange><tt:Mode>ON</tt:Mode><tt:Level>50</tt:Level></tt:WideDynamicRange>"
					}
					if !strings.Contains(body, expectedXML) || !strings.Contains(body, "source&lt;1") {
						t.Fatal("wrong/unescaped setting")
					}
					if mode == "timeout" {
						return nil, context.DeadlineExceeded
					}
					if mode != "readback" {
						value = "50"
					}
					reply = soap("SetImagingSettings", "")
					if mode == "malformed" {
						reply = "<broken"
					}
				default:
					t.Fatal("unexpected operation")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply))}, nil
			})
			cat := MediaCatalog{Profiles: []MediaProfile{{ID: "profile-1", sourceToken: "source<1"}}}
			wanted, previous := "50", "40"
			if mode == "range" {
				wanted = "101"
			}
			if mode == "stale" {
				previous = "39"
			}
			field := "Brightness"
			if mode == "nested" {
				field = "WideDynamicRange.Level"
			}
			state, e := c.SetImageSetting(context.Background(), cat, "profile-1", "synthetic-serial", field, wanted, previous, vault.Secret{})
			switch mode {
			case "ok", "nested":
				if e != nil || state != "verified" || writes != 1 {
					t.Fatal(state, e, writes)
				}
			case "timeout", "readback", "malformed":
				if e == nil || state != "unconfirmed" || writes != 1 {
					t.Fatal("outcome/retry guard", state, e, writes)
				}
			default:
				if e == nil || state != "blocked" || writes != 0 {
					t.Fatal("preflight guard", state, e, writes)
				}
			}
		})
	}
}

func TestRecordingBoundsAreCoarseAndServiceScoped(t *testing.T) {
	for _, bad := range []bool{false, true} {
		c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device"})
		c.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(r.Body)
			body := string(raw)
			reply := ""
			switch {
			case strings.Contains(body, "GetServices"):
				reply = soap("GetServices", `<Service><Namespace>`+recordingNS+`</Namespace><XAddr>http://192.0.2.3/onvif/recording</XAddr></Service><Service><Namespace>`+searchNS+`</Namespace><XAddr>http://192.0.2.3/onvif/search</XAddr></Service>`)
			case strings.Contains(body, "GetRecordings"):
				reply = soap("GetRecordings", `<RecordingItem><RecordingToken>synthetic&lt;recording</RecordingToken></RecordingItem>`)
			case strings.Contains(body, "GetRecordingInformation"):
				if r.URL.Path != "/onvif/search" || !strings.Contains(body, "synthetic&lt;recording") {
					t.Fatal("search service/token boundary")
				}
				reply = soap("GetRecordingInformation", `<RecordingInformation><EarliestRecording>2026-01-01T10:00:00+13:00</EarliestRecording><LatestRecording>2026-01-01T12:00:00+13:00</LatestRecording></RecordingInformation>`)
				if bad {
					reply = soap("GetRecordingInformation", "")
				}
			default:
				t.Fatal("unexpected operation")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply))}, nil
		})
		start, _ := time.Parse(time.RFC3339, "2025-12-31T20:00:00Z")
		r, e := c.RecordingMetadata(context.Background(), recordings.Window{Start: start, End: start.Add(4 * time.Hour)}, vault.Secret{})
		if bad {
			if e == nil {
				t.Fatal("incomplete response accepted")
			}
			continue
		}
		if e != nil || len(r.Intervals) != 1 || len(r.Gaps) != 0 || r.Replay == "verified" || r.Intervals[0].Precision != "coarse" {
			t.Fatal(r, e)
		}
	}
}

func TestANPRUnknownsUTCAndSafeFields(t *testing.T) {
	raw := []byte(`<Envelope><NotificationMessage><Topic>tns:RuleEngine/ANPR/Plate</Topic><Message UtcTime="2026-01-01T13:00:00+13:00"><Data><SimpleItem Name="LicensePlate" Value="SYNTHETIC-PLATE"/><SimpleItem Name="Password" Value="must-not-leak"/><SimpleItem Name="Lane" Value="2"/></Data></Message></NotificationMessage><NotificationMessage><Topic>Motion</Topic><Message><SimpleItem Name="Plate" Value="not-a-plate-event"/></Message></NotificationMessage></Envelope>`)
	a, e := ParsePlates(raw, time.Now())
	if e != nil || len(a) != 1 || a[0].Plate != "SYNTHETIC-PLATE" || a[0].Confidence != nil || a[0].DeviceTime != "2026-01-01T00:00:00Z" || a[0].Integration != "not-verified" {
		t.Fatal(a, e)
	}
	b, _ := ParsePlates(raw, time.Now().Add(time.Minute))
	if a[0].ID != b[0].ID {
		t.Fatal("dedup key changed on repeat")
	}
	if _, e = ParsePlates([]byte(`<broken`), time.Now()); e == nil {
		t.Fatal("malformed event accepted")
	}
	c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device"})
	c.http.Transport = mediaTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsafe probe sent")
		return nil, errors.New("blocked")
	})
	if _, _, e = c.Inspect(context.Background(), "SetImagingSettings", vault.Secret{}); e == nil {
		t.Fatal("inspector permits write")
	}
}
