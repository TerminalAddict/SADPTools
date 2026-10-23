package webinterface

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic camera addresses are redirected to loopback fixtures. These tests
// never probe a LAN device, send authentication or follow a device action URL.
const cameraIP = "192.0.2.40"

func fixtureServer(t *testing.T, secure bool, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(handler)
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	if secure {
		s.StartTLS()
	} else {
		s.Start()
	}
	t.Cleanup(s.Close)
	return s
}

func fixtureClient(ports map[string]*httptest.Server) *Client {
	return &Client{transport: func(target Target) *http.Transport {
		tr := transport(target)
		tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || host != cameraIP {
				return nil, errors.New("unexpected fixture destination")
			}
			if s := ports[port]; s != nil {
				return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
			}
			return nil, errors.New("synthetic closed port")
		}
		return tr
	}}
}

func TestSeparatelyAdvertisedHTTPSPort(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }
	plain := fixtureServer(t, false, handler)
	secure := fixtureServer(t, true, handler)
	result, err := fixtureClient(map[string]*httptest.Server{"80": plain, "8443": secure}).Detect(context.Background(), Target{Address: cameraIP, Port: 80, HTTPSPort: 8443})
	if err != nil || result.Endpoint.URL != "https://192.0.2.40:8443/" {
		t.Fatal("separate ONVIF HTTPS port was ignored", result, err)
	}
}

func TestDetectWebProtocolsAndPorts(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		advertised, httpPort, httpsPort int
		scheme                          string
		port                            int
	}{
		{"http only", 80, 80, 0, "http", 80},
		{"custom http", 8080, 8080, 0, "http", 8080},
		{"prefer https", 80, 80, 443, "https", 443},
		{"https only", 80, 0, 443, "https", 443},
		{"custom https", 8443, 0, 8443, "https", 8443},
		{"unknown advertised port", 0, 0, 443, "https", 443},
		{"unreachable", 80, 0, 0, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ports := map[string]*httptest.Server{}
			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.RequestURI() != "/" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("probe sent an action, credentials or non-root path")
				}
				w.WriteHeader(http.StatusUnauthorized) // A login challenge still identifies a web service.
			}
			if tc.httpPort > 0 {
				ports[strconv.Itoa(tc.httpPort)] = fixtureServer(t, false, handler)
			}
			if tc.httpsPort > 0 {
				ports[strconv.Itoa(tc.httpsPort)] = fixtureServer(t, true, handler)
			}
			result, err := fixtureClient(ports).Detect(context.Background(), Target{Address: cameraIP, Port: tc.advertised})
			if err != nil || result.Endpoint.Scheme != tc.scheme || result.Endpoint.Port != tc.port {
				t.Fatalf("%+v %v", result, err)
			}
			if tc.scheme == "" {
				if result.Endpoint.URL != "" || result.Message == "" {
					t.Fatal(result)
				}
				return
			}
			if !result.Endpoint.ValidFor(cameraIP) {
				t.Fatal(result)
			}
			if tc.scheme == "https" {
				hash := sha256.Sum256(ports[strconv.Itoa(tc.httpsPort)].Certificate().Raw)
				if result.Endpoint.CertificateSHA256 != hex.EncodeToString(hash[:]) || result.Endpoint.CertificateTrusted {
					t.Fatal("certificate not inspected correctly", result)
				}
			}
		})
	}
}

func TestRedirectsStayOnCameraAndOnlyProbeOrigins(t *testing.T) {
	for _, location := range []string{
		"https://192.0.2.40:8443/synthetic-action?reset=true",
		"/login?next=synthetic-action", "https://example.invalid/",
		"https://user:password@192.0.2.40/", "file:///etc/passwd", "http://192.0.2.40:99999/",
	} {
		t.Run(location, func(t *testing.T) {
			var hits atomic.Int32
			s := fixtureServer(t, false, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, location, http.StatusFound) })
			tls := fixtureServer(t, true, func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.URL.RequestURI() != "/" {
					t.Error("followed action path")
				}
				w.WriteHeader(200)
			})
			result, err := fixtureClient(map[string]*httptest.Server{"80": s, "8443": tls}).Detect(context.Background(), Target{Address: cameraIP, Port: 80})
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case strings.Contains(location, ":8443"):
				if result.Endpoint.URL != "https://192.0.2.40:8443/" || hits.Load() != 1 {
					t.Fatal(result, hits.Load())
				}
			case strings.HasPrefix(location, "/login"):
				if result.Endpoint.URL != "http://192.0.2.40/" || hits.Load() != 0 {
					t.Fatal(result)
				}
			default:
				if result.Endpoint.URL != "" || hits.Load() != 0 {
					t.Fatal("unsafe redirect accepted", result)
				}
			}
		})
	}
}

func TestCancellationJoinsStalledProbes(t *testing.T) {
	started := make(chan struct{}, 3)
	var active atomic.Int32
	c := &Client{transport: func(target Target) *http.Transport {
		tr := transport(target)
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			active.Add(1)
			defer active.Add(-1)
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return tr
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Detect(ctx, Target{Address: cameraIP, Port: 80}); done <- err }()
	for range 3 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("probes did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("detection did not cancel")
	}
	// net/http may finish cancelling its dial goroutines just after Do returns.
	deadline := time.Now().Add(5 * time.Second)
	for active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active.Load() != 0 {
		t.Fatal("dial remained active after cancellation")
	}
}

func TestEndpointValidation(t *testing.T) {
	for _, ip := range []string{"", "127.0.0.1", "::1", "0.0.0.0", "239.255.255.250", "255.255.255.255", "example.invalid"} {
		if (Target{Address: ip}).Validate() == nil || (Endpoint{URL: "http://" + ip + "/", Scheme: "http", Port: 80}).ValidFor(ip) {
			t.Fatal("invalid IP accepted", ip)
		}
	}
	for _, url := range []string{"javascript:alert(1)", "https://example.invalid/", "http://192.0.2.40/action", "http://user@192.0.2.40/", "http://192.0.2.40/?reset=true"} {
		if (Endpoint{URL: url, Scheme: "http", Port: 80}).ValidFor(cameraIP) {
			t.Fatal("invalid endpoint accepted", url)
		}
	}
}
