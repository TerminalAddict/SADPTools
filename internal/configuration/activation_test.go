package configuration

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

const activationSource = "192.0.2.10:37020"
const activationChallenge = "0123456789abcdef0123456789abcdef"
const activationPassword = "Synthetic<&>1"

func activationRequest() Request {
	return Request{Action: sadp.Activate, MAC: syntheticMAC, Interface: "Ethernet Test", TimeoutSeconds: 2, Password: activationPassword}
}

// Every request is inspected by a synthetic camera. No network is opened.
func activationPacket(t *testing.T, b []byte) (string, packet) {
	t.Helper()
	var wire struct {
		UUID                       string `xml:"Uuid"`
		MAC, Types, Code, Password string
	}
	if err := xml.Unmarshal(b, &wire); err != nil {
		t.Fatal("invalid request XML")
	}
	fields := "<Result>success</Result>"
	switch wire.Types {
	case "inquiry":
		fields = "<MAC>" + syntheticMAC + "</MAC><IPv4Address>192.0.2.10</IPv4Address><Activated>false</Activated>"
	case "exchangecode":
		der, err := base64.StdEncoding.DecodeString(wire.Code)
		if err != nil {
			t.Fatal(err)
		}
		pub, err := x509.ParsePKCS1PublicKey(der)
		if err != nil {
			t.Fatal(err)
		}
		cipher, err := rsa.EncryptPKCS1v15(rand.Reader, pub, []byte(activationChallenge))
		if err != nil {
			t.Fatal(err)
		}
		fields += "<Code>" + base64.StdEncoding.EncodeToString(cipher) + "</Code>"
	case "activate":
		cipher, err := base64.StdEncoding.DecodeString(wire.Password)
		if err != nil || len(cipher) != 32 {
			t.Fatal("password was not encrypted")
		}
		block, _ := aes.NewCipher([]byte(activationChallenge[:16]))
		block.Decrypt(cipher[:16], cipher[:16])
		block.Decrypt(cipher[16:], cipher[16:])
		if string(cipher[:16]) != activationChallenge[:16] || string(bytes.TrimRight(cipher[16:], "\x00")) != activationPassword {
			t.Fatal("wrong encrypted password")
		}
		clear(cipher)
	default:
		t.Fatal("unexpected request", wire.Types)
	}
	if strings.Contains(string(b), activationPassword) {
		t.Fatal("plaintext password sent")
	}
	if wire.Types != "inquiry" && wire.MAC != "02-00-00-00-00-01" {
		t.Fatal("incorrect activation MAC format")
	}
	// Actual key-exchange/activation response shape: no MAC. UUID is unique
	// per stage and the source IP is pinned by the preceding MAC-bearing inquiry.
	return wire.Types, packet{"<ProbeMatch><Uuid>" + wire.UUID + "</Uuid><Types>" + wire.Types + "</Types>" + fields + "</ProbeMatch>", 3, activationSource}
}

func TestEncryptedActivationSequenceAndCorrelation(t *testing.T) {
	s := newSocket()
	var logs bytes.Buffer
	var stages, ids []string
	s.send = func(b []byte) {
		kind, good := activationPacket(t, b)
		stages = append(stages, kind)
		var wire struct {
			UUID string `xml:"Uuid"`
		}
		_ = xml.Unmarshal(b, &wire)
		ids = append(ids, wire.UUID)
		// If any unrelated response is accidentally accepted, it must fail the
		// operation rather than accidentally satisfying the success assertion.
		unrelated := good
		unrelated.body = strings.Replace(unrelated.body, "<Result>success</Result>", "<Result>failed</Result>", 1)
		unrelated.body = strings.Replace(unrelated.body, "<Activated>false</Activated>", "<Activated>true</Activated>", 1)
		bad := unrelated
		bad.index = 99
		s.packets <- bad
		bad = unrelated
		bad.body = strings.Replace(bad.body, "<Uuid>", "<Uuid>stale-", 1)
		s.packets <- bad
		bad = unrelated
		bad.body = strings.Replace(bad.body, "<Types>"+kind, "<Types>unrelated", 1)
		s.packets <- bad
		bad = unrelated
		bad.body = strings.Replace(bad.body, "</ProbeMatch>", "<MAC>02:00:00:00:00:02</MAC></ProbeMatch>", 1)
		s.packets <- bad
		bad = unrelated
		bad.body = strings.Replace(bad.body, "</ProbeMatch>", "<Uuid>duplicate</Uuid></ProbeMatch>", 1)
		s.packets <- bad
		if kind != "inquiry" {
			bad = unrelated
			bad.source = "192.0.2.11:37020"
			s.packets <- bad
			bad = unrelated
			bad.source = ""
			s.packets <- bad
		}
		s.packets <- good
	}
	e := testEngine(s, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	got, err := e.Execute(context.Background(), activationRequest())
	if err != nil || got.Outcome != "acknowledged" || !got.Sent || got.Challenge != "" || s.writes.Load() != 3 || strings.Join(stages, ",") != "inquiry,exchangecode,activate" {
		t.Fatal(got, err, stages)
	}
	if ids[0] == ids[1] || ids[1] == ids[2] || ids[0] == ids[2] {
		t.Fatal("reused request UUID")
	}
	for _, secret := range []string{activationPassword, activationChallenge, "<Code>", "<Password>"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret logged")
		}
	}
	select {
	case <-s.done:
	default:
		t.Fatal("socket leaked")
	}
}

