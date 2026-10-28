package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

const sendUsage = `Usage:
 sadptools send <current-IP-or-0.0.0.0> <action> --mac MAC --interface NAME [options]
Actions: activate, update, exchangecode, getencryptstring, getencryptstring_v31,
         resetpassword, securitycode
The IP is a descriptive hint; the MAC selects the device over local multicast.
Exactly one interface is required. Each device write is sent once, without retries.
Activation first checks inactive status and exchanges encryption keys (8–16 byte password).
Network updates first check the camera's authentication profile and preserve HTTP/IPv6 settings.
Salted SADP network authentication supports current admin passwords up to 16 bytes.
Password input: prefer --password-stdin or --password-env VARIABLE.
Inline --password and --code are supported but visible in shell history/process lists.
Support-code recovery uses unverified legacy formats; modern encrypted support-code recovery is not implemented.
--dry-run validates and prints a redacted plan without opening a network socket.
`

type executeFunc func(context.Context, configuration.Request) (configuration.Result, error)

func runSend(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, execute executeFunc) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, sendUsage)
		return 0
	}
	if len(args) < 2 {
		fmt.Fprint(errOut, sendUsage)
		return 2
	}
	ip, err := netip.ParseAddr(args[0])
	if err != nil || !ip.Is4() {
		fmt.Fprintln(errOut, "target hint must be an IPv4 address; use 0.0.0.0 if unknown")
		return 2
	}
	action := sadp.Action(strings.ToLower(args[1]))
	if action == "resetpassword" {
		action = sadp.ResetPassword
	}
	if action == "securitycode" {
		action = sadp.SecurityCode
	}
	flags := flag.NewFlagSet("send", flag.ContinueOnError)
	// flag's default error messages can repeat arbitrary argument values. Keep secrets out.
	flags.SetOutput(io.Discard)
	r := configuration.Request{Action: action}
	flags.StringVar(&r.MAC, "mac", "", "device MAC (required)")
	flags.StringVar(&r.Interface, "interface", "", "one adapter name (required)")
	flags.StringVar(&r.Password, "password", "", "password (prefer stdin/environment)")
	flags.StringVar(&r.Code, "code", "", "support-provided reset code (prefer --code-file/--code-env)")
	passStdin := flags.Bool("password-stdin", false, "read password from stdin, one line")
	passEnv := flags.String("password-env", "", "read password from this environment variable")
	codeEnv := flags.String("code-env", "", "read recovery code from this environment variable")
	codeFile := flags.String("code-file", "", "read a plain-text recovery code (not vendor XML)")
	flags.StringVar(&r.Network.IPv4, "ip", "", "new/static IPv4 address (required for update)")
	flags.StringVar(&r.Network.SubnetMask, "mask", "", "subnet mask (required for update)")
	flags.StringVar(&r.Network.Gateway, "gateway", "", "gateway, or 0.0.0.0 for none (required for update)")
	flags.IntVar(&r.Network.CommandPort, "port", 0, "SDK command port (required for update; not HTTP)")
	flags.BoolVar(&r.Network.DHCP, "dhcp", false, "enable DHCP; explicit static values are still required")
	duration := flags.Duration("timeout", 10*time.Second, "whole-second timeout, 1s–5m")
	dryRun := flags.Bool("dry-run", false, "validate without any network activity")
	jsonOutput := flags.Bool("json", false, "structured result")
	debug := flags.Bool("debug", false, "metadata diagnostics only; no passwords, codes or raw XML")
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(out, sendUsage)
			flags.SetOutput(out)
			flags.PrintDefaults()
			return 0
		}
		fmt.Fprintln(errOut, "Invalid send arguments; use sadptools send --help or append --help after the action.")
		return 2
	}
	if flags.NArg() != 0 || *duration < time.Second || *duration > 300*time.Second || *duration%time.Second != 0 {
		fmt.Fprintln(errOut, "Unexpected arguments or timeout: use a whole number of seconds from 1s to 5m.")
		return 2
	}
	r.TimeoutSeconds = int(*duration / time.Second)
	passSources, codeSources := 0, 0
	if r.Password != "" {
		passSources++
	}
	if *passStdin {
		passSources++
	}
	if *passEnv != "" {
		passSources++
	}
	if r.Code != "" {
		codeSources++
	}
	if *codeEnv != "" {
		codeSources++
	}
	if *codeFile != "" {
		codeSources++
	}
	if passSources > 1 || codeSources > 1 {
		fmt.Fprintln(errOut, "Choose one password source and one recovery-code source.")
		return 2
	}
	if *passEnv != "" {
		r.Password = os.Getenv(*passEnv)
	}
	if *codeEnv != "" {
		r.Code = os.Getenv(*codeEnv)
	}
	readSecret := func(reader io.Reader, max int) (string, error) {
		b, err := io.ReadAll(io.LimitReader(reader, int64(max+3)))
		defer clear(b)
		if err != nil {
			return "", errors.New("could not read secret input")
		}
		s := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
		if len(s) > max || strings.ContainsAny(s, "\r\n") {
			return "", errors.New("secret input must be a single line within the allowed size")
		}
		return s, nil
	}
	if *passStdin {
		r.Password, err = readSecret(in, 128)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
	}
	if *codeFile != "" {
		f, openErr := os.Open(*codeFile)
		if openErr != nil {
			fmt.Fprintln(errOut, "Could not open recovery-code file.")
			return 2
		}
		r.Code, err = readSecret(f, 4096)
		_ = f.Close()
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		if strings.HasPrefix(strings.TrimSpace(r.Code), "<") {
			fmt.Fprintln(errOut, "Vendor XML recovery imports are not supported; provide a plain-text authorised recovery code.")
			return 2
		}
	}
	if err = r.Validate(); err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	if r.Action.ReadOnly() && (passSources != 0 || codeSources != 0) {
		fmt.Fprintln(errOut, "Recovery-data requests do not accept password or code arguments.")
		return 2
	}
	if *dryRun {
		preview := struct {
			Action    sadp.Action                    `json:"action"`
			MAC       string                         `json:"mac"`
			Interface string                         `json:"interface"`
			Network   *configuration.NetworkSettings `json:"network,omitempty"`
			Sent      bool                           `json:"sent"`
			Message   string                         `json:"message"`
		}{Action: r.Action, MAC: r.MAC, Interface: r.Interface, Message: "Validated only; no packet sent. Credentials omitted."}
		if r.Action == sadp.UpdateNetwork {
			preview.Network = &r.Network
		}
		if err = json.NewEncoder(out).Encode(preview); err != nil {
			return 1
		}
		return 0
	}
	if execute == nil {
		level := slog.LevelWarn
		if *debug {
			level = slog.LevelDebug
		}
		execute = configuration.New(slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: level}))).Execute
	}
	result, err := execute(ctx, r)
	if *jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(result); encodeErr != nil {
			return 1
		}
	} else {
		fmt.Fprintf(out, "%s: %s\n", result.Outcome, result.Message)
		if result.ErrorCode != "" {
			fmt.Fprintln(out, "Device error code:", result.ErrorCode)
		}
		if result.Challenge != "" {
			fmt.Fprintln(out, "Recovery data:", printable(result.Challenge))
		}
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		if errors.Is(err, context.Canceled) {
			return 130
		}
		if errors.Is(err, configuration.ErrRejected) {
			return 4
		}
		if errors.Is(err, configuration.ErrUnconfirmed) {
			return 5
		}
		return 1
	}
	return 0
}
