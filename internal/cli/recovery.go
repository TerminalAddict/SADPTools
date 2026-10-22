package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/recovery"
)

const recoveryUsage = `Usage:
 sadptools recovery questions <IP> --interface NAME [--port 80] [--timeout 10s] [--json]
 sadptools recovery reset <IP> --interface NAME --secrets-stdin --confirm-reset [--port 80] [--timeout 10s] [--json]
The camera must be on the adapter's IPv4 subnet. Questions reads availability
and the configured question text without submitting answers or changing passwords.
Reset reads one JSON object from stdin: {"answers":[{"id":1,"answer":"..."},...],"password":"..."}
Use the three IDs returned by questions. Never put real secrets in shell history.
Reset rechecks the configured IDs, encrypts the answers/password, and sends once.
A timeout after submission leaves the outcome unconfirmed. No automatic retries.
`

func runRecovery(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, engine recovery.Engine, interfaces func() ([]platform.Adapter, error)) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, recoveryUsage)
		return 0
	}
	if len(args) < 2 || (args[0] != "questions" && args[0] != "reset") {
		fmt.Fprint(errOut, recoveryUsage)
		return 2
	}
	f := flag.NewFlagSet("recovery", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	adapter := f.String("interface", "", "local adapter")
	port := f.Int("port", 80, "camera HTTP port")
	timeout := f.Duration("timeout", 10*time.Second, "whole-operation timeout")
	structured := f.Bool("json", false, "structured output")
	secrets := f.Bool("secrets-stdin", false, "read answers and password from stdin")
	confirm := f.Bool("confirm-reset", false, "explicitly submit the reset")
	if err := f.Parse(args[2:]); errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(out, recoveryUsage)
		return 0
	} else if err != nil || f.NArg() != 0 || *timeout < time.Second || *timeout > 300*time.Second || *adapter == "" {
		fmt.Fprintln(errOut, "Invalid recovery arguments; use sadptools recovery --help.")
		return 2
	}
	write := args[0] == "reset"
	if (write && (!*secrets || !*confirm)) || (!write && (*secrets || *confirm)) {
		fmt.Fprintln(errOut, "Reset requires --secrets-stdin and --confirm-reset; questions accepts neither.")
		return 2
	}
	var r recovery.Request
	if write {
		data, err := io.ReadAll(io.LimitReader(in, 8193))
		defer clear(data)
		if err != nil || len(data) > 8192 || json.Unmarshal(data, &r) != nil {
			fmt.Fprintln(errOut, "Secret input must be a single JSON object, at most 8192 bytes.")
			return 2
		}
		if err := r.Validate(); err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
	}
	all, err := interfaces()
	if err != nil {
		fmt.Fprintln(errOut, "Could not enumerate local adapters.")
		return 1
	}
	target, err := recovery.ForAddress(args[1], *port, *adapter, all)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if engine == nil {
		engine = recovery.New()
	}
	if !write {
		status, err := engine.Check(ctx, target)
		if *structured {
			if json.NewEncoder(out).Encode(status) != nil {
				return 1
			}
		} else {
			fmt.Fprintf(out, "%s: %s\n", status.State, printable(status.Message))
			for _, q := range status.Questions {
				fmt.Fprintf(out, "%d: %s\n", q.ID, printable(q.Text))
			}
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return 130
			}
			fmt.Fprintln(errOut, err)
			return 1
		}
		if status.State != "ready" {
			return 3
		}
		return 0
	}
	result, err := engine.Reset(ctx, target, r)
	if *structured {
		if json.NewEncoder(out).Encode(result) != nil {
			return 1
		}
	} else {
		fmt.Fprintf(out, "%s: %s\n", result.Outcome, printable(result.Message))
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(errOut, err)
	if errors.Is(err, configuration.ErrUnconfirmed) {
		return 5
	}
	if errors.Is(err, configuration.ErrRejected) {
		return 4
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}
