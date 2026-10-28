package configuration

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

const networkTestSalt = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const networkTestPeer = "192.0.2.10:37020"

func networkRequest() Request {
	return Request{Action: sadp.UpdateNetwork, MAC: syntheticMAC, Interface: "Ethernet Test", TimeoutSeconds: 2,
		Password: "Synthetic<&>1", Network: NetworkSettings{IPv4: "192.0.2.10", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", CommandPort: 8000, DHCP: true}}
}

func networkPacket(t *testing.T, b []byte) (string, packet) {
	t.Helper()
	var p struct {
		ID       string `xml:"Uuid"`
		Type     string `xml:"Types"`
		MAC      string `xml:"MAC"`
		Password string
	}
	if err := xml.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	fields := "<Result>success</Result>"
	switch p.Type {
	case "inquiry":
		fields = "<MAC>" + syntheticMAC + "</MAC><IPv4Address>192.0.2.10</IPv4Address><Activated>true</Activated><Encrypt>true</Encrypt><Salt>" + networkTestSalt + "</Salt><HttpPort>8080</HttpPort>"
	case "update":
		if p.MAC != "02-00-00-00-00-01" || len(p.Password) != 88 || strings.Contains(string(b), "Synthetic") || strings.Contains(string(b), networkTestSalt) || !strings.Contains(string(b), `bSalt="true"`) || !strings.Contains(string(b), "<HttpPort>8080</HttpPort>") {
			t.Fatal("unsafe network payload")
		}
	default:
		t.Fatalf("unexpected command %q", p.Type)
	}
	return p.Type, packet{fmt.Sprintf("<ProbeMatch><Uuid>%s</Uuid><Types>%s</Types>%s</ProbeMatch>", p.ID, p.Type, fields), 3, networkTestPeer}
}

func TestNetworkFreshAuthenticationCorrelationAndIsolation(t *testing.T) {
	s := newSocket()
	var stages []string
	var logs bytes.Buffer
	s.send = func(b []byte) {
		kind, good := networkPacket(t, b)
		stages = append(stages, kind)
		for _, change := range []func(*packet){
			func(p *packet) { p.index = 99 },
			func(p *packet) { p.body = strings.Replace(p.body, "<Uuid>", "<Uuid>wrong-", 1) },
			func(p *packet) { p.body = strings.Replace(p.body, "<Types>", "<Types>wrong-", 1) },
			func(p *packet) { p.source = "198.51.100.99:37020" },
			func(p *packet) { p.source = "0.0.0.0:37020" },
			func(p *packet) { p.body = strings.Replace(p.body, "</ProbeMatch>", "<Password>malformed", 1) },
			func(p *packet) {
				p.body = strings.Replace(p.body, "</ProbeMatch>", "<MAC>02:00:00:00:00:99</MAC></ProbeMatch>", 1)
			},
		} {
			p := good
			change(&p)
			s.packets <- p
		}
		s.packets <- good
	}
	e := testEngine(s, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	r, err := e.Execute(context.Background(), networkRequest())
	if err != nil || !r.Sent || r.Outcome != "acknowledged" || strings.Join(stages, ",") != "inquiry,update" || s.writes.Load() != 2 {
		t.Fatal(r, err, stages)
	}
	for _, secret := range []string{"Synthetic", networkTestSalt, "<ProbeMatch>"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret/raw packet logged")
		}
	}
	select {
	case <-s.done:
	default:
		t.Fatal("socket leaked")
	}
}

func TestNetworkPreparationFailuresNeverWrite(t *testing.T) {
	for _, tc := range []struct{ name, old, new string }{
		{"inactive", "<Activated>true", "<Activated>false"},
		{"missing authentication", "<Encrypt>true</Encrypt>", ""},
		{"bad salt", networkTestSalt, "unsupported"},
		{"missing unrelated port", "<HttpPort>8080</HttpPort>", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) {
				kind, p := networkPacket(t, b)
				if kind != "inquiry" {
					t.Fatal("unexpected write")
				}
				p.body = strings.Replace(p.body, tc.old, tc.new, 1)
				s.packets <- p
			}
			r, err := testEngine(s, nil).Execute(context.Background(), networkRequest())
			if err == nil || r.Sent || r.Outcome != "not_sent" || s.writes.Load() != 1 || !strings.Contains(r.Message, "Network change was not sent") {
				t.Fatal(r, err)
			}
		})
	}
}

func TestNetworkRejectionsAndAddressChangingReplies(t *testing.T) {
	for _, tc := range []struct {
		name, status, source, mac, outcome string
		dhcp                               bool
		want                               error
	}{
		{"incorrect password", "PasswordError", networkTestPeer, "", "rejected", true, ErrRejected},
		{"lockout", "locked", networkTestPeer, "", "rejected", true, ErrRejected},
		{"inactive", "notActivated", networkTestPeer, "", "rejected", true, ErrRejected},
		{"dhcp new address with identity", "success", "198.51.100.20:37020", syntheticMAC, "acknowledged", true, nil},
		{"static new address without MAC", "success", "192.0.2.20:37020", "", "acknowledged", false, nil},
		{"unknown response", "unexpected", networkTestPeer, "", "unconfirmed", true, ErrUnconfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) {
				kind, p := networkPacket(t, b)
				if kind == "update" {
					p.source = tc.source
					p.body = strings.Replace(p.body, "<Result>success</Result>", "<Result>"+tc.status+"</Result>", 1)
					if tc.mac != "" {
						p.body = strings.Replace(p.body, "</ProbeMatch>", "<MAC>"+tc.mac+"</MAC></ProbeMatch>", 1)
					}
				}
				s.packets <- p
			}
			req := networkRequest()
			req.Network.DHCP = tc.dhcp
			req.Network.IPv4 = "192.0.2.20"
			r, err := testEngine(s, nil).Execute(context.Background(), req)
			if !errors.Is(err, tc.want) || r.Outcome != tc.outcome || !r.Sent || s.writes.Load() != 2 {
				t.Fatal(r, err)
			}
		})
	}
}

func TestNetworkCancellationTimeoutAndSendFailureDoNotRetry(t *testing.T) {
	for _, stage := range []string{"inquiry", "update"} {
		for _, failure := range []string{"cancel", "timeout", "send"} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				s := newSocket()
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				s.send = func(b []byte) {
					kind, p := networkPacket(t, b)
					if kind == stage {
						switch failure {
						case "cancel":
							cancel()
						case "send":
							s.errorSend = errors.New("synthetic transport failure")
						}
						return
					}
					s.packets <- p
				}
				r, err := testEngine(s, nil).Execute(ctx, networkRequest())
				wantWrites := int32(1)
				if stage == "update" {
					wantWrites = 2
				}
				if err == nil || r.Sent != (stage == "update") || s.writes.Load() != wantWrites || errors.Is(err, ErrUnconfirmed) != (stage == "update") {
					t.Fatal(r, err)
				}
				select {
				case <-s.done:
				default:
					t.Fatal("socket leaked")
				}
			})
		}
	}
}
