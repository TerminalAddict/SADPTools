package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type TraceOptions struct {
	Interface string `json:"interface"`
	Target    string `json:"target"`
	Seconds   int    `json:"seconds"`
	Approved  bool   `json:"approved"`
}
type TraceCapability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

func TraceSupport() TraceCapability {
	_, e := exec.LookPath("dumpcap")
	if e != nil {
		return TraceCapability{Reason: "Optional dumpcap is not installed. Metadata bundles remain available. Install/authorise capture separately; SADPTools never installs drivers or elevates itself."}
	}
	return TraceCapability{Available: true, Reason: "dumpcap found; OS capture permission is checked only when you explicitly start. Windows may require an existing Npcap installation."}
}
func TraceArguments(o TraceOptions, output string) ([]string, error) {
	ip, e := netip.ParseAddr(o.Target)
	if e != nil || !usable(ip) || o.Interface == "" || o.Seconds < 1 || o.Seconds > 60 || !o.Approved {
		return nil, errors.New("approve a raw trace on exactly one adapter and one IPv4 host for 1–60 seconds")
	}
	return []string{"-q", "-p", "-i", o.Interface, "-f", "host " + ip.String(), "-a", "duration:" + strconv.Itoa(o.Seconds), "-a", "filesize:8192", "-w", output}, nil
}

// CaptureTrace never installs a driver, invokes a shell or requests elevation.
// Failed/cancelled captures are removed; successful sensitive files are private.
func CaptureTrace(ctx context.Context, o TraceOptions, destination string) error {
	if _, e := TraceArguments(o, destination); e != nil {
		return e
	}
	adapters, e := platform.Interfaces()
	if e != nil {
		return e
	}
	found := false
	for _, a := range adapters {
		if a.Name == o.Interface {
			found = true
		}
	}
	if !found {
		return errors.New("selected capture adapter no longer exists")
	}
	tool, e := exec.LookPath("dumpcap")
	if e != nil {
		return errors.New(TraceSupport().Reason)
	}
	f, e := os.CreateTemp(filepath.Dir(destination), ".sadptools-trace-*.pcapng")
	if e != nil {
		return e
	}
	temp := f.Name()
	f.Close()
	defer os.Remove(temp)
	captureName, e := platform.CaptureAdapter(o.Interface)
	if e != nil {
		return e
	}
	o.Interface = captureName
	args, e := TraceArguments(o, temp)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(o.Seconds+10)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if e = cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("capture failed: check dumpcap/Npcap permissions and adapter name; no elevation or driver installation attempted")
	}
	info, e := os.Stat(temp)
	if e != nil || info.Size() == 0 || info.Size() > 9<<20 {
		return errors.New("capture was empty or exceeded its bound")
	}
	if e = os.Chmod(temp, 0600); e != nil {
		return e
	}
	if e = os.Rename(temp, destination); e != nil {
		return fmt.Errorf("could not save capture to chosen location")
	}
	return nil
}
