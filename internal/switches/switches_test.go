package switches

import (
	"context"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/gosnmp/gosnmp"
	"strings"
	"testing"
)

// Synthetic MIB responses. No physical switch is queried by these tests.
func pdus() map[string]gosnmp.SnmpPDU {
	out := map[string]gosnmp.SnmpPDU{}
	put := func(oid string, value any) { out[oid] = gosnmp.SnmpPDU{Name: oid, Value: value} }
	put(ifEntry+".2.10", []byte("Synthetic port"))
	put(ifEntry+".8.10", 1)
	put(ifEntry+".7.10", 1)
	put(ifXEntry+".1.10", []byte("Fixture10"))
	put(ifXEntry+".15.10", uint32(1000))
	put(bridgePorts+".1", 10)
	put(qfdbEntry+".2.77.2.0.0.0.0.1", 1)
	put(qfdbEntry+".3.77.2.0.0.0.0.1", 3)
	put(vlanEntry+".3.0.20", uint32(77))
	put(vlanEntry+".4.0.20", []byte{0x80})
	put(lldpLocal+".2.9", 5)
	put(lldpLocal+".3.9", []byte("Fixture10"))
	put(lldpRemote+".4.0.9.1", 4)
	put(lldpRemote+".5.0.9.1", []byte{2, 0, 0, 0, 0, 1})
	put(lldpRemote+".9.0.9.1", []byte("Synthetic camera"))
	put(lldpRemote+".12.0.9.1", []byte{1})
	return out
}
func TestFDBVLANLLDPAndUnknownPower(t *testing.T) {
	a := Asset{ID: "fixture-switch", Name: "Synthetic switch"}
	target := devices.Device{MAC: "02:00:00:00:00:01", IPv4: "192.0.2.10"}
	r := Decode(a, Report{At: "synthetic-time"}, pdus(), []devices.Device{target})
	if len(r.Ports) != 1 || len(r.Mappings) != 1 || r.Mappings[0].Confidence != "direct/strong" || r.Mappings[0].VLANs[0] != 20 || r.Mappings[0].Port != 10 || r.Mappings[0].PowerWatts != nil {
		t.Fatal(r)
	}
	rows := pdus()
	oid := qfdbEntry + ".2.77.2.0.0.0.0.2"
	rows[oid] = gosnmp.SnmpPDU{Name: oid, Value: 1}
	r = Decode(a, Report{}, rows, []devices.Device{target})
	if r.Mappings[0].Confidence != "ambiguous" {
		t.Fatal("multiple MAC port claimed direct")
	}
	rows = pdus()
	oid = lldpRemote + ".12.0.9.1"
	rows[oid] = gosnmp.SnmpPDU{Name: oid, Value: []byte{0x20}}
	r = Decode(a, Report{}, rows, []devices.Device{target})
	if !r.Ports[0].Uplink || r.Mappings[0].Confidence != "ambiguous" {
		t.Fatal("LLDP bridge uplink claimed direct")
	}
}
func TestPoEIndexMustBeMappedAndCiscoMilliwatts(t *testing.T) {
	rows := pdus()
	for oid, value := range map[string]any{poeStatus + ".1.7": 3, ciscoPower + ".1.7": uint32(6300)} {
		rows[oid] = gosnmp.SnmpPDU{Name: oid, Value: value}
	}
	r := Decode(Asset{}, Report{}, rows, nil)
	if len(r.Power) != 1 || r.Power[0].Watts == nil || *r.Power[0].Watts != 6.3 || r.Ports[0].PowerWatts != nil {
		t.Fatal("assumed PoE index equals ifIndex", r)
	}
	a := Asset{PortLinks: []PortLink{{IfIndex: 10, PoEGroup: 1, PoEPort: 7}}}
	r = Decode(a, Report{}, rows, nil)
	if r.Ports[0].PowerWatts == nil || *r.Ports[0].PowerWatts != 6.3 || r.Ports[0].PoE != "delivering power" {
		t.Fatal(r)
	}
	rows[ciscoPhysical+".1.7"] = gosnmp.SnmpPDU{Value: 42}
	rows[entityAlias+".42.0"] = gosnmp.SnmpPDU{Value: ifEntry + ".1.10"}
	r = Decode(Asset{}, Report{}, rows, nil)
	if r.Ports[0].PowerWatts == nil {
		t.Fatal("documented ENTITY mapping not used")
	}
}

type fakeWalker struct {
	calls  []string
	bad    bool
	cancel context.CancelFunc
}

func (w *fakeWalker) BulkWalk(root string, fn gosnmp.WalkFunc) error {
	w.calls = append(w.calls, root)
	if w.cancel != nil {
		w.cancel()
	}
	if w.bad {
		return fn(gosnmp.SnmpPDU{Name: ".9.9.9", Value: "outside"})
	}
	for _, p := range pdus() {
		if strings.HasPrefix(p.Name, root+".") {
			if e := fn(p); e != nil {
				return e
			}
		}
	}
	return nil
}
func TestReadOnlyCollectionAndBounds(t *testing.T) {
	w := &fakeWalker{}
	rows, warnings := Collect(context.Background(), w, ".1.3.6.1.4.1.9.1.1", nil)
	if len(rows) == 0 || len(warnings) > 0 || len(w.calls) != len(columns)+3 {
		t.Fatal(len(rows), warnings)
	}
	w = &fakeWalker{bad: true}
	rows, warnings = Collect(context.Background(), w, "", nil)
	if len(rows) != 0 || len(warnings) == 0 {
		t.Fatal("out-of-scope OIDs accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	w = &fakeWalker{cancel: cancel}
	Collect(ctx, w, "", nil)
	if len(w.calls) != 1 {
		t.Fatal("continued after cancellation")
	}
}
func TestSNMPProtectionChoice(t *testing.T) {
	a := Asset{Name: "Synthetic", Host: "192.0.2.10", Port: 161, Version: "3", AuthProtocol: "sha256"}
	secret := vault.Secret{Username: "fixture", Password: "Synthetic-auth-123", PrivacyPassword: "Synthetic-privacy-123"}
	c, e := client(context.Background(), a, a.Host, secret)
	if e != nil || c.Version != gosnmp.Version3 || c.MsgFlags != gosnmp.AuthPriv || c.Retries != 0 {
		t.Fatal(e)
	}
	a.Version = "2c"
	if _, e = client(context.Background(), a, a.Host, secret); e == nil {
		t.Fatal("v2c implicitly enabled")
	}
	a.AllowV2 = true
	if _, e = client(context.Background(), a, a.Host, secret); e != nil {
		t.Fatal(e)
	}
}
