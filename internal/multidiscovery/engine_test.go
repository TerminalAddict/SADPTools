package multidiscovery

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"testing"
	"time"
)

func TestProtocolsPartialFailureAndStrongMerge(t *testing.T) {
	calls := 0
	engine := &Engine{SADP: func(ctx context.Context, o discovery.Options, fn func(devices.Device)) (discovery.Result, error) {
		fn(devices.Device{MAC: "02:00:00:00:00:01", Serial: "synthetic", Protocols: []string{"sadp"}})
		return discovery.Result{}, nil
	}, ONVIF: func(ctx context.Context, o discovery.Options, fn func(devices.Device)) (discovery.Result, error) {
		calls++
		fn(devices.Device{MAC: "02:00:00:00:00:01", Serial: "synthetic", UUID: "synthetic-uuid", Protocols: []string{"onvif"}})
		return discovery.Result{}, nil
	}}
	r, e := engine.Scan(context.Background(), discovery.Options{Timeout: time.Millisecond}, nil)
	if e != nil || len(r.Devices) != 1 || calls != 0 {
		t.Fatal(r, e)
	}
	r, e = engine.Scan(context.Background(), discovery.Options{Protocols: []string{"sadp", "onvif"}, Timeout: time.Millisecond}, nil)
	if e != nil || len(r.Devices) != 1 || len(r.Devices[0].Protocols) != 2 {
		t.Fatal(r, e)
	}
	engine.ONVIF = func(context.Context, discovery.Options, func(devices.Device)) (discovery.Result, error) {
		return discovery.Result{}, errors.New("synthetic unavailable")
	}
	r, e = engine.Scan(context.Background(), discovery.Options{Protocols: []string{"sadp", "onvif"}}, nil)
	if e != nil || len(r.Warnings) != 1 {
		t.Fatal(r, e)
	}
}
