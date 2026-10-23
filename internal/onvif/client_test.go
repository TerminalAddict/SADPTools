package onvif

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// All addresses, identities and credentials in these fixtures are synthetic.
func TestDiscoveryScopeCorrelationAndPartialMetadata(t *testing.T) {
	packet := `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:d="` + DiscoveryNS + `" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"><s:Header><a:RelatesTo>synthetic-request</a:RelatesTo></s:Header><s:Body><d:ProbeMatches><d:ProbeMatch><a:EndpointReference><a:Address>urn:uuid:synthetic-camera</a:Address></a:EndpointReference><d:Scopes>onvif://www.onvif.org/name/Synthetic%20camera onvif://www.onvif.org/hardware/Fixture</d:Scopes><d:XAddrs>http://192.0.2.10/onvif/device_service http://192.0.2.11/onvif/device_service</d:XAddrs></d:ProbeMatch></d:ProbeMatches></s:Body></s:Envelope>`
	d, e := Parse([]byte(packet), "192.0.2.10", "synthetic-request", platform.Adapter{Name: "test-adapter"})
	if e != nil || len(d) != 1 || d[0].Name != "Synthetic camera" || len(d[0].XAddrs) != 1 || d[0].MAC != "" || d[0].Activated != nil || d[0].SADPCompatible() {
		t.Fatalf("partial device: %+v %v", d, e)
	}
	for _, bad := range []string{packet + "<another/>", strings.ReplaceAll(packet, "synthetic-request", "other"), "<broken", strings.Repeat("x", 65537)} {
		if _, e = Parse([]byte(bad), "192.0.2.10", "synthetic-request", platform.Adapter{}); e == nil {
			t.Fatal("accepted malformed/unrelated response")
		}
	}
	for _, bad := range []string{"http://evil.example/onvif/device_service", "http://192.0.2.11/onvif/device_service", "file:///onvif/device_service", "http://user:secret@192.0.2.10/onvif/x", "http://192.0.2.10/onvif/../ISAPI/x", "http://192.0.2.10:99999/onvif/x", "http://192.0.2.10/onvif/x?forward=http://other/"} {
		if ValidXAddr(bad, "192.0.2.10") {
			t.Fatal("unsafe XAddr accepted")
		}
	}
	if !ValidXAddr("https://192.0.2.10:8443/onvif/device_service", "192.0.2.10") {
		t.Fatal("valid custom HTTPS port rejected")
	}
	var envelope any
	if e = xml.Unmarshal(Probe("synthetic<&>"), &envelope); e != nil {
		t.Fatal(e)
	}
}
func soap(operation, body string) string {
	return `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><` + operation + `Response xmlns="` + deviceNS + `">` + body + `</` + operation + `Response></s:Body></s:Envelope>`
}
func TestReadOnlyChallengeAndInformation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "PasswordDigest") || strings.Contains(string(b), "synthetic-secret") {
			t.Error("unsafe UsernameToken")
		}
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="fixture", nonce="synthetic-nonce", algorithm=SHA-256, qop="auth"`)
			w.WriteHeader(401)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			t.Error("no digest response")
		}
		for _, op := range []string{"GetDeviceInformation", "GetNetworkInterfaces", "GetNetworkProtocols", "GetNetworkDefaultGateway"} {
			if strings.Contains(string(b), "<tds:"+op+"/>") {
				body := ""
				switch op {
				case "GetDeviceInformation":
					body = `<Manufacturer>Synthetic vendor</Manufacturer><Model>Fixture</Model><SerialNumber>fixture-1</SerialNumber><FirmwareVersion>fixture-fw</FirmwareVersion>`
				case "GetNetworkInterfaces":
					body = `<NetworkInterfaces><Enabled>true</Enabled><Info><HwAddress>02:00:00:00:00:01</HwAddress></Info><IPv4><Enabled>true</Enabled><Config><Manual><Address>192.0.2.10</Address><PrefixLength>24</PrefixLength></Manual><DHCP>false</DHCP></Config></IPv4></NetworkInterfaces>`
				case "GetNetworkProtocols":
					body = `<NetworkProtocols><Name>HTTPS</Name><Enabled>true</Enabled><Port>8443</Port></NetworkProtocols>`
				case "GetNetworkDefaultGateway":
					body = `<NetworkGateway><IPv4Address>192.0.2.1</IPv4Address></NetworkGateway>`
				}
				io.WriteString(w, soap(op, body))
				return
			}
		}
		w.WriteHeader(400)
	}))
	defer server.Close()
	c := &Client{target: Target{URL: server.URL, Address: "192.0.2.10"}, http: server.Client()}
	info, e := c.Information(context.Background(), devices.Device{UUID: "fixture", IPv4: "192.0.2.10"}, vault.Secret{Username: "fixture", Password: "synthetic-secret"})
	if e != nil || info.Device.Serial != "fixture-1" || info.Device.MAC != "02:00:00:00:00:01" || info.Device.SubnetMask != "255.255.255.0" || info.Device.HTTPSPort == nil || *info.Device.HTTPSPort != 8443 || requests.Load() != 8 {
		t.Fatalf("%+v %v requests=%d", info, e, requests.Load())
	}
	before := requests.Load()
	if _, e = c.Read(context.Background(), "SetNetworkInterfaces", vault.Secret{}); e == nil || requests.Load() != before {
		t.Fatal("write operation was sent")
	}
}
func TestDeniedMalformedAndCancelled(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{{403, "", ErrAuthentication}, {500, "", ErrUnsupported}, {200, "<broken", nil}, {200, soap("GetDNS", "") + "<extra/>", nil}, {200, strings.Repeat("x", (1<<20)+1), nil}} {
		t.Run(base64.RawURLEncoding.EncodeToString([]byte(tc.body[:min(len(tc.body), 8)])), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer server.Close()
			c := &Client{target: Target{URL: server.URL}, http: server.Client()}
			_, e := c.Read(context.Background(), "GetDNS", vault.Secret{})
			if e == nil || (tc.want != nil && !errors.Is(e, tc.want)) {
				t.Fatal(e)
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer server.Close()
	c := &Client{target: Target{URL: server.URL}, http: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, e := c.Read(ctx, "GetDNS", vault.Secret{}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}

func TestHTTPSRequiresTrustedCertificateOrExactExplicitPin(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		io.WriteString(w, soap("GetDNS", ""))
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	fingerprint := sha256.Sum256(server.Certificate().Raw)
	for _, tc := range []struct {
		name, pin string
		allowed   bool
	}{
		{"untrusted", "", false},
		{"accepted exact fingerprint", hex.EncodeToString(fingerprint[:]), true},
		{"changed certificate", strings.Repeat("00", 32), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(Target{Address: "192.0.2.10", URL: "https://192.0.2.10/onvif/device_service", CertificateSHA256: tc.pin})
			if err != nil {
				t.Fatal(err)
			}
			// Keep production TLS checks; route this synthetic target only to the
			// loopback fixture. No physical camera or external network is contacted.
			transport := c.http.Transport.(*http.Transport)
			transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp4", server.Listener.Addr().String())
			}
			before := received.Load()
			_, err = c.Read(context.Background(), "GetDNS", vault.Secret{Username: "fixture", Password: "synthetic-secret"})
			if (err == nil) != tc.allowed {
				t.Fatal("incorrect certificate acceptance", err)
			}
			if !tc.allowed && received.Load() != before {
				t.Fatal("credentials sent before certificate verification")
			}
		})
	}
}
