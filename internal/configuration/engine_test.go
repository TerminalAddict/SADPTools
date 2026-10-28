package configuration

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

// Synthetic device and messages only; this transport never opens a real socket.
const syntheticMAC = "02:00:00:00:00:01"

func testRequest() Request {
	return Request{Action: sadp.UpdateNetwork, MAC: syntheticMAC, Interface: "Ethernet Test", TimeoutSeconds: 2, Password: "Synthetic-pass<>&1", Network: NetworkSettings{IPv4: "192.0.2.10", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", CommandPort: 8000}}
}

type packet struct {
	body   string
	index  int
	source string
}
type commandSocket struct {
	packets                           chan packet
	done                              chan struct{}
	once                              sync.Once
	writes                            atomic.Int32
	send                              func([]byte)
	joinErr, errorSend, errorDeadline error
}

func newSocket() *commandSocket {
	return &commandSocket{packets: make(chan packet, 20), done: make(chan struct{})}
}
func (s *commandSocket) Join(platform.Adapter) error { return s.joinErr }
func (s *commandSocket) Send(_ platform.Adapter, b []byte) error {
	s.writes.Add(1)
	if s.send != nil {
		s.send(b)
	}
	return s.errorSend
}
func (s *commandSocket) Read(b []byte) (int, platform.PacketInfo, error) {
	select {
	case p := <-s.packets:
		return copy(b, p.body), platform.PacketInfo{InterfaceIndex: p.index, Source: p.source}, nil
	case <-s.done:
		return 0, platform.PacketInfo{}, io.EOF
	}
}
func (s *commandSocket) SetDeadline(time.Time) error     { return s.errorDeadline }
func (s *commandSocket) SetReadDeadline(time.Time) error { return nil }
func (s *commandSocket) Close() error                    { s.once.Do(func() { close(s.done) }); return nil }
func testEngine(s *commandSocket, log *slog.Logger) *SADPEngine {
	e := New(log)
	e.interfaces = func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Index: 3, Name: "Ethernet Test", Suitable: true}}, nil
	}
	e.open = func() (platform.Transport, error) { return s, nil }
	return e
}
func response(b []byte, fields string) string {
	var p struct {
		ID   string `xml:"Uuid"`
		Type string `xml:"Types"`
		MAC  string `xml:"MAC"`
	}
	_ = xml.Unmarshal(b, &p)
	return fmt.Sprintf("<ProbeMatch><Uuid>%s</Uuid><MAC>%s</MAC><Types>%s</Types>%s</ProbeMatch>", p.ID, p.MAC, p.Type, fields)
}
func TestTransactionCorrelationAndCredentialIsolation(t *testing.T) {
	s := newSocket()
	var logs bytes.Buffer
	s.send = func(b []byte) {
		good := response(b, "<Result>success</Result><Password>Synthetic-pass&lt;&gt;&amp;1</Password>")
		for _, p := range []packet{{`<ProbeMatch><Password>Synthetic-pass<>&1`, 3, ""}, {good, 99, ""}, {strings.Replace(good, syntheticMAC, "02:00:00:00:00:02", 1), 3, ""}, {strings.Replace(good, "<Uuid>", "<Uuid>wrong-", 1), 3, ""}, {strings.Replace(good, "<Types>update", "<Types>inquiry", 1), 3, ""}, {good, 3, ""}} {
			s.packets <- p
		}
	}
	e := testEngine(s, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	result, err := e.Execute(context.Background(), testRequest())
	if err != nil || result.Outcome != "acknowledged" || s.writes.Load() != 1 {
		t.Fatal(result, err)
	}
	select {
	case <-s.done:
	default:
		t.Fatal("socket left open")
	}
	if strings.Contains(logs.String(), "Synthetic-pass") || strings.Contains(logs.String(), "Password>") {
		t.Fatal("credentials logged")
	}
}
func TestOutcomesNeverAssumeSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, fields, outcome string
		action                sadp.Action
		want                  error
	}{
		{"rejected", "<Result>denied</Result><ErrorCode>2001</ErrorCode>", "rejected", sadp.UpdateNetwork, ErrRejected},
		{"missing status", "<Password>echo</Password>", "unconfirmed", sadp.UpdateNetwork, ErrUnconfirmed},
		{"unknown status", "<Result>SYNTHETIC-PASSWORD</Result>", "unconfirmed", sadp.UpdateNetwork, ErrUnconfirmed},
		{"conflicting code", "<Result>success</Result><ErrorCode>7</ErrorCode>", "rejected", sadp.UpdateNetwork, ErrRejected},
		{"recovery", "<Code>SYNTHETIC-CHALLENGE</Code>", "challenge", sadp.ExchangeCode, nil},
		{"encryption", "<EncryptString>SYNTHETIC-STRING</EncryptString>", "challenge", sadp.EncryptionString, nil},
		{"missing recovery data", "<Result>success</Result>", "unconfirmed", sadp.ExchangeCode, ErrUnconfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) { s.packets <- packet{response(b, tc.fields), 3, ""} }
			r := testRequest()
			r.Action = tc.action
			if r.Action.ReadOnly() {
				r.Password = ""
				r.Network = NetworkSettings{}
			}
			got, err := testEngine(s, nil).Execute(context.Background(), r)
			if got.Outcome != tc.outcome || !errors.Is(err, tc.want) {
				t.Fatal(got, err)
			}
			if strings.Contains(got.Message, "SYNTHETIC-PASSWORD") {
				t.Fatal("device text echoed")
			}
		})
	}
}
func TestCancellationTimeoutAndRepeatedCleanup(t *testing.T) {
	for _, cancelEarly := range []bool{true, false} {
		for i := 0; i < 10; i++ {
			s := newSocket()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			if cancelEarly {
				s.send = func([]byte) { cancel() }
			}
			got, err := testEngine(s, nil).Execute(ctx, testRequest())
			cancel()
			want := context.DeadlineExceeded
			if cancelEarly {
				want = context.Canceled
			}
			if !errors.Is(err, ErrUnconfirmed) || !errors.Is(err, want) || !got.Sent || s.writes.Load() != 1 {
				t.Fatal(got, err)
			}
			select {
			case <-s.done:
			default:
				t.Fatal("socket leaked")
			}
		}
	}
}
func TestValidationAndTransportFailureBeforeSend(t *testing.T) {
	for _, change := range []func(*Request){func(r *Request) { r.MAC = "bad" }, func(r *Request) { r.Interface = "" }, func(r *Request) { r.TimeoutSeconds = 0 }, func(r *Request) { r.Action = "reboot" }, func(r *Request) { r.Password = "" }, func(r *Request) { r.Action = sadp.ResetPassword; r.Code = "" }} {
		s := newSocket()
		e := testEngine(s, nil)
		opened := false
		e.open = func() (platform.Transport, error) { opened = true; return s, nil }
		r := testRequest()
		change(&r)
		got, err := e.Execute(context.Background(), r)
		if err == nil || got.Sent || opened {
			t.Fatal("invalid input opened a socket")
		}
	}
	for _, kind := range []string{"join", "deadline", "open"} {
		s := newSocket()
		e := testEngine(s, nil)
		switch kind {
		case "join":
			s.joinErr = errors.New("synthetic join failure")
		case "deadline":
			s.errorDeadline = errors.New("synthetic deadline failure")
		case "open":
			e.open = func() (platform.Transport, error) { return nil, errors.New("port conflict") }
		}
		got, err := e.Execute(context.Background(), testRequest())
		if err == nil || got.Sent || s.writes.Load() != 0 {
			t.Fatal(got, err)
		}
	}
}
func TestEngineBusyAndCancelledBeforeSend(t *testing.T) {
	s := newSocket()
	e := testEngine(s, nil)
	started := make(chan struct{})
	s.send = func([]byte) { close(started) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _, _ = e.Execute(ctx, testRequest()) }()
	<-started
	if _, err := e.Execute(context.Background(), testRequest()); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	cancel()
	<-done
	got, err := e.Execute(ctx, testRequest())
	if !errors.Is(err, context.Canceled) || got.Sent {
		t.Fatal(got, err)
	}
}
func TestNetworkValidation(t *testing.T) {
	good := NetworkSettings{IPv4: "192.0.2.10", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", CommandPort: 8123}
	if err := ValidateNetwork(good); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*NetworkSettings){func(n *NetworkSettings) { n.IPv4 = "192.0.2.0" }, func(n *NetworkSettings) { n.IPv4 = "192.0.2.255" }, func(n *NetworkSettings) { n.SubnetMask = "255.0.255.0" }, func(n *NetworkSettings) { n.Gateway = "198.51.100.1" }, func(n *NetworkSettings) { n.Gateway = n.IPv4 }, func(n *NetworkSettings) { n.CommandPort = 0 }, func(n *NetworkSettings) { n.DHCP = true; n.IPv4 = "" }} {
		n := good
		change(&n)
		if ValidateNetwork(n) == nil {
			t.Fatal("invalid network accepted", n)
		}
	}
	good.Gateway = "0.0.0.0"
	good.DHCP = true
	if err := ValidateNetwork(good); err != nil {
		t.Fatal(err)
	}
}
func TestRequestLogRedaction(t *testing.T) {
	r := testRequest()
	r.Code = "SYNTHETIC-CODE"
	var b bytes.Buffer
	slog.New(slog.NewJSONHandler(&b, nil)).Info("test", "request", r)
	for _, s := range []string{fmt.Sprintf("%v", r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r), b.String()} {
		if strings.Contains(s, r.Password) || strings.Contains(s, r.Code) {
			t.Fatal("secret leaked")
		}
	}
}

func TestSendFailureIsUnconfirmedAndNeverRetried(t *testing.T) {
	for _, cancelDuringSend := range []bool{false, true} {
		s := newSocket()
		s.errorSend = errors.New("synthetic write failure")
		ctx, cancel := context.WithCancel(context.Background())
		if cancelDuringSend {
			s.send = func([]byte) { cancel() }
		}
		result, err := testEngine(s, nil).Execute(ctx, testRequest())
		cancel()
		if !result.Sent || !errors.Is(err, ErrUnconfirmed) || s.writes.Load() != 1 {
			t.Fatal(result, err)
		}
		if cancelDuringSend && !errors.Is(err, context.Canceled) {
			t.Fatal("lost cancellation", err)
		}
		select {
		case <-s.done:
		default:
			t.Fatal("socket left open")
		}
	}
}
func TestNetworkRejectsIPv6MappedSubnetMask(t *testing.T) {
	if err := ValidateNetwork(NetworkSettings{IPv4: "192.0.2.10", SubnetMask: "::ffff:255.255.255.0", Gateway: "192.0.2.1", CommandPort: 8000}); err == nil {
		t.Fatal("accepted non-IPv4 mask on wire")
	}
}
