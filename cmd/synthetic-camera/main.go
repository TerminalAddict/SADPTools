//go:build testcamera

// Explicit developer-only fixture. Never installed or launched by SADPTools.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming/testcamera"
)

func main() {
	hevc := flag.Bool("hevc", false, "Serve the synthetic HEVC pattern")
	flag.Parse()
	c, e := testcamera.New(*hevc, false, "")
	if e != nil {
		panic(e)
	}
	defer c.Close()
	dir, e := os.MkdirTemp("", "sadptools-synthetic-preview-")
	if e != nil {
		panic(e)
	}
	// Retain the isolated test directory for inspection. Never use real user data.
	dataDir := filepath.Join(dir, "SADPTools")
	if e = os.MkdirAll(dataDir, 0700); e != nil {
		panic(e)
	}
	s, e := sites.Open(filepath.Join(dataDir, "sites.sqlite"))
	if e != nil {
		panic(e)
	}
	site, e := s.SaveSite(sites.Site{Name: "SYNTHETIC PLAYBACK TEST", Networks: []string{"127.0.0.0/8"}, Protocols: []string{"sadp"}})
	if e != nil {
		panic(e)
	}
	u, _ := url.Parse(c.URL)
	port, _ := strconv.Atoi(u.Port())
	p := port
	_, e = s.SaveObserved(site.ID, []devices.Device{{ID: "synthetic-preview", MAC: "02:00:00:00:00:99", Serial: "SYNTHETIC-PREVIEW-ONLY", IPv4: "127.0.0.1", Model: "SYNTHETIC TEST PATTERN", RTSPPort: &p, LastSeen: time.Now()}})
	if e != nil {
		panic(e)
	}
	rows, _ := s.Records(site.ID)
	record := rows[0]
	record.CredentialMode = "none"
	if _, e = s.SaveRecord(record); e != nil {
		panic(e)
	}
	s.Close()
	settings := map[string]any{"theme": "dark", "timeoutSeconds": 5, "intervalSeconds": 30, "automatic": false, "checkUpdates": false, "updateChannel": "beta", "protocols": []string{"sadp"}}
	b, _ := json.Marshal(settings)
	if e = os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0600); e != nil {
		panic(e)
	}
	fmt.Printf("Synthetic RTSP source: %s\nIsolated XDG_CONFIG_HOME=%s\nSite %s; record %s\nNo camera credentials required. Press Ctrl+C to stop.\n", c.URL, dir, site.ID, record.ID)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
}
