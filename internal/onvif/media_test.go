package onvif

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/vault"
)

type mediaTransport func(*http.Request) (*http.Response, error)

func (f mediaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMediaProfilesURIAndDomainRestrictions(t *testing.T) {
	for _, namespace := range []string{media1NS, media2NS} {
		t.Run(namespace, func(t *testing.T) {
			calls := 0
			client, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device_service"})
			client.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				b, _ := io.ReadAll(r.Body)
				body := string(b)
				var reply string
				if strings.Contains(body, "synthetic-password") {
					t.Fatal("plaintext password in SOAP")
				}
				switch {
				case strings.Contains(body, "GetServices"):
					reply = soap("GetServices", `<Service><Namespace>`+namespace+`</Namespace><XAddr>http://192.0.2.3:80/onvif/media</XAddr></Service>`)
				case strings.Contains(body, "GetProfiles"):
					encoder := `<VideoEncoderConfiguration><Encoding>H264</Encoding><Resolution><Width>640</Width><Height>360</Height></Resolution></VideoEncoderConfiguration>`
					if namespace == media2NS {
						encoder = strings.ReplaceAll(strings.ReplaceAll(encoder, "VideoEncoderConfiguration", "VideoEncoder"), "<VideoEncoder>", "<Configurations><VideoEncoder>") + "</Configurations>"
					}
					reply = soap("GetProfiles", `<Profiles token="synthetic&lt;token"><Name>Synthetic sub stream</Name>`+encoder+`</Profiles>`)
				case strings.Contains(body, "GetStreamUri"):
					if !strings.Contains(body, "synthetic&lt;token") {
						t.Fatal("profile token not XML escaped")
					}
					if namespace == media1NS && !strings.Contains(body, "StreamSetup") {
						t.Fatal("missing Media1 setup")
					}
					if namespace == media2NS && !strings.Contains(body, "<tds:Protocol>RTSP") {
						t.Fatal("missing Media2 protocol")
					}
					reply = soap("GetStreamUri", `<MediaUri><Uri>rtsp://192.0.2.3:8554/synthetic</Uri></MediaUri>`)
				default:
					t.Fatal("unexpected operation")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(reply)), Header: make(http.Header)}, nil
			})
			secret := vault.Secret{Username: "fixture", Password: "synthetic-password"}
			catalog, e := client.MediaProfiles(context.Background(), secret, 30)
			if e != nil {
				t.Fatal(e)
			}
			if len(catalog.Profiles) != 1 || catalog.Profiles[0].Width != 640 || strings.Contains(catalog.Profiles[0].ID, "token") {
				t.Fatal("bad safe profile metadata")
			}
			raw, e := client.StreamURI(context.Background(), catalog, catalog.Profiles[0].ID, false, secret)
			if e != nil || raw != "rtsp://192.0.2.3:8554/synthetic" || calls != 3 {
				t.Fatal(raw, e, calls)
			}
		})
	}
	for _, endpoint := range []string{"http://192.0.2.4/onvif/media", "http://192.0.2.3:8080/onvif/media", "https://192.0.2.3/onvif/media", "http://u:p@192.0.2.3/onvif/media"} {
		client, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device_service"})
		calls := 0
		client.http.Transport = mediaTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(soap("GetServices", `<Service><Namespace>`+media1NS+`</Namespace><XAddr>`+endpoint+`</XAddr></Service>`)))}, nil
		})
		if _, e := client.MediaProfiles(context.Background(), vault.Secret{}, 0); e == nil || calls != 1 {
			t.Fatal("unsafe media authority followed")
		}
	}
}

func TestServiceAuthorityUsesEffectiveHTTPPort(t *testing.T) {
	for _, tc := range []struct {
		base, service string
		same          bool
	}{
		{"http://192.0.2.3/onvif/device_service", "http://192.0.2.3:80/onvif/media", true},
		{"http://192.0.2.3:80/onvif/device_service", "http://192.0.2.3/onvif/media", true},
		{"https://192.0.2.3/onvif/device_service", "https://192.0.2.3:443/onvif/media", true},
		{"http://192.0.2.3:2020/onvif/device_service", "http://192.0.2.3:2020/onvif/media", true},
		{"http://192.0.2.3:2020/onvif/device_service", "http://192.0.2.3/onvif/media", false},
		{"http://192.0.2.3/onvif/device_service", "https://192.0.2.3/onvif/media", false},
		{"http://192.0.2.3/onvif/device_service", "http://192.0.2.4/onvif/media", false},
	} {
		base, _ := url.Parse(tc.base)
		u, _ := url.Parse(tc.service)
		if sameHTTPAuthority(base, u) != tc.same {
			t.Fatalf("wrong authority comparison for %s / %s", tc.base, tc.service)
		}
		c, _ := NewClient(Target{Address: "192.0.2.3", URL: tc.base})
		if c.sameAuthority(tc.service) != tc.same {
			t.Fatalf("ONVIF service authority mismatch for %s", tc.service)
		}
	}
}
func TestMediaMalformedAndNoAuthFallback(t *testing.T) {
	for _, data := range []string{"<broken", soap("GetProfiles", `<Profiles token="x"><VideoEncoderConfiguration><Resolution><Width>999999</Width></Resolution></VideoEncoderConfiguration></Profiles>`), soap("GetProfiles", strings.Repeat(`<Profiles token="x"/>`, 65))} {
		if _, e := parseMediaProfiles([]byte(data), media1NS); e == nil {
			t.Fatal("invalid profiles accepted")
		}
	}
	client, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device_service"})
	calls := 0
	client.http.Transport = mediaTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 403, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if _, e := client.MediaProfiles(context.Background(), vault.Secret{Username: "fixture"}, 0); e != ErrAuthentication || calls != 1 {
		t.Fatal("retried rejected credentials")
	}
	if _, e := client.MediaProfiles(context.Background(), vault.Secret{}, 301); e == nil || calls != 1 {
		t.Fatal("invalid offset contacted camera")
	}
}

func TestSOAPAuthenticationFaultDoesNotTryAnotherService(t *testing.T) {
	c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device_service"})
	calls := 0
	c.http.Transport = mediaTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><s:Fault><s:Code><s:Value>s:Sender</s:Value><s:Subcode><s:Value>ter:NotAuthorized</s:Value></s:Subcode></s:Code></s:Fault></s:Body></s:Envelope>`))}, nil
	})
	if _, e := c.MediaProfiles(context.Background(), vault.Secret{Username: "synthetic"}, 0); e != ErrAuthentication || calls != 1 {
		t.Fatal("authentication SOAP fault retried", e, calls)
	}
}
