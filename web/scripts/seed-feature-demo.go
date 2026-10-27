//go:build ignore

// Offline demo inventory and loopback-only synthetic RTSP source for screenshots.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/audit"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/nvr"
	"github.com/TerminalAddict/SADPTools/internal/retention"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
	"github.com/TerminalAddict/SADPTools/internal/switches"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func raw(v any) json.RawMessage { b, e := json.Marshal(v); must(e); return b }
func ptr[T any](v T) *T         { return &v }
func main() {
	dir := os.Args[1]
	camera, err := testcamera.New(false, false, "")
	must(err)
	defer camera.Close()
	uri, err := url.Parse(camera.URL)
	must(err)
	port, err := strconv.Atoi(uri.Port())
	must(err)
	st, err := sites.Open(filepath.Join(dir, "SADPTools/sites.sqlite"))
	must(err)
	all, err := st.Sites()
	must(err)
	var site sites.Site
	var records []sites.Record
	if len(all) == 0 {
		site, err = st.SaveSite(sites.Site{Name: "Demonstration site — Harbour Warehouse", Customer: "Synthetic examples, not real equipment", Networks: []string{"192.0.2.0/24"}, Protocols: []string{"sadp", "onvif"}, Tags: []string{"demonstration"}})
		must(err)
		names := []string{"Main entrance — test pattern", "Loading bay", "East car park", "Reception", "Stock room", "Rear access", "North perimeter", "Warehouse NVR"}
		for i, name := range names {
			d := devices.Device{Name: name, Vendor: "Demonstration", Model: "Demo camera", IPv4: fmt.Sprintf("192.0.2.%d", 21+i), MAC: fmt.Sprintf("02:00:00:00:00:%02x", i+1), Serial: fmt.Sprintf("DEMO-%04d", i+1), Firmware: "Illustrative", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", HTTPPort: ptr(80), RTSPPort: ptr(554), Activated: ptr(i != 6), DHCP: ptr(false), Protocols: []string{"sadp", "onvif"}, LastSeen: time.Now()}
			if i == 7 {
				d.Model = "Demo recorder"
			}
			d.ID = d.Key()
			r, e := st.SaveRecord(sites.Record{SiteID: site.ID, Device: d, Location: []string{"Entrance", "Loading dock", "Outdoor", "Building B", "Building B", "Building A", "Outdoor", "Comms cabinet"}[i], CredentialMode: "none", Notes: "Synthetic demonstration record"})
			must(e)
			records = append(records, r)
		}
		must(st.Unlock(hex.EncodeToString(vault.Random(24))))
		_, err = st.SaveProfile(sites.Profile{SiteID: site.ID, Label: "Demonstration operator profile", Username: "demo-operator"}, vault.Secret{Username: "demo-operator", Password: hex.EncodeToString(vault.Random(24))})
		must(err)
		save := func(kind, title string, value any, deviceID string) {
			_, e := st.SaveDocument(sites.Document{SiteID: site.ID, DeviceID: deviceID, Kind: kind, Title: title, Version: 1, Data: raw(value)})
			must(e)
		}
		stamp := time.Now().UTC().Format(time.RFC3339)
		report := audit.Report{SiteID: site.ID, SiteName: site.Name, Started: stamp, Finished: stamp, Done: 8, Total: 8}
		for i, r := range records {
			status := "pass"
			evidence := "Synthetic fixture: advertised web service observed"
			if i == 6 {
				status = "warning"
				evidence = "Synthetic fixture: device not activated"
			}
			report.Findings = append(report.Findings, audit.Finding{RecordID: r.ID, IP: r.Device.IPv4, Model: r.Device.Model, MAC: r.Device.MAC, Check: "Illustrative service check", Status: status, Evidence: evidence, Source: "Synthetic fixture", At: stamp})
		}
		_, err = st.SaveRun(sites.Run{SiteID: site.ID, Kind: "audit", Scope: "Synthetic examples only", Outcome: "illustrative", Data: string(raw(report))})
		must(err)
		sw, err := st.SaveSwitch(switches.Asset{SiteID: site.ID, Name: "Demo access switch", Host: "192.0.2.2", Version: "3", AuthProtocol: "sha256", Port: 161})
		must(err)
		swreport := switches.Report{AssetID: sw.ID, SiteID: site.ID, SwitchName: sw.Name, Address: sw.Host, At: stamp, SystemName: "DEMO-SW01", Description: "Synthetic managed switch", Warnings: []string{"Demonstration data — no switch was polled"}}
		for i, r := range records[:7] {
			swreport.Ports = append(swreport.Ports, switches.Port{Index: i + 1, Name: fmt.Sprintf("Gi1/0/%d", i+1), Admin: "up", Operational: "up", SpeedMbps: ptr(100.0), VLANs: []int{20}, PoE: "delivering", PowerWatts: ptr(5.5), PoESource: "Synthetic fixture"})
			swreport.Mappings = append(swreport.Mappings, switches.Mapping{DeviceID: r.ID, IP: r.Device.IPv4, MAC: r.Device.MAC, SwitchID: sw.ID, SwitchName: sw.Name, Port: i + 1, PortName: fmt.Sprintf("Gi1/0/%d", i+1), VLANs: []int{20}, Confidence: "likely", Evidence: []string{"Synthetic FDB example"}, Link: "up", SpeedMbps: ptr(100.0), PoE: "delivering", PowerWatts: ptr(5.5), At: stamp})
		}
		_, err = st.SaveRun(sites.Run{SiteID: site.ID, Kind: "switch", Scope: "Synthetic switch example", Outcome: "illustrative", Data: string(raw(swreport))})
		must(err)
		image, err := os.ReadFile("web/.cache/demo-plan.png")
		must(err)
		a, err := st.SaveAsset(sites.Asset{SiteID: site.ID, Kind: "plan", Title: "Demonstration warehouse plan"}, image)
		must(err)
		plan := installation.Plan{ImageID: a.ID, Notes: "Illustrative layout only; no actual premises shown."}
		positions := [][3]float64{{.12, .81, 270}, {.62, .81, 230}, {.83, .25, 90}, {.12, .23, 45}, {.12, .65, 315}, {.63, .23, 135}, {.9, .8, 270}}
		for i, pos := range positions {
			plan.Markers = append(plan.Markers, installation.Marker{ID: fmt.Sprintf("demo-marker-%d", i), DeviceID: records[i].ID, X: pos[0], Y: pos[1], Bearing: pos[2], Cone: 65, Label: names[i], Layer: "Ground floor"})
		}
		save("plan", "Demonstration warehouse plan", plan, "")
		save("retention", "Demo 30-day recording plan", retention.Scenario{Name: "Demo 30-day recording plan", Channels: []retention.Channel{{Label: "Warehouse cameras (estimate)", Count: 8, Mbps: retention.Number(4), LowMbps: retention.Number(2), HighMbps: retention.Number(6), HoursPerDay: 24, DutyCycle: 1, Source: "estimate"}}, Capacity: 12, Unit: "TB", Reserve: .05, Overhead: .05, SafetyMargin: .1, TargetDays: 30}, "")
		save("commissioning", "Demonstration handover", installation.Commissioning{Title: "Demonstration site handover", Technician: "Demonstration technician", Date: "2026-10-27", Project: "DEMO-001", Summary: "Illustrative checklist. No physical equipment has been tested.", Checks: []installation.Check{{DeviceID: records[0].ID, Test: "live-video", Result: "not-tested", Source: "manual"}}}, "")
		inv := nvr.Inventory{NVRID: records[0].ID, At: stamp, Provider: "Synthetic demonstration", Warnings: []string{"Illustrative channel inventory, not a hardware recording test"}}
		for i, r := range records[:7] {
			status := "connected"
			if i == 6 {
				status = "offline"
			}
			inv.Channels = append(inv.Channels, nvr.Channel{ID: fmt.Sprint(i + 1), Name: names[i], Model: "Demo camera", Address: r.Device.IPv4, Status: status, Recording: "unknown", LinkedDeviceID: r.ID, Confidence: "manual example", Streams: []nvr.Stream{{ID: fmt.Sprint((i+1)*100 + 1), Codec: "H264", Width: 1920, Height: 1080}}})
		}
		save("nvr-channels", "Illustrative recorder channels", inv, records[0].ID)
		must(st.Visit(site.ID))
	} else {
		site = all[0]
		records, err = st.Records(site.ID)
		must(err)
	}
	for _, r := range records {
		if r.Device.Serial == "DEMO-0001" {
			r.Device.IPv4 = "127.0.0.1"
			r.Device.RTSPPort = &port
			r.CredentialMode = "none"
			_, err = st.SaveRecord(r)
			must(err)
		}
	}
	must(st.Close())
	must(os.WriteFile(filepath.Join(dir, "../ready.json"), raw(map[string]any{"rtspPort": port, "siteID": site.ID}), 0600))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
