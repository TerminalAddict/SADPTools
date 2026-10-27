//go:build ignore

// Synthetic, non-networked inventory for website screenshots only.
package main

import (
 "fmt"
 "os"
 "path/filepath"
 "time"
 "github.com/TerminalAddict/SADPTools/internal/devices"
 "github.com/TerminalAddict/SADPTools/internal/sites"
)
func must(err error) { if err != nil { panic(err) } }
func main() {
 root := os.Args[1]
 store, err := sites.Open(filepath.Join(root, "SADPTools/sites.sqlite")); must(err); defer store.Close()
 site, err := store.SaveSite(sites.Site{Name:"Demonstration site — Harbour Warehouse", Customer:"Illustrative data only", Address:"Demo installation", Networks:[]string{"192.0.2.0/24"}, Protocols:[]string{"sadp","onvif"}, Tags:[]string{"demonstration"}, Notes:"Synthetic screenshot fixture. No real equipment or credentials."}); must(err)
 names := []string{"Main entrance", "Loading bay", "East car park", "Reception", "Stock room", "Rear access", "North perimeter", "Warehouse NVR"}
 locations := []string{"Building A", "Building A", "Outdoor", "Building B", "Building B", "Building A", "Outdoor", "Comms cabinet"}
 active, dhcp, httpPort, rtspPort := true, false, 80, 554
 for i, name := range names {
  model := "Demo network camera"; if i == 7 { model = "Demo network recorder" }
  d := devices.Device{Name:name, Model:model, Vendor:"Demo", MAC:fmt.Sprintf("02:00:00:00:00:%02x", i+1), Serial:fmt.Sprintf("DEMO-%04d", i+1), IPv4:fmt.Sprintf("192.0.2.%d", i+21), SubnetMask:"255.255.255.0", Gateway:"192.0.2.1", Firmware:"Illustrative", Activated:&active, DHCP:&dhcp, HTTPPort:&httpPort, RTSPPort:&rtspPort, Protocols:[]string{"sadp","onvif"}, LastSeen:time.Date(2026,10,27,0,0,0,0,time.UTC)}
  d.ID=d.Key()
  _, err = store.SaveRecord(sites.Record{SiteID:site.ID,Device:d,Location:locations[i],Notes:"Synthetic demonstration equipment. Do not connect.",CredentialMode:"none"}); must(err)
 }
 must(store.Visit(site.ID))
}
