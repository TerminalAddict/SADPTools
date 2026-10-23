package devices

import (
	"sync"
	"testing"
	"time"
)

func TestInventoryMergeAndIsolation(t *testing.T) {
	i := NewInventory()
	start := time.Unix(100, 0)
	active := false
	port := 8080
	d, created := i.Upsert(Device{MAC: "02-aa-bb-cc-dd-01", Model: "synthetic", Serial: "test", Activated: &active, HTTPPort: &port, Interfaces: []Interface{{Index: 1, Name: "Adapter A"}}}, start)
	if !created {
		t.Fatal("not new")
	}
	*d.Activated = true
	d.Interfaces[0].Name = "mutated"
	active = true
	_, created = i.Upsert(Device{MAC: "02:AA:BB:CC:DD:01", IPv4: "192.0.2.5", Activated: &active, Interfaces: []Interface{{Index: 2, Name: "Adapter B"}}}, start.Add(time.Second))
	if created {
		t.Fatal("not deduplicated")
	}
	list := i.Snapshot()
	if len(list) != 1 {
		t.Fatal(list)
	}
	d = list[0]
	if d.Model != "synthetic" || d.IPv4 != "192.0.2.5" || !d.FirstSeen.Equal(start) || !d.LastSeen.Equal(start.Add(time.Second)) || len(d.Interfaces) != 2 || d.Interfaces[0].Name != "Adapter A" || !*d.Activated || *d.HTTPPort != 8080 {
		t.Fatalf("bad merge: %+v", d)
	}
	*list[0].HTTPPort = 99
	if *i.Snapshot()[0].HTTPPort != 8080 {
		t.Fatal("snapshot aliases inventory")
	}
}
func TestInventoryIdentityPromotion(t *testing.T) {
	i := NewInventory()
	now := time.Now()
	i.Upsert(Device{Serial: "fixture", Model: "kept"}, now)
	i.Upsert(Device{MAC: "02:00:00:00:00:01", Serial: "fixture"}, now)
	i.Upsert(Device{Serial: "fixture", Firmware: "new"}, now)
	list := i.Snapshot()
	if len(list) != 1 || list[0].Model != "kept" || list[0].Firmware != "new" || list[0].ID != "mac:02:00:00:00:00:01" {
		t.Fatal(list)
	}
	i.Upsert(Device{MAC: "02:00:00:00:00:02", Serial: "fixture"}, now)
	if len(i.Snapshot()) != 2 {
		t.Fatal("distinct MACs merged")
	}
	if _, created := i.Upsert(Device{}, now); created {
		t.Fatal("unidentified device accepted")
	}
}
func TestInventoryConcurrent(t *testing.T) {
	i := NewInventory()
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 100; k++ {
				i.Upsert(Device{MAC: "02:00:00:00:00:01"}, time.Now())
				i.Snapshot()
			}
		}()
	}
	wg.Wait()
	if len(i.Snapshot()) != 1 {
		t.Fatal("duplicate")
	}
}
func TestReconcileSeparateSerialAndMACRecords(t *testing.T) {
	i := NewInventory()
	now := time.Unix(100, 0)
	i.Upsert(Device{MAC: "02:00:00:00:00:01", Firmware: "older", Interfaces: []Interface{{Index: 1, Name: "A"}}}, now)
	i.Upsert(Device{Serial: "synthetic", Firmware: "newer", Model: "retained", Interfaces: []Interface{{Index: 2, Name: "B"}}}, now.Add(time.Second))
	i.Upsert(Device{MAC: "02:00:00:00:00:01", Serial: "synthetic"}, now.Add(2*time.Second))
	list := i.Snapshot()
	if len(list) != 1 || len(list[0].Interfaces) != 2 || list[0].Firmware != "newer" || list[0].Model != "retained" || !list[0].FirstSeen.Equal(now) {
		t.Fatalf("lost observations during identity reconciliation: %+v", list)
	}
}
