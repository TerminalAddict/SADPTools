package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"testing"
)

func TestTroubleshootValidationAndCancelledEnvelope(t *testing.T) {
	for _, args := range [][]string{{}, {"--ip", "http://192.0.2.3"}, {"--ip", "192.0.2.3", "--ports", "1,1"}, {"--ip", "192.0.2.3", "--timeout", "0s"}} {
		var out, err bytes.Buffer
		if code := runTroubleshoot(context.Background(), args, &out, &err); code != 2 {
			t.Fatal(args, code)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	code := runTroubleshoot(ctx, []string{"--ip", "127.0.0.1", "--report", "--icmp=false", "--ports", "9"}, &out, &stderr)
	if code != 130 {
		t.Fatal(code, stderr.String())
	}
	var report diagnostics.Envelope
	if e := json.Unmarshal(out.Bytes(), &report); e != nil || report.Schema != 1 || report.Version == "" {
		t.Fatal(report, e)
	}
}
