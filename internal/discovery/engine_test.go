package discovery

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

type datagram struct {
	body  string
	index int
}
type fakeTransport struct {
	mu           sync.Mutex
	queue        []datagram
	readDeadline time.Time
	closed       chan struct{}
	started      chan struct{}
	once         sync.Once
	closeOnce    sync.Once
	closes       atomic.Int32
	sends        atomic.Int32
	onSend       func(platform.Adapter, []byte)
	joinErr      map[int]error
	sendErr      error
	readErr      error
}

func fake() *fakeTransport {
	return &fakeTransport{closed: make(chan struct{}), started: make(chan struct{}), joinErr: map[int]error{}}
}
func (f *fakeTransport) Join(a platform.Adapter) error { return f.joinErr[a.Index] }
func (f *fakeTransport) Send(a platform.Adapter, b []byte) error {
	f.sends.Add(1)
	f.once.Do(func() { close(f.started) })
	if f.onSend != nil {
		f.onSend(a, b)
	}
	return f.sendErr
}
func (f *fakeTransport) Read(b []byte) (int, platform.PacketInfo, error) {
	if f.readErr != nil {
		return 0, platform.PacketInfo{}, f.readErr
	}
	f.mu.Lock()
	if len(f.queue) > 0 {
		p := f.queue[0]
		f.queue = f.queue[1:]
		f.mu.Unlock()
		return copy(b, p.body), platform.PacketInfo{InterfaceIndex: p.index, Source: "198.51.100.20:37020"}, nil
	}
	deadline := f.readDeadline
	f.mu.Unlock()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-f.closed:
		return 0, platform.PacketInfo{}, net.ErrClosed
	case <-timer.C:
		return 0, platform.PacketInfo{}, os.ErrDeadlineExceeded
	}
}
func (f *fakeTransport) SetDeadline(t time.Time) error { return f.SetReadDeadline(t) }
func (f *fakeTransport) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	f.readDeadline = t
	f.mu.Unlock()
	return nil
}
func (f *fakeTransport) Close() error {
	f.closes.Add(1)
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}
func testEngine(f *fakeTransport) *Engine {
	e := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.open = func() (platform.Transport, error) { return f, nil }
	e.interfaces = func() ([]platform.Adapter, error) {
		return []platform.Adapter{{Index: 1, Name: "Ethernet 2", Addresses: []string{"192.0.2.1/24"}, Suitable: true}, {Index: 2, Name: "Wi-Fi", Addresses: []string{"203.0.113.1/24"}, Suitable: true}}, nil
	}
	return e
}
func reply(id, mac string) string {
	return "<ProbeMatch><Uuid>" + id + "</Uuid><MAC>" + mac + "</MAC><IPv4Address>198.51.100.20</IPv4Address></ProbeMatch>"
}
func TestScanCorrelationDedupAndOffSubnet(t *testing.T) {
	f := fake()
	sent := map[int]bool{}
	f.onSend = func(a platform.Adapter, b []byte) {
		if sent[a.Index] {
			return
		}
		sent[a.Index] = true
		var p struct {
			UUID string `xml:"Uuid"`
		}
		if err := xml.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		f.queue = append(f.queue, datagram{"malformed", a.Index}, datagram{reply("unrelated", "02:00:00:00:00:02"), a.Index}, datagram{reply(p.UUID, "02:00:00:00:00:03"), 99}, datagram{reply(p.UUID, "02:00:00:00:00:01"), a.Index}, datagram{reply("", "02:00:00:00:00:01"), a.Index})
	}
	count := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// End after the expected replies, with a generous failure bound. The first
	// UUID generation on a Windows runner can exceed the old 30 ms scan window.
	r, err := testEngine(f).Scan(ctx, Options{Timeout: 5 * time.Second}, func(devices.Device) {
		count++
		if count == 4 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(r.Devices) != 1 || len(r.Devices[0].Interfaces) != 2 || r.Devices[0].IPv4 != "198.51.100.20" || count != 4 {
		t.Fatalf("unexpected result: %+v, callbacks %d", r, count)
	}
	if f.closes.Load() != 1 {
		t.Fatalf("close count: %d", f.closes.Load())
	}
}
func TestScanCancellationAndBusy(t *testing.T) {
	f := fake()
	e := testEngine(f)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := e.Scan(ctx, Options{Timeout: time.Minute}, nil); done <- err }()
	<-f.started
	if _, err := e.Scan(context.Background(), Options{}, nil); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not unblock read")
	}
	if f.closes.Load() != 1 {
		t.Fatal(f.closes.Load())
	}
}
func TestScanRepeatedTimeoutReleasesSockets(t *testing.T) {
	e := testEngine(fake())
	sockets := []*fakeTransport{}
	e.open = func() (platform.Transport, error) { f := fake(); sockets = append(sockets, f); return f, nil }
	for n := 0; n < 25; n++ {
		r, err := e.Scan(context.Background(), Options{Timeout: 2 * time.Millisecond}, nil)
		if err != nil || len(r.Devices) != 0 {
			t.Fatal(r, err)
		}
	}
	for _, f := range sockets {
		if f.closes.Load() != 1 {
			t.Fatal("socket leak", f.closes.Load())
		}
	}
}
func TestScanFailurePaths(t *testing.T) {
	for _, name := range []string{"enumeration", "missing adapter", "port conflict", "join all", "join partial", "send all", "read"} {
		t.Run(name, func(t *testing.T) {
			f := fake()
			e := testEngine(f)
			opts := Options{Timeout: 10 * time.Millisecond}
			switch name {
			case "enumeration":
				e.interfaces = func() ([]platform.Adapter, error) { return nil, errors.New("enumeration failed") }
			case "missing adapter":
				opts.Interface = "unavailable"
			case "port conflict":
				e.open = func() (platform.Transport, error) { return nil, errors.New("address in use") }
			case "join all":
				f.joinErr[1] = errors.New("failed")
				f.joinErr[2] = errors.New("failed")
			case "join partial":
				f.joinErr[2] = errors.New("failed")
			case "send all":
				f.sendErr = errors.New("network unreachable")
			case "read":
				f.readErr = errors.New("interface removed")
			}
			r, err := e.Scan(context.Background(), opts, nil)
			if name == "join partial" {
				if err != nil || len(r.Warnings) != 1 {
					t.Fatal(r, err)
				}
			} else if err == nil {
				t.Fatal("expected failure")
			}
		})
	}
}
func TestCancelledBeforeOpeningAndInvalidTimeout(t *testing.T) {
	e := testEngine(fake())
	e.open = func() (platform.Transport, error) { t.Fatal("opened socket"); return nil, nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Scan(ctx, Options{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.Scan(context.Background(), Options{Timeout: -time.Second}, nil); err == nil {
		t.Fatal("negative timeout accepted")
	}
}
func TestScanParentDeadlineIsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := testEngine(fake()).Scan(ctx, Options{Timeout: time.Second}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
