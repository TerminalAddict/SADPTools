// Package discovery coordinates read-only SADP scans, independently of any UI.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

var ErrBusy = errors.New("a discovery scan is already running")

const DefaultTimeout = 5 * time.Second
const MaxTimeout = 5 * time.Minute

type Options struct {
	Interface string
	Timeout   time.Duration
}
type Result struct {
	Devices  []devices.Device `json:"devices"`
	Warnings []string         `json:"warnings"`
	Started  time.Time        `json:"started"`
	Finished time.Time        `json:"finished"`
}

type Engine struct {
	log        *slog.Logger
	interfaces func() ([]platform.Adapter, error)
	open       func() (platform.Transport, error)
	active     atomic.Bool
}

func New(log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{log: log, interfaces: platform.Interfaces, open: platform.Open}
}

// Scan calls onDevice synchronously; callbacks should be short and must not
// invoke another scan. Returned devices include partial results on error.
func (e *Engine) Scan(parent context.Context, opts Options, onDevice func(devices.Device)) (result Result, err error) {
	result = Result{Devices: []devices.Device{}, Warnings: []string{}, Started: time.Now()}
	defer func() { result.Finished = time.Now() }()
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.Timeout < 0 || opts.Timeout > MaxTimeout {
		return result, fmt.Errorf("timeout must be positive and at most %s", MaxTimeout)
	}
	if !e.active.CompareAndSwap(false, true) {
		return result, ErrBusy
	}
	defer e.active.Store(false)
	if err = parent.Err(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()
	inventory := devices.NewInventory()
	defer func() { result.Devices = inventory.Snapshot() }()
	adapters, err := e.interfaces()
	if err != nil {
		return result, err
	}
	adapters, err = platform.Select(adapters, opts.Interface)
	if err != nil {
		return result, err
	}
	conn, err := e.open()
	if err != nil {
		return result, err
	}
	// Wait for an in-flight close callback before returning, including cancellation races.
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = conn.Close(); close(closed) })
	defer func() {
		if stop() {
			_ = conn.Close()
		} else {
			<-closed
		}
	}()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return result, fmt.Errorf("set socket deadline: %w", err)
	}
	warnSeen := map[string]bool{}
	warn := func(message string) {
		if !warnSeen[message] {
			warnSeen[message] = true
			result.Warnings = append(result.Warnings, message)
			e.log.Warn(message)
		}
	}
	type target struct {
		adapter platform.Adapter
		uuid    string
		probes  [][]byte
	}
	targets := []target{}
	byIndex := map[int]platform.Adapter{}
	ids := map[string]int{}
	for _, a := range adapters {
		if ctx.Err() != nil {
			break
		}
		if joinErr := conn.Join(a); joinErr != nil {
			warn(fmt.Sprintf("%s: cannot join multicast: %v", a.Name, joinErr))
			continue
		}
		uuid, idErr := sadp.NewUUID()
		if idErr != nil {
			return result, idErr
		}
		probes, probeErr := sadp.Probes(uuid)
		if probeErr != nil {
			return result, probeErr
		}
		targets = append(targets, target{a, uuid, probes})
		byIndex[a.Index] = a
		ids[strings.ToLower(uuid)] = a.Index
		e.log.Info("Scanning interface", "interface", a.Name, "addresses", a.Addresses, "timeout", opts.Timeout)
	}
	finish := func() error {
		if parent.Err() != nil {
			return parent.Err()
		}
		if parentDeadline, ok := parent.Deadline(); ok && !time.Now().Before(parentDeadline) {
			return context.DeadlineExceeded
		}
		return nil
	}
	if ctx.Err() != nil {
		return result, finish()
	}
	if len(targets) == 0 {
		return result, errors.New("could not join SADP multicast on any selected interface")
	}
	send := func() int {
		sent := 0
		for _, target := range targets {
			for _, b := range target.probes {
				if ctx.Err() != nil {
					return sent
				}
				if sendErr := conn.Send(target.adapter, b); sendErr != nil {
					warn(fmt.Sprintf("%s: multicast send failed: %v", target.adapter.Name, sendErr))
				} else {
					sent++
					e.log.Debug("Sent SADP inquiry", "interface", target.adapter.Name, "uuid", target.uuid, "bytes", len(b))
				}
			}
		}
		return sent
	}
	if send() == 0 {
		if ctx.Err() != nil {
			return result, finish()
		}
		return result, errors.New("all SADP multicast sends failed")
	}
	nextSend := time.Now().Add(time.Second)
	buf := make([]byte, sadp.MaxPacketSize)
	for {
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return result, finish()
		}
		if !time.Now().Before(nextSend) {
			send()
			nextSend = time.Now().Add(time.Second)
		}
		readUntil := nextSend
		if deadline.Before(readUntil) {
			readUntil = deadline
		}
		if err = conn.SetReadDeadline(readUntil); err != nil {
			if ctx.Err() != nil {
				return result, finish()
			}
			return result, err
		}
		n, info, readErr := conn.Read(buf)
		if readErr != nil {
			if ctx.Err() != nil {
				return result, finish()
			}
			var timeout net.Error
			if errors.As(readErr, &timeout) && timeout.Timeout() {
				continue
			}
			return result, fmt.Errorf("receive SADP: %w", readErr)
		}
		reply, parseErr := sadp.Parse(buf[:n])
		if parseErr != nil {
			e.log.Debug("Ignored datagram", "source", info.Source, "bytes", n, "reason", parseErr)
			continue
		}
		adapter, selected := byIndex[info.InterfaceIndex]
		if !selected {
			e.log.Debug("Ignored reply from unselected or unknown interface", "index", info.InterfaceIndex)
			continue
		}
		if reply.UUID != "" {
			if _, ok := ids[strings.ToLower(reply.UUID)]; !ok {
				e.log.Debug("Ignored unrelated reply UUID", "uuid", reply.UUID)
				continue
			}
		} else {
			e.log.Debug("Accepted legacy reply without UUID", "interface", adapter.Name)
		}
		reply.Device.Interfaces = []devices.Interface{{Index: adapter.Index, Name: adapter.Name, Address: adapter.Address()}}
		d, isNew := inventory.Upsert(reply.Device, time.Now())
		if isNew {
			e.log.Info("Discovered device", "mac", d.MAC, "ip", d.IPv4, "interface", adapter.Name)
		}
		if onDevice != nil {
			onDevice(d)
		}
	}
}
