//go:build ignore

// Creates a temporary, credential-free saved entry for a loopback RTSP relay.
package main

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sites"
)

func main() {
	port, err := strconv.Atoi(os.Args[2])
	must(err)
	st, err := sites.Open(filepath.Join(os.Args[1], "SADPTools", "sites.sqlite"))
	must(err)
	defer st.Close()
	site, err := st.SaveSite(sites.Site{Name: "Camera preview", Customer: "Authorised Live View screenshot session"})
	must(err)
	d := devices.Device{Name: "Live camera", IPv4: "127.0.0.1", RTSPPort: &port, UUID: "manual-live-preview"}
	d.ID = d.Key()
	_, err = st.SaveRecord(sites.Record{SiteID: site.ID, Device: d, CredentialMode: "none"})
	must(err)
	must(st.Visit(site.ID))
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
