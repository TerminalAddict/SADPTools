package configuration

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

// Entirely synthetic camera; no sockets or physical writes in these tests.
func questionBody(id string, configured bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<ProbeMatch><Uuid>%s</Uuid><Types>getsecurityquestion</Types><Result>success</Result><QuestionList>", id)
	for i := 1; i <= 15; i++ {
		fmt.Fprintf(&b, "<Question><ID>%d</ID><mark>%t</mark></Question>", i, configured && i <= 3)
	}
	b.WriteString("</QuestionList></ProbeMatch>")
	return b.String()
}
func questionRequest() Request {
	_, questions, err := sadp.ParseSecurityQuestions([]byte(questionBody("synthetic", false)))
	if err != nil {
		panic(err)
	}
	return Request{Action: sadp.SetSecurityQuestions, MAC: syntheticMAC, Interface: "Ethernet Test", TimeoutSeconds: 3, Password: activationPassword,
		QuestionRevision: sadp.QuestionRevision(syntheticMAC+"\x00\x00\x00\x00Ethernet Test", questions),
		Answers:          []sadp.SecurityAnswer{{ID: 1, Answer: "Synthetic answer"}, {ID: 2, Answer: " Another answer "}, {ID: 3, Answer: "Synthetic 漢字"}}}
}
func questionPacket(t *testing.T, b []byte, configured bool) (string, packet) {
	t.Helper()
	var w struct {
		UUID            string `xml:"Uuid"`
		Types, Password string
		Questions       []struct {
			ID     int
			Answer string
			Mark   bool `xml:"mark"`
		} `xml:"QuestionList>Question"`
	}
	if err := xml.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	switch w.Types {
	case "inquiry":
		kind, p := activationPacket(t, b)
		p.body = strings.Replace(p.body, "<Activated>false</Activated>", "<Activated>true</Activated><SupportSecurityQuestion>true</SupportSecurityQuestion>", 1)
		return kind, p
	case "exchangecode":
		return activationPacket(t, b)
	case "getsecurityquestion":
		return w.Types, packet{questionBody(w.UUID, configured), 3, activationSource}
	case "setsecurityquestion":
		block, _ := aes.NewCipher([]byte(activationChallenge[:16]))
		decrypt := func(s string) []byte {
			v, err := base64.StdEncoding.DecodeString(s)
			if err != nil || len(v)%16 != 0 || len(v) == 0 {
				t.Fatal("unencrypted setup field")
			}
			for i := 0; i < len(v); i += 16 {
				block.Decrypt(v[i:i+16], v[i:i+16])
			}
			return v
		}
		pw := decrypt(w.Password)
		defer clear(pw)
		if len(pw) != 32 || string(pw[:16]) != activationChallenge[:16] || string(bytes.TrimRight(pw[16:], "\x00")) != activationPassword {
			t.Fatal("invalid setup password encoding")
		}
		if len(w.Questions) != 3 {
			t.Fatal("wrong question count")
		}
		for i, q := range w.Questions {
			v := decrypt(q.Answer)
			if !q.Mark || q.ID != i+1 || string(bytes.TrimRight(v, "\x00")) != questionRequest().Answers[i].Answer {
				t.Fatal("answer changed or wrong encoding")
			}
			clear(v)
		}
		return w.Types, packet{response(b, "<Result>success</Result>"), 3, activationSource}
	default:
		t.Fatal("unexpected operation", w.Types)
		return "", packet{}
	}
}

func TestSecuritySetupSequenceReadbackAndIsolation(t *testing.T) {
	s := newSocket()
	configured := false
	stages := []string{}
	var logs bytes.Buffer
	s.send = func(b []byte) {
		kind, p := questionPacket(t, b, configured)
		stages = append(stages, kind)
		if kind == "setsecurityquestion" {
			configured = true
		}
		if strings.Contains(string(b), "Synthetic") || strings.Contains(string(b), " Another answer ") {
			t.Fatal("plaintext credential on wire")
		}
		if kind == "getsecurityquestion" {
			for _, change := range []func(*packet){func(p *packet) { p.index = 99 }, func(p *packet) { p.source = "192.0.2.99:37020" }, func(p *packet) { p.body = strings.Replace(p.body, "<Uuid>", "<Uuid>stale-", 1) }, func(p *packet) {
				p.body = strings.Replace(p.body, "</ProbeMatch>", "<MAC>02:00:00:00:00:99</MAC></ProbeMatch>", 1)
			}} {
				bad := p
				change(&bad)
				s.packets <- bad
			}
		}
		s.packets <- p
	}
	engine := testEngine(s, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	engine.interfaces = func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Index: 3, Name: "Ethernet Test", Suitable: true, Addresses: []string{"198.51.100.8/24"}}}, nil
	}
	r, err := engine.Execute(context.Background(), questionRequest())
	if err != nil || !r.Sent || r.Outcome != "verified" || strings.Join(stages, ",") != "inquiry,getsecurityquestion,exchangecode,setsecurityquestion,getsecurityquestion" {
		t.Fatal(r.Outcome, err, stages)
	}
	if r.ObservedDevice == nil || len(r.ObservedDevice.Interfaces) != 1 || r.ObservedDevice.Interfaces[0].Address != "198.51.100.8" {
		t.Fatal("observation must retain the host adapter address, not the camera's address")
	}
	data, _ := json.Marshal(r)
	for _, secret := range []string{activationPassword, " Another answer ", activationChallenge, "<Probe"} {
		if strings.Contains(logs.String()+string(data), secret) {
			t.Fatal("secret/raw packet exposed")
		}
	}
	select {
	case <-s.done:
	default:
		t.Fatal("socket leaked")
	}
}

