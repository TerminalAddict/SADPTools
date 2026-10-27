package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/version"
	"io"
	"strconv"
	"strings"
	"time"
)

func runTroubleshoot(ctx context.Context, args []string, out, errOut io.Writer) int {
	f := flag.NewFlagSet("troubleshoot", flag.ContinueOnError)
	f.SetOutput(errOut)
	target := f.String("ip", "", "one IPv4 address or hostname")
	adapter := f.String("interface", "", "optional source adapter")
	ports := f.String("ports", "80,443,554,8000", "1–12 explicit TCP ports")
	timeout := f.Duration("timeout", 2*time.Second, "per-probe timeout (100ms–10s)")
	report := f.Bool("report", false, "emit versioned remote diagnostic envelope")
	jsonOut := f.Bool("json", false, "emit diagnostic evidence")
	ping := f.Bool("icmp", true, "attempt ICMP, with permission fallback")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return 2
	}
	o := diagnostics.Options{Target: *target, Interface: *adapter, TimeoutMS: int(timeout.Milliseconds()), ICMP: *ping}
	for _, s := range strings.Split(*ports, ",") {
		p, e := strconv.Atoi(strings.TrimSpace(s))
		if e != nil {
			return 2
		}
		o.Ports = append(o.Ports, p)
		if p == 80 {
			o.HTTPPorts = append(o.HTTPPorts, p)
		}
		if p == 443 {
			o.HTTPSPorts = append(o.HTTPSPorts, p)
		}
	}
	if e := o.Validate(); e != nil {
		fmt.Fprintln(errOut, e)
		return 2
	}
	r, e := diagnostics.NewNetwork().Run(ctx, o, nil)
	if *report {
		if err := json.NewEncoder(out).Encode(diagnostics.Envelope{Schema: 1, Version: version.Value, Run: r}); err != nil {
			return 1
		}
	} else if *jsonOut {
		if err := json.NewEncoder(out).Encode(r); err != nil {
			return 1
		}
	} else {
		fmt.Fprint(out, r.Summary())
	}
	if ctx.Err() != nil {
		return 130
	}
	if e != nil {
		fmt.Fprintln(errOut, e)
		return 1
	}
	for _, step := range r.Steps {
		if step.Status == "failed" {
			return 3
		}
	}
	return 0
}
