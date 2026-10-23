package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/recovery"
)

type questionEngine struct {
	resets  int
	state   string
	failure error
}

func (e *questionEngine) Check(context.Context, recovery.Target) (recovery.Status, error) {
	return recovery.Status{State: e.state, Questions: []recovery.Question{{ID: 2, Text: "Synthetic question?"}}}, nil
}
func (e *questionEngine) Reset(context.Context, recovery.Target, recovery.Request) (recovery.Result, error) {
	e.resets++
	return recovery.Result{Sent: true, Outcome: "unconfirmed", Message: "Synthetic uncertain outcome"}, e.failure
}
func TestRecoveryCLIExplicitSubmissionAndPrivateInput(t *testing.T) {
	interfaces := func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Name: "Ethernet Fixture", Suitable: true, UnicastSuitable: true, Addresses: []string{"192.0.2.10/24"}}}, nil
	}
	secrets := `{"answers":[{"id":2,"answer":"SYNTHETIC-PRIVATE-A"},{"id":7,"answer":"SYNTHETIC-PRIVATE-B"},{"id":9,"answer":"SYNTHETIC-PRIVATE-C"}],"password":"SYNTHETIC-private1"}`
	for _, tc := range []struct {
		name        string
		args        []string
		input       string
		want, calls int
	}{
		{"check", []string{"questions", "192.0.2.40", "--interface", "Ethernet Fixture", "--json"}, "", 0, 0},
		{"routed check", []string{"questions", "198.51.100.40", "--interface", "Ethernet Fixture", "--json"}, "", 0, 0},
		{"no explicit confirmation", []string{"reset", "192.0.2.40", "--interface", "Ethernet Fixture", "--secrets-stdin"}, secrets, 2, 0},
		{"invalid secret JSON", []string{"reset", "192.0.2.40", "--interface", "Ethernet Fixture", "--secrets-stdin", "--confirm-reset"}, `{"password":"SYNTHETIC-PRIVATE",`, 2, 0},
		{"unconfirmed", []string{"reset", "192.0.2.40", "--interface", "Ethernet Fixture", "--secrets-stdin", "--confirm-reset", "--json"}, secrets, 5, 1},
		{"routed reset", []string{"reset", "198.51.100.40", "--interface", "Ethernet Fixture", "--secrets-stdin", "--confirm-reset"}, secrets, 5, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &questionEngine{state: "ready", failure: errors.Join(configuration.ErrUnconfirmed, context.Canceled)}
			var out, errOut bytes.Buffer
			code := runRecovery(context.Background(), tc.args, strings.NewReader(tc.input), &out, &errOut, engine, interfaces)
			if code != tc.want || engine.resets != tc.calls {
				t.Fatal(code, engine.resets, out.String(), errOut.String())
			}
			if strings.Contains(out.String()+errOut.String(), "SYNTHETIC-PRIVATE") || strings.Contains(out.String()+errOut.String(), "SYNTHETIC-private1") {
				t.Fatal("secret echoed")
			}
		})
	}
}
