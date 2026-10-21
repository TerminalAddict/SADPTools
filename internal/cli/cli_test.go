package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

func TestCLIOutputAndExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result discovery.Result
		err    error
		code   int
	}{
		{"empty", discovery.Result{Devices: []devices.Device{}}, nil, 0},
		{"partial", discovery.Result{Devices: []devices.Device{}, Warnings: []string{"adapter failed"}}, nil, 3},
		{"failure", discovery.Result{Devices: []devices.Device{}}, errors.New("port conflict"), 1},
		{"cancel", discovery.Result{Devices: []devices.Device{}}, context.Canceled, 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			scan := func(_ context.Context, opts discovery.Options, _ *slog.Logger) (discovery.Result, error) {
				if opts.Interface != "Ethernet 2" || opts.Timeout != 2*time.Second {
					t.Fatal(opts)
				}
				return tc.result, tc.err
			}
			code := run(context.Background(), []string{"discover", "--interface", "Ethernet 2", "--timeout", "2s", "--json"}, &out, &stderr, nil, scan)
			var list []devices.Device
			if code != tc.code || json.Unmarshal(out.Bytes(), &list) != nil || list == nil {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
		})
	}
}
func TestCLIUsageAndInterfaces(t *testing.T) {
	for _, args := range [][]string{{"invalid"}, {"discover", "--timeout", "0s"}, {"discover", "--timeout", "6m"}, {"discover", "--all-interfaces", "--interface", "Ethernet 2"}, {"discover", "unexpected"}} {
		var out bytes.Buffer
		if code := run(context.Background(), args, &out, &out, nil, nil); code != 2 {
			t.Fatal(args, code)
		}
	}
	var out bytes.Buffer
	interfaces := func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Name: "Ethernet 2", Suitable: true}}, nil
	}
	if code := run(context.Background(), []string{"interfaces", "--json"}, &out, &out, interfaces, nil); code != 0 {
		t.Fatal(code)
	}
	var list []platform.Adapter
	if err := json.Unmarshal(out.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatal(out.String())
	}
}
