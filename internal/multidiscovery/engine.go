// Package multidiscovery combines protocol providers without coupling them to a GUI.
package multidiscovery

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"log/slog"
	"slices"
	"sync"
	"time"
)

type ScanFunc func(context.Context, discovery.Options, func(devices.Device)) (discovery.Result, error)
type Engine struct{ SADP, ONVIF ScanFunc }

func New(log *slog.Logger) *Engine { return &Engine{SADP: discovery.New(log).Scan, ONVIF: onvif.Scan} }
func Validate(protocols []string) error {
	if len(protocols) > 2 {
		return errors.New("select SADP and/or ONVIF")
	}
	for _, p := range protocols {
		if p != "sadp" && p != "onvif" {
			return errors.New("unknown discovery protocol")
		}
	}
	return nil
}
func (e *Engine) Scan(ctx context.Context, o discovery.Options, callback func(devices.Device)) (discovery.Result, error) {
	result := discovery.Result{Started: time.Now(), Devices: []devices.Device{}, Warnings: []string{}}
	if err := Validate(o.Protocols); err != nil {
		return result, err
	}
	protocols := o.Protocols
	if len(protocols) == 0 {
		protocols = []string{"sadp"}
	}
	inventory := devices.NewInventory()
	var mu, callbackMu sync.Mutex
	var wg sync.WaitGroup
	success := 0
	for _, name := range []string{"sadp", "onvif"} {
		if !slices.Contains(protocols, name) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			scan := e.SADP
			if name == "onvif" {
				scan = e.ONVIF
			}
			r, err := scan(ctx, o, func(d devices.Device) {
				callbackMu.Lock()
				defer callbackMu.Unlock()
				merged, _ := inventory.Upsert(d, time.Now())
				if callback != nil {
					callback(merged)
				}
			})
			mu.Lock()
			defer mu.Unlock()
			for _, w := range r.Warnings {
				result.Warnings = append(result.Warnings, name+": "+w)
			}
			if err != nil {
				result.Warnings = append(result.Warnings, name+": "+err.Error())
			} else {
				success++
			}
		}()
	}
	wg.Wait()
	result.Devices = inventory.Snapshot()
	result.Finished = time.Now()
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if success == 0 {
		return result, errors.New("discovery could not complete on any enabled protocol; see diagnostic warnings")
	}
	return result, nil
}
