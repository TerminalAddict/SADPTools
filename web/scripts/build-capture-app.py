#!/usr/bin/env python3
"""Build a screenshot-only binary with synthetic devices/adapters via Go overlays.

Never ships or edits application sources. No scanning, recovery, or device writes
are performed by capture-features.py. The video source runs on loopback only.
"""
from pathlib import Path
import json
import subprocess

root = Path(__file__).resolve().parents[2]
cache = root / 'web/.cache/capture-overlay'
cache.mkdir(parents=True, exist_ok=True)
replacements = {}

def overlay(path, before, after):
    original = root / path
    source = Path(replacements.get(str(original), original)).read_text()
    assert source.count(before) == 1, f'Capture hook changed: {path}'
    target = cache / path.replace('/', '_')
    target.write_text(source.replace(before, after))
    replacements[str(original)] = str(target)

overlay('internal/platform/interfaces.go', 'func Interfaces() ([]Adapter, error) {', '''func Interfaces() ([]Adapter, error) {
    return []Adapter{{Name: "Demo Ethernet", Index: 9999, Addresses: []string{"192.0.2.10/24"}, Gateway: "192.0.2.1", Suitable: true, UnicastSuitable: true}}, nil
''')
overlay('internal/app/controller.go', '\tgo c.automaticLoop()\n\treturn c', '''
    // Synthetic screenshot fixture, absent from release builds.
    active, inactive, dhcp, port := true, false, false, 80
    names := []string{"Main entrance", "Loading bay", "East car park", "Reception", "Stock room", "Rear access", "North perimeter", "Warehouse NVR"}
    for i, name := range names {
        state := &active
        if i == 6 { state = &inactive }
        d := devices.Device{Name: name, Vendor: "Demonstration", Model: "Demo camera", IPv4: fmt.Sprintf("192.0.2.%d", 21+i), MAC: fmt.Sprintf("02:00:00:00:00:%02x", i+1), Serial: fmt.Sprintf("DEMO-%04d", i+1), Firmware: "Illustrative", SubnetMask: "255.255.255.0", Gateway: "192.0.2.1", HTTPPort: &port, Activated: state, DHCP: &dhcp, Protocols: []string{"sadp", "onvif"}, Interfaces: []devices.Interface{{Index:9999, Name:"Demo Ethernet", Address:"192.0.2.10"}}}
        if i == 7 { d.Model = "Demo recorder" }
        c.inventory.Upsert(d, time.Now())
    }
    c.log.Info("Demonstration inventory — synthetic devices; no network scan performed")
    go c.automaticLoop()
    return c''')
overlay('desktop.go', 'return s.controller.CheckDeviceWeb(id, refresh)', 'return webinterface.Result{}, nil // Capture fixture: no endpoint probes.')
overlay('desktop.go', 'return s.controller.CheckSecurityQuestions(id, adapter, certificateSHA256)', '''return recovery.Status{State:"ready", CheckID:"demo-only", Message:"Synthetic demonstration questions; no camera was contacted.", Questions:[]recovery.Question{{ID:1, Text:"Demonstration security question 1"},{ID:2, Text:"Demonstration security question 2"},{ID:3, Text:"Demonstration security question 3"}}}, nil''')
overlay('internal/toolkit/audit.go', 'return audit.Clone(m.audit)', '''
    all, _ := m.Store.Sites()
    if len(all)>0 {
        view, _ := m.Store.View(all[0].ID)
        for _, saved := range view.Runs {
            if saved.Kind == "audit" {
                run, err := m.Store.Run(all[0].ID, saved.ID)
                var report audit.Report
                if err == nil && json.Unmarshal([]byte(run.Data), &report) == nil { return report }
            }
        }
    }
    return audit.Clone(m.audit)''')
(cache/'overlay.json').write_text(json.dumps({'Replace': replacements}))
subprocess.run(['go','build','-overlay',str(cache/'overlay.json'),'-tags','desktop,production','-buildvcs=false','-ldflags','-s -w -X github.com/TerminalAddict/SADPTools/internal/version.Value=0.6.0-demo','-o',str(root/'web/.cache/SADPTools-capture'),'.'],cwd=root,check=True)
print('Built ignored capture-only app; production sources and binary unchanged.')
