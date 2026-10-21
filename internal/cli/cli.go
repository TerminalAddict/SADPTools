package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/TerminalAddict/SADPTools/internal/discovery"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/version"
)

const usage = `SADPTools — Hikvision discovery and device management
Usage:
  sadptools discover [--interface NAME | --all-interfaces] [--timeout 10s] [--json] [--debug]
  sadptools interfaces [--json]
  sadptools send <IP> <action> --mac MAC --interface NAME [options]
  sadptools send --help
  sadptools --version

Discovery defaults to all suitable IPv4 adapters. Ctrl+C cancels an active scan.
Exit codes: 0 success (including no devices), 1 failure, 2 usage, 3 partial scan,
4 device rejected command, 5 command outcome unconfirmed, 130 cancelled.
`

type scanFunc func(context.Context, discovery.Options, *slog.Logger) (discovery.Result, error)

func Run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) > 0 && args[0] == "send" {
		return runSend(ctx, args[1:], os.Stdin, out, errOut, nil)
	}
	return run(ctx, args, out, errOut, platform.Interfaces, func(ctx context.Context, opts discovery.Options, log *slog.Logger) (discovery.Result, error) {
		return discovery.New(log).Scan(ctx, opts, nil)
	})
}

func run(ctx context.Context, args []string, out, errOut io.Writer, interfaces func() ([]platform.Adapter, error), scan scanFunc) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return 0
	}
	if args[0] == "--version" {
		if len(args) > 1 {
			return 2
		}
		fmt.Fprintln(out, "sadptools", version.Value)
		return 0
	}
	if args[0] != "discover" && args[0] != "interfaces" {
		fmt.Fprint(errOut, usage)
		return 2
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(errOut)
	jsonOutput := flags.Bool("json", false, "emit JSON")
	var adapter string
	var all, debug bool
	timeout := discovery.DefaultTimeout
	if args[0] == "discover" {
		flags.StringVar(&adapter, "interface", "", "adapter name (exact match)")
		flags.BoolVar(&all, "all-interfaces", false, "scan all suitable adapters (default)")
		flags.BoolVar(&debug, "debug", false, "detailed diagnostics on stderr")
		flags.DurationVar(&timeout, "timeout", timeout, "discovery duration")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || (adapter != "" && all) || timeout <= 0 || timeout > discovery.MaxTimeout {
		fmt.Fprintln(errOut, "Invalid arguments: use --help; timeout must be >0 and <=5m.")
		return 2
	}
	encode := func(v any) int {
		e := json.NewEncoder(out)
		e.SetIndent("", "  ")
		if err := e.Encode(v); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}
	if args[0] == "interfaces" {
		adapters, err := interfaces()
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		if *jsonOutput {
			return encode(adapters)
		}
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "INDEX\tINTERFACE\tIPv4 ADDRESSES\tSTATUS")
		for _, a := range adapters {
			status := a.Reason
			if a.Suitable {
				status = "ready"
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", a.Index, a.Name, strings.Join(a.Addresses, ", "), status)
		}
		if err := w.Flush(); err != nil {
			return 1
		}
		return 0
	}
	level := slog.LevelWarn
	if debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: level}))
	result, err := scan(ctx, discovery.Options{Interface: adapter, Timeout: timeout}, log)
	if *jsonOutput {
		if code := encode(result.Devices); code != 0 {
			return code
		}
	} else {
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "IP ADDRESS\tMODEL / TYPE\tMAC\tSERIAL\tFIRMWARE\tACTIVATION\tINTERFACE")
		for _, d := range result.Devices {
			status := "unknown"
			if d.Activated != nil {
				status = "inactive"
				if *d.Activated {
					status = "active"
				}
			}
			names := []string{}
			for _, a := range d.Interfaces {
				names = append(names, a.Name)
			}
			model := d.Model
			if model == "" {
				model = d.DeviceType
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", printable(d.IPv4), printable(model), printable(d.MAC), printable(d.Serial), printable(d.Firmware), status, strings.Join(names, ", "))
		}
		if flushErr := w.Flush(); flushErr != nil {
			return 1
		}
		fmt.Fprintf(out, "%d device(s) discovered.\n", len(result.Devices))
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		if errors.Is(err, context.Canceled) {
			return 130
		}
		return 1
	}
	if len(result.Warnings) > 0 {
		return 3
	}
	return 0
}

// Keep untrusted device strings from emitting terminal control sequences.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
}
