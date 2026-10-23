package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/recovery"
)

type recoveryFixture struct {
	checks, resets int
	reset          func(context.Context, recovery.Target, recovery.Request) (recovery.Result, error)
}

func (f *recoveryFixture) Check(context.Context, recovery.Target) (recovery.Status, error) {
	f.checks++
	return recovery.Status{State: "ready", Questions: []recovery.Question{{ID: 2, Text: "Synthetic A?"}, {ID: 7, Text: "Synthetic B?"}, {ID: 9, Text: "Synthetic C?"}}}, nil
}
func (f *recoveryFixture) Reset(ctx context.Context, target recovery.Target, r recovery.Request) (recovery.Result, error) {
	f.resets++
	if f.reset != nil {
		return f.reset(ctx, target, r)
	}
	return recovery.Result{Sent: true, Outcome: "acknowledged"}, nil
}
func questionController(t *testing.T) (*Controller, string, *recoveryFixture, recovery.Request) {
	c, id, _ := configController(t)
	port := 80
	active := true
	c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", IPv4: "192.0.2.40", HTTPPort: &port, Activated: &active}, time.Now())
	c.recoveryInterfaces = func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Name: "Ethernet Test", Suitable: true, UnicastSuitable: true, Addresses: []string{"192.0.2.10/24"}}}, nil
	}
	f := &recoveryFixture{}
	c.questionRecovery = f
	r := recovery.Request{Password: "Synthetic-pass1", Answers: []recovery.Answer{{ID: 2, Answer: "synthetic one"}, {ID: 7, Answer: "synthetic two"}, {ID: 9, Answer: "synthetic three"}}}
	return c, id, f, r
}
func TestQuestionCheckBindsOneSubmissionToDeviceAndConnection(t *testing.T) {
	for _, kind := range []string{"success", "unchecked", "token", "address", "source", "expired", "question"} {
		t.Run(kind, func(t *testing.T) {
			c, id, f, r := questionController(t)
			if kind != "unchecked" {
				s, err := c.CheckSecurityQuestions(id, "")
				if err != nil || s.CheckID == "" {
					t.Fatal(s, err)
				}
				r.CheckID = s.CheckID
			}
			switch kind {
			case "token":
				r.CheckID = "stale-check"
			case "address":
				c.inventory.Upsert(devices.Device{MAC: "02:00:00:00:00:01", IPv4: "192.0.2.41"}, time.Now())
			case "source":
				c.recoveryInterfaces = func() ([]platform.Adapter, error) {
					return []platform.Adapter{{Name: "Ethernet Test", UnicastSuitable: true, Addresses: []string{"198.51.100.10/24"}}}, nil
				}
			case "expired":
				c.mu.Lock()
				c.recoveryCheck.expires = time.Now().Add(-time.Second)
				c.mu.Unlock()
			case "question":
				r.Answers[0].ID = 4
			}
			result, err := c.ResetWithSecurityQuestions(id, "", r)
			if kind == "success" {
				if err != nil || result.Outcome != "acknowledged" || f.resets != 1 {
					t.Fatal(result, err)
				}
				if _, err := c.ResetWithSecurityQuestions(id, "", r); err == nil || f.resets != 1 {
					t.Fatal("reused a consumed check")
				}
			} else if err == nil || f.resets != 0 {
				t.Fatal("unverified target received reset")
			}
			data, _ := json.Marshal(c.Snapshot())
			if strings.Contains(string(data), r.Password) || strings.Contains(string(data), "synthetic one") || (r.CheckID != "" && strings.Contains(string(data), r.CheckID)) {
				t.Fatal("private recovery inputs retained in snapshot/logs")
			}
		})
	}
}
func TestQuestionRecoveryCancelsAndBlocksOtherOperations(t *testing.T) {
	c, id, f, r := questionController(t)
	s, err := c.CheckSecurityQuestions(id, "")
	if err != nil {
		t.Fatal(err)
	}
	r.CheckID = s.CheckID
	started := make(chan struct{})
	done := make(chan struct{})
	f.reset = func(ctx context.Context, _ recovery.Target, _ recovery.Request) (recovery.Result, error) {
		close(started)
		<-ctx.Done()
		return recovery.Result{Sent: true, Outcome: "unconfirmed"}, errors.Join(configuration.ErrUnconfirmed, ctx.Err())
	}
	go func() {
		defer close(done)
		result, err := c.ResetWithSecurityQuestions(id, "", r)
		if err != nil || result.Outcome != "unconfirmed" {
			t.Error(result, err)
		}
	}()
	<-started
	if err := c.StartScan(); !errors.Is(err, configuration.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := c.CheckSecurityQuestions(id, ""); !errors.Is(err, configuration.ErrBusy) {
		t.Fatal(err)
	}
	if err := c.LaunchUpdate(func(Settings) error { t.Fatal("update launched during reset"); return nil }); err == nil {
		t.Fatal(err)
	}
	c.Close()
	<-done
	if c.Snapshot().Configuring || f.resets != 1 {
		t.Fatal("operation not joined")
	}
	if _, err := c.CheckSecurityQuestions(id, ""); err == nil {
		t.Fatal("check after shutdown")
	}
}

func TestQuestionRecoveryOnRoutedCamera(t *testing.T) {
	c, id, f, r := questionController(t)
	c.recoveryInterfaces = func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Name: "Ethernet Test", UnicastSuitable: true, Addresses: []string{"198.51.100.10/24"}}}, nil
	}
	status, err := c.CheckSecurityQuestions(id, "")
	if err != nil || status.State != "ready" || status.CheckID == "" || f.checks != 1 {
		t.Fatal(status, err)
	}
	r.CheckID = status.CheckID
	f.reset = func(_ context.Context, target recovery.Target, _ recovery.Request) (recovery.Result, error) {
		if target.Source != "198.51.100.10" || target.Address != "192.0.2.40" || target.Interface != "Ethernet Test" {
			t.Fatal("routed connection changed", target)
		}
		return recovery.Result{Sent: true, Outcome: "acknowledged"}, nil
	}
	result, err := c.ResetWithSecurityQuestions(id, "", r)
	if err != nil || result.Outcome != "acknowledged" || f.resets != 1 {
		t.Fatal(result, err)
	}
}