func TestActivationPreparationFailureNeverSendsPassword(t *testing.T) {
	for _, tc := range []struct {
		name, stage, old, new string
		writes                int
	}{
		{"already active", "inquiry", "<Activated>false</Activated>", "<Activated>true</Activated>", 1},
		{"unknown state", "inquiry", "<Activated>false</Activated>", "", 1},
		{"exchange rejected", "exchangecode", "<Result>success</Result>", "<Result>failed</Result>", 2},
		{"bad crypto", "exchangecode", "<Code>", "<Code>not-base64-", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) {
				kind, p := activationPacket(t, b)
				if kind == "activate" {
					t.Fatal("activation sent after failed preparation")
				}
				if kind == tc.stage {
					p.body = strings.Replace(p.body, tc.old, tc.new, 1)
				}
				s.packets <- p
			}
			got, err := testEngine(s, nil).Execute(context.Background(), activationRequest())
			if err == nil || got.Sent || got.Outcome != "not_sent" || got.Challenge != "" || !strings.Contains(got.Message, "Activation was not sent") || s.writes.Load() != int32(tc.writes) {
				t.Fatal(got, err)
			}
			select {
			case <-s.done:
			default:
				t.Fatal("socket leaked")
			}
		})
	}
}

func TestActivationCancellationAndSendFailuresAtEachStage(t *testing.T) {
	for _, stage := range []string{"inquiry", "exchangecode", "activate"} {
		for _, failure := range []string{"cancel", "send error"} {
			t.Run(stage+"/"+failure, func(t *testing.T) {
				s := newSocket()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s.send = func(b []byte) {
					kind, p := activationPacket(t, b)
					if kind == stage {
						if failure == "cancel" {
							cancel()
						} else {
							s.errorSend = errors.New("secret-error-must-not-leak")
						}
						return
					}
					s.packets <- p
				}
				got, err := testEngine(s, nil).Execute(ctx, activationRequest())
				if err == nil || got.Sent != (stage == "activate") || errors.Is(err, ErrUnconfirmed) != (stage == "activate") || strings.Contains(err.Error(), "secret-error") {
					t.Fatal(got, err)
				}
				if failure == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal("lost cancellation")
				}
				want := map[string]int32{"inquiry": 1, "exchangecode": 2, "activate": 3}[stage]
				if s.writes.Load() != want {
					t.Fatal("retried a request")
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

func TestActivationLostAcknowledgementAndRepeatedCleanup(t *testing.T) {
	for i := 0; i < 3; i++ {
		s := newSocket()
		s.send = func(b []byte) {
			kind, p := activationPacket(t, b)
			if kind != "activate" {
				s.packets <- p
			}
		}
		// Leave room for RSA key generation on slower/race-enabled CI runners;
		// the deadline still exercises real cancellation and socket cleanup.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		got, err := testEngine(s, nil).Execute(ctx, activationRequest())
		cancel()
		if !errors.Is(err, ErrUnconfirmed) || !errors.Is(err, context.DeadlineExceeded) || !got.Sent || s.writes.Load() != 3 {
			t.Fatal(got, err)
		}
		select {
		case <-s.done:
		default:
			t.Fatal("socket leaked")
		}
	}
}

func TestActivationPasswordLimitBeforeOpeningSocket(t *testing.T) {
	r := activationRequest()
	r.Password = strings.Repeat("Aa1", 6)
	s := newSocket()
	got, err := testEngine(s, nil).Execute(context.Background(), r)
	if err == nil || got.Sent || s.writes.Load() != 0 {
		t.Fatal("long activation password accepted")
	}
}

func TestActivationRecognizedAndUnknownResults(t *testing.T) {
	for _, tc := range []struct {
		status, outcome, message string
		want                     error
	}{
		{"riskPassword", "rejected", "too weak", ErrRejected},
		{"hasActivated", "rejected", "already active", ErrRejected},
		{"failed", "rejected", "rejected", ErrRejected},
		{"unknown-SYNTHETIC-SECRET", "unconfirmed", "No conclusive reply", ErrUnconfirmed},
	} {
		t.Run(tc.status, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) {
				kind, p := activationPacket(t, b)
				if kind == "activate" {
					p.body = strings.Replace(p.body, "<Result>success</Result>", "<Result>"+tc.status+"</Result>", 1)
				}
				s.packets <- p
			}
			got, err := testEngine(s, nil).Execute(context.Background(), activationRequest())
			if !errors.Is(err, tc.want) || got.Outcome != tc.outcome || !strings.Contains(got.Message, tc.message) || !got.Sent || s.writes.Load() != 3 || strings.Contains(got.Message, "SYNTHETIC-SECRET") {
				t.Fatal(got, err)
			}
		})
	}
}
