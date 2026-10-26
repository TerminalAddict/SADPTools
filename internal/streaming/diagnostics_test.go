package streaming

import (
	"context"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"strings"
	"testing"
	"time"
)

func TestMeasuredDiagnosticsAndUnknownCounters(t *testing.T) {
	m := NewManager(nil)
	defer m.Close()
	camera, e := testcamera.New(false, true, "")
	if e != nil {
		t.Fatal(e)
	}
	defer camera.Close()
	ticket, e := m.Prepare(func(context.Context, func(string, string), func([]Profile)) (Source, error) {
		return NewSource("127.0.0.1", camera.URL, "rtsp", "", vault.Secret{Username: "fixture", Password: "synthetic-password"})
	}, nil, nil, "synthetic")
	if e != nil {
		t.Fatal(e)
	}
	initial := m.Status(ticket.ID)
	if initial.Metrics.EncodedFPS != nil || initial.Metrics.DecodedDrops != nil {
		t.Fatal("unknown metric fabricated")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	packets := 0
	e = m.Consume(ctx, ticket.ID, func(p Packet) error {
		if p.Kind == 1 {
			packets++
			if packets > 3 {
				cancel()
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	state := m.Status(ticket.ID)
	if state.Metrics.RTPPackets == nil || *state.Metrics.RTPPackets == 0 || state.Metrics.RTPMbps == nil || *state.Metrics.RTPMbps <= 0 || state.Metrics.FirstMediaMS == nil || state.Info.Width != 320 || state.Info.Height != 180 {
		t.Fatalf("missing actual telemetry: %+v", state)
	}
	if state.Metrics.NominalFPS != nil || state.Metrics.DecodedDrops != nil {
		t.Fatal("unavailable metrics fabricated")
	}
	b, _ := json.Marshal(state)
	if strings.Contains(string(b), "synthetic-password") || strings.Contains(string(b), camera.URL) {
		t.Fatal("diagnostics leaked endpoint/secret")
	}
	if len(state.Timeline) < 3 {
		t.Fatal("missing timeline")
	}
	state.Timeline[0].State = "mutated"
	if m.Status(ticket.ID).Timeline[0].State == "mutated" {
		t.Fatal("mutable timeline leaked")
	}
}
