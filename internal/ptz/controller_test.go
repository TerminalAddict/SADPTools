package ptz

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fake struct {
	moves, stops atomic.Int32
	fail         bool
}

func (f *fake) Move(context.Context, float64, float64, float64) error {
	f.moves.Add(1)
	if f.fail {
		return errors.New("synthetic disconnect")
	}
	return nil
}
func (f *fake) Stop(context.Context) error           { f.stops.Add(1); return nil }
func (f *fake) Recall(context.Context, string) error { return nil }
func TestMovementFailsafeReleaseAndClose(t *testing.T) {
	p := &fake{}
	c := New(p)
	if e := c.Pulse(context.Background(), .2, 0, 0); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(2 * time.Second)
	for p.stops.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.stops.Load() == 0 {
		t.Fatal("failsafe did not stop")
	}
	c.Stop()
	c.Close()
	if p.stops.Load() < 3 {
		t.Fatal("release/close stop missing")
	}
	if e := c.Pulse(context.Background(), 1, 0, 0); e == nil {
		t.Fatal("closed movement allowed")
	}
}
func TestFailedMovementStillStopsAndDoesNotRetry(t *testing.T) {
	p := &fake{fail: true}
	c := New(p)
	defer c.Close()
	if e := c.Pulse(context.Background(), 1, 0, 0); e == nil {
		t.Fatal("error hidden")
	}
	if p.moves.Load() != 1 || p.stops.Load() != 1 {
		t.Fatal("movement retried or stop missed")
	}
}