func TestSecuritySetupReadOnlyAndPreflight(t *testing.T) {
	for _, tc := range []struct{ name, old, new, outcome string }{
		{"ready", "", "", "questions_ready"},
		{"inactive", "<Activated>true", "<Activated>false", "inactive"},
		{"unsupported", "<SupportSecurityQuestion>true", "<SupportSecurityQuestion>false", "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) {
				kind, p := questionPacket(t, b, false)
				if kind != "inquiry" && kind != "getsecurityquestion" {
					t.Fatal("check sent a write/key exchange")
				}
				if tc.old != "" {
					p.body = strings.Replace(p.body, tc.old, tc.new, 1)
				}
				s.packets <- p
			}
			r := questionRequest()
			r.Action = sadp.GetSecurityQuestions
			r.Password = ""
			r.Answers = nil
			r.QuestionRevision = ""
			got, err := testEngine(s, nil).Execute(context.Background(), r)
			if err != nil || got.Sent || got.Outcome != tc.outcome || got.ObservedDevice == nil {
				t.Fatal(got.Outcome, err)
			}
		})
	}
	for _, which := range []string{"stale", "replacement", "bad answer", "long password"} {
		t.Run(which, func(t *testing.T) {
			s := newSocket()
			s.send = func(b []byte) {
				kind, p := questionPacket(t, b, which == "replacement")
				if kind == "exchangecode" || kind == "setsecurityquestion" {
					t.Fatal("failed review negotiated or wrote")
				}
				s.packets <- p
			}
			r := questionRequest()
			switch which {
			case "stale":
				r.QuestionRevision = strings.Repeat("f", 64)
			case "replacement":
				_, qs, _ := sadp.ParseSecurityQuestions([]byte(questionBody("synthetic", true)))
				r.QuestionRevision = sadp.QuestionRevision(syntheticMAC+"\x00\x00\x00\x00Ethernet Test", qs)
			case "bad answer":
				r.Answers[0].Answer = ""
			case "long password":
				r.Password = strings.Repeat("x", 17)
			}
			got, err := testEngine(s, nil).Execute(context.Background(), r)
			if err == nil || got.Sent || got.Outcome != "not_sent" {
				t.Fatal(got.Outcome, err)
			}
		})
	}
}

func TestSecuritySetupRejectedOrUncertainNeverRetries(t *testing.T) {
	for _, status := range []string{"PasswordError", "locked", "LongAnswer", "QuestionNumErr", "failed", "unexpected", "lost reply", "wrong readback"} {
		t.Run(status, func(t *testing.T) {
			s := newSocket()
			writes := 0
			s.send = func(b []byte) {
				kind, p := questionPacket(t, b, false)
				if kind == "setsecurityquestion" {
					writes++
					if status == "lost reply" {
						return
					}
					if status != "wrong readback" {
						p.body = strings.Replace(p.body, "<Result>success", "<Result>"+status, 1)
					}
				}
				s.packets <- p
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := testEngine(s, nil).Execute(ctx, questionRequest())
			want := ErrRejected
			if status == "unexpected" || status == "lost reply" || status == "wrong readback" {
				want = ErrUnconfirmed
			}
			if !got.Sent || !errors.Is(err, want) || writes != 1 {
				t.Fatal(got.Outcome, err, writes)
			}
		})
	}
	for _, stage := range []string{"inquiry", "getsecurityquestion", "exchangecode", "setsecurityquestion"} {
		t.Run("cancel/"+stage, func(t *testing.T) {
			s := newSocket()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.send = func(b []byte) {
				kind, p := questionPacket(t, b, false)
				if kind == stage {
					cancel()
					return
				}
				s.packets <- p
			}
			got, err := testEngine(s, nil).Execute(ctx, questionRequest())
			if !errors.Is(err, context.Canceled) || got.Sent != (stage == "setsecurityquestion") {
				t.Fatal(got.Outcome, err)
			}
			select {
			case <-s.done:
			default:
				t.Fatal("socket leaked")
			}
		})
	}
}
