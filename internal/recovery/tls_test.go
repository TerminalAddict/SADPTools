package recovery

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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type endpointFixture struct{ endpoint webinterface.Endpoint }

func (f endpointFixture) Detect(context.Context, webinterface.Target) (webinterface.Result, error) {
	return webinterface.Result{Endpoint: f.endpoint, Message: "Synthetic endpoint"}, nil
}

func TestRecoveryRequiresExplicitTrustAndKeepsDetectedEndpoint(t *testing.T) {
	for _, mode := range []string{"no trust", "wrong pin", "matching pin", "system trust", "https unavailable"} {
		t.Run(mode, func(t *testing.T) {
			pin := strings.Repeat("ab", 32)
			endpoint := webinterface.Endpoint{URL: "https://192.0.2.40:8443/", Scheme: "https", Port: 8443, CertificateSHA256: pin}
			target := fixtureTarget
			wantReady := mode == "matching pin" || mode == "system trust"
			switch mode {
			case "matching pin":
				target.CertificateSHA256 = pin
			case "wrong pin":
				target.CertificateSHA256 = strings.Repeat("cd", 32)
			case "system trust":
				endpoint.CertificateTrusted = true
			case "https unavailable":
				target.CertificateSHA256 = pin
				endpoint = webinterface.Endpoint{URL: "http://192.0.2.40/", Scheme: "http", Port: 80}
			}
			f := &fixture{t: t}
			client := f.client()
			client.detector = endpointFixture{endpoint}
			factory := client.newHTTP
			connections := 0
			client.newHTTP = func(resolved Target) *http.Client {
				connections++
				if resolved.Scheme != "https" || resolved.Port != 8443 || resolved.CertificateSHA256 != target.CertificateSHA256 || resolved.Source != target.Source {
					t.Fatal("detected connection changed", resolved)
				}
				return factory(resolved)
			}
			f.change = func(r *http.Request) (*http.Response, error, bool) {
				if r.URL.Scheme != "https" || r.URL.Host != "192.0.2.40:8443" {
					t.Fatal("request used a different endpoint", r.URL)
				}
				return nil, nil, false
			}
			status, err := client.Check(context.Background(), target)
			if err != nil || status.Endpoint != endpoint {
				t.Fatal(status, err)
			}
			if wantReady {
				if status.State != "ready" || connections != 1 {
					t.Fatal(status, connections)
				}
				result, err := client.Reset(context.Background(), target, fixtureRequest())
				if err != nil || result.Outcome != "acknowledged" || f.puts != 1 || f.posts != 1 || connections != 2 {
					t.Fatal(result, err)
				}
			} else {
				if status.State != "certificate_required" || connections != 0 {
					t.Fatal(status, connections)
				}
				if _, err := client.Reset(context.Background(), target, fixtureRequest()); err == nil {
					t.Fatal("reset without accepted TLS certificate")
				}
				if connections != 0 || len(f.paths) != 0 {
					t.Fatal("sent recovery requests before trust confirmation")
				}
			}
		})
	}
}

func TestRecoveryTransportVerifiesEveryTLSConnection(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(200) }))
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.StartTLS()
	defer s.Close()
	hash := sha256.Sum256(s.Certificate().Raw)
	for _, mode := range []string{"untrusted", "accepted", "changed"} {
		t.Run(mode, func(t *testing.T) {
			target := fixtureTarget
			target.Scheme, target.Port = "https", 8443
			if mode == "accepted" {
				target.CertificateSHA256 = hex.EncodeToString(hash[:])
			}
			if mode == "changed" {
				target.CertificateSHA256 = strings.Repeat("00", 32)
			}
			client := localHTTP(target)
			defer client.CloseIdleConnections()
			client.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
			}
			before := requests.Load()
			// DisableKeepAlives forces a fresh handshake for each API request.
			for range 2 {
				_, _, err := request(context.Background(), client, target, http.MethodGet, capabilitiesPath, nil)
				if mode == "accepted" && err != nil {
					t.Fatal(err)
				}
				if mode != "accepted" && err == nil {
					t.Fatal("untrusted connection accepted")
				}
				if mode == "changed" && !errors.Is(err, errCertificateChanged) {
					t.Fatal(err)
				}
			}
			want := int32(0)
			if mode == "accepted" {
				want = 2
			}
			if requests.Load()-before != want {
				t.Fatal("unexpected requests before TLS verification", requests.Load()-before)
			}
		})
	}
}
