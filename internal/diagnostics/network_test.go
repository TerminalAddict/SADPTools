package diagnostics

import (
	"context"
	"crypto/x509"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixtureNetwork() *Network {
	n := NewNetwork()
	n.RouteDetail = nil
	n.Route = func(string, int) (string, error) { return "127.0.0.1", nil }
	n.Interfaces = func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Name: "Synthetic adapter", Addresses: []string{"127.0.0.1/8"}, UnicastSuitable: true}}, nil
	}
	n.Probes.Neighbours = func() ([]ipscan.Neighbour, error) { return nil, nil }
	n.Probes.Ping = func(context.Context, string, string, time.Duration) ipscan.Evidence {
		return ipscan.Evidence{State: "unsupported", Detail: "Synthetic permission denied"}
	}
	return n
}
func TestNetworkHTTPAndNoCredentialRedirect(t *testing.T) {
	var redirected bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected credential")
		}
		w.Header().Set("Location", other.URL)
		w.WriteHeader(302)
	}))
	defer server.Close()
	_, p, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(p)
	r, e := fixtureNetwork().Run(context.Background(), Options{Target: "127.0.0.1", Ports: []int{port}, HTTPPorts: []int{port}, TimeoutMS: 500, ICMP: true}, nil)
	if e != nil || redirected {
		t.Fatal(e, redirected)
	}
	last := r.Steps[len(r.Steps)-1]
	if last.Code != 302 || !strings.Contains(last.Evidence, "redirect refused") {
		t.Fatal(last)
	}
	if !strings.Contains(r.Summary(), "does not prove") {
		t.Fatal("absence caveat missing")
	}
}
func TestDistinctConnectionFailures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{{syscall.ECONNREFUSED, "refused"}, {syscall.EHOSTUNREACH, "unreachable"}, {context.DeadlineExceeded, "timed out"}, {x509.HostnameError{}, "mismatch"}, {x509.UnknownAuthorityError{}, "not trusted"}, {x509.CertificateInvalidError{Reason: x509.Expired}, "expired"}} {
		got, _ := connectionFailure(tc.err)
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%v: %s", tc.err, got)
		}
	}
}
func TestDNSRouteAdapterAndCancellation(t *testing.T) {
	o := Options{Target: "synthetic.invalid", Ports: []int{80}, TimeoutMS: 200}
	n := fixtureNetwork()
	n.Resolver = func(context.Context, string) ([]net.IP, error) { return nil, errors.New("synthetic DNS") }
	r, e := n.Run(context.Background(), o, nil)
	if e != nil || len(r.Steps) != 1 || r.Steps[0].Protocol != "dns" {
		t.Fatal(r, e)
	}
	o.Target = "127.0.0.1"
	n.Route = func(string, int) (string, error) { return "", errors.New("synthetic no route") }
	r, e = n.Run(context.Background(), o, nil)
	if e != nil || r.Steps[0].Status != "failed" {
		t.Fatal(r, e)
	}
	n = fixtureNetwork()
	o.Interface = "missing"
	if _, e = n.Run(context.Background(), o, nil); e == nil {
		t.Fatal("missing adapter accepted")
	}
	o.Interface = ""
	n.Dial = func(ctx context.Context, _, _ string, _ time.Duration) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { _, e := n.Run(ctx, o, nil); done <- e }()
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation leaked")
	}
}
func TestTargetPortValidation(t *testing.T) {
	for _, target := range []string{"http://127.0.0.1", "name@host", "127.0.0.1/24", "x\n--command", ""} {
		if (Options{Target: target, Ports: []int{80}, TimeoutMS: 200}).Validate() == nil {
			t.Fatal(target)
		}
	}
	for _, ip := range []string{"0.0.0.0", "224.0.0.1", "255.255.255.255", "::1"} {
		if usable(netip.MustParseAddr(ip)) {
			t.Fatal(ip)
		}
	}
}
