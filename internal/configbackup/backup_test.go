package configbackup

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"strings"
	"testing"
)

func syntheticBackup() Backup {
	b := Backup{Version: 1, Device: devices.Device{ID: "fixture", Vendor: "Synthetic", Model: "Fixture-1", Serial: "serial-fixture", Firmware: "1.0"}, Provider: "synthetic fixture", At: "2026-01-01T00:00:00Z", Settings: []Setting{{Key: "clock.timezone", Value: "UTC", State: "available", Portable: true}}}
	b.Seal()
	return b
}
func TestEncryptedPackageAndCompatibility(t *testing.T) {
	b := syntheticBackup()
	raw, e := Export(b, "synthetic portable passphrase")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), b.Device.Serial) {
		t.Fatal("plaintext metadata")
	}
	round, e := Import(raw, "synthetic portable passphrase")
	if e != nil || round.Checksum != b.Checksum {
		t.Fatal(e)
	}
	if _, e = Import(raw, "wrong synthetic passphrase"); e == nil {
		t.Fatal("wrong password")
	}
	raw[len(raw)-1] ^= 1
	if _, e = Import(raw, "synthetic portable passphrase"); e == nil {
		t.Fatal("damaged package")
	}
	p := Plan(b, b.Device)
	if !p.Compatible || p.Enabled {
		t.Fatal(p)
	}
	d := b.Device
	d.Firmware = "2.0"
	if Plan(b, d).Compatible {
		t.Fatal("different firmware accepted")
	}
	if !errors.Is(Restore(context.Background(), p), ErrGated) {
		t.Fatal("production gate absent")
	}
	b.Version = 2
	b.Seal()
	if b.Validate() == nil {
		t.Fatal("unsupported future schema")
	}
}
func TestDiffRedactsSecretsAndNonportableValues(t *testing.T) {
	a, b := syntheticBackup(), syntheticBackup()
	a.Settings = append(a.Settings, Setting{Key: "password", Value: "SENTINEL-A", State: "available", Portable: true}, Setting{Key: "network.address", Value: "192.0.2.2", State: "available"})
	b.Settings = append(b.Settings, Setting{Key: "password", Value: "SENTINEL-B", State: "available", Portable: true}, Setting{Key: "network.address", Value: "192.0.2.3", State: "available"})
	b.Settings[0].Value = "NZST-12NZDT"
	for _, x := range Diff(a, b) {
		if strings.Contains(x.Before, "SENTINEL") || strings.Contains(x.After, "SENTINEL") || strings.Contains(x.Before, "192.0.2.") {
			t.Fatal("sensitive diff", x)
		}
	}
	b.Seal()
	if b.Validate() == nil {
		t.Fatal("sensitive import accepted")
	}
}

type fakeWriter struct {
	id        string
	calls     int
	uncertain bool
}

func (f *fakeWriter) Identity(context.Context) (string, error) { return f.id, nil }
func (f *fakeWriter) Write(context.Context, Setting) error {
	f.calls++
	if f.uncertain {
		return errors.New("synthetic disconnect")
	}
	return nil
}
func (f *fakeWriter) Verify(context.Context, Setting) error { return nil }
func TestUncertainWriteHaltsAndIdentityIsRechecked(t *testing.T) {
	b := syntheticBackup()
	p := Plan(b, b.Device)
	p.Settings = append(p.Settings, p.Settings[0])
	w := &fakeWriter{id: b.Device.Serial, uncertain: true}
	out, e := execute(context.Background(), p, w)
	if e == nil || w.calls != 1 || len(out) != 1 || out[0].State != "unconfirmed" {
		t.Fatal(out, e, w.calls)
	}
	w = &fakeWriter{id: "other-device-same-IP"}
	if _, e = execute(context.Background(), p, w); e == nil || w.calls != 0 {
		t.Fatal("wrong target written")
	}
}
