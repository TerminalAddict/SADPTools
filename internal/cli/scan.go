package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/ipscan"
	"io"
	"strconv"
	"strings"
)

func runIPScan(ctx context.Context, args []string, out, errOut io.Writer) int {
	f := flag.NewFlagSet("scan", flag.ContinueOnError)
	f.SetOutput(errOut)
	o := ipscan.Options{TimeoutMS: 500, Workers: 24, Rate: 60, ICMP: true}
	f.StringVar(&o.Interface, "interface", "", "exact local adapter name (required)")
	f.StringVar(&o.CIDR, "cidr", "", "IPv4 subnet; defaults to adapter subnet")
	f.StringVar(&o.Start, "start", "", "first IPv4 address")
	f.StringVar(&o.End, "end", "", "last IPv4 address")
	f.IntVar(&o.TimeoutMS, "timeout-ms", 500, "per-probe timeout 100–3000 ms")
	f.IntVar(&o.Workers, "workers", 24, "concurrent hosts 1–64")
	f.IntVar(&o.Rate, "rate", 60, "maximum hosts started per second (1–200)")
	f.BoolVar(&o.ICMP, "icmp", true, "send ICMP echo probes")
	f.BoolVar(&o.ConfirmScope, "confirm-scope", false, "explicitly confirm public, routed or >1024-address scope")
	ports := f.String("tcp-ports", "", "optional comma-separated TCP connect ports; disabled by default")
	structured := f.Bool("json", false, "JSON output")
	if e := f.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if f.NArg() != 0 || o.Interface == "" || (o.CIDR != "" && (o.Start != "" || o.End != "")) {
		fmt.Fprintln(errOut, "Choose an adapter and either CIDR or start/end range.")
		return 2
	}
	if *ports != "" {
		for _, v := range strings.Split(*ports, ",") {
			p, e := strconv.Atoi(strings.TrimSpace(v))
			if e != nil {
				fmt.Fprintln(errOut, "Invalid TCP port.")
				return 2
			}
			o.TCPPorts = append(o.TCPPorts, p)
		}
	}
	engine := ipscan.New()
	adapters, e := engine.Interfaces()
	if e != nil {
		fmt.Fprintln(errOut, "Cannot enumerate adapters.")
		return 1
	}
	scope, e := ipscan.Prepare(o, adapters)
	if e != nil {
		fmt.Fprintln(errOut, e)
		return 2
	}
	if scope.NeedsConfirmation && !o.ConfirmScope {
		fmt.Fprintf(errOut, "Scope %s–%s (%d addresses) is public, routed or large. Confirm authorisation using --confirm-scope.\n", scope.Start, scope.End, scope.Total)
		return 2
	}
	result, e := engine.Scan(ctx, o, nil, nil, nil)
	if *structured {
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return 1
		}
	} else {
		fmt.Fprintln(out, "IP ADDRESS\tSTATUS\tEVIDENCE")
		for _, h := range result.Hosts {
			sources := []string{}
			for _, v := range h.Evidence {
				sources = append(sources, v.Probe+":"+v.State)
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", h.IP, h.Status, strings.Join(sources, ", "))
		}
		fmt.Fprintln(out, "No response does not mean an address is free.")
	}
	for _, w := range result.Warnings {
		fmt.Fprintln(errOut, w)
	}
	if e != nil {
		if errors.Is(e, context.Canceled) {
			return 130
		}
		fmt.Fprintln(errOut, e)
		return 1
	}
	return 0
}
