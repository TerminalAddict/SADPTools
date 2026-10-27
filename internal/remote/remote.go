// Package remote runs a fixed read-only SADPTools CLI over authenticated SSH.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Schema = 1
const MaxOutput = 8 << 20

type Capabilities struct {
	Troubleshoot bool     `json:"troubleshoot"`
	Schema       int      `json:"schema"`
	Version      string   `json:"version"`
	Discovery    bool     `json:"discovery"`
	Protocols    []string `json:"protocols"`
}
type Report struct {
	Schema   int              `json:"schema"`
	Version  string           `json:"version"`
	At       string           `json:"at"`
	Devices  []devices.Device `json:"devices"`
	Warnings []string         `json:"warnings"`
	Partial  bool             `json:"partial"`
}
type Target struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	Fingerprint  string `json:"fingerprint"`
	KnownHosts   string `json:"knownHosts"`
	KeyPath      string `json:"keyPath"`
	Agent        bool   `json:"agent"`
	CredentialID string `json:"credentialID"`
	Executable   string `json:"executable"`
	Interface    string `json:"interface"`
	Protocols    string `json:"protocols"`
	Timeout      int    `json:"timeout"`
}
type Observation struct {
	Connectivity *diagnostics.Envelope `json:"connectivity,omitempty"`
	Target       Target                `json:"target"`
	At           string                `json:"at"`
	LatencyMS    int64                 `json:"latencyMS"`
	Report       Report                `json:"report"`
	Diagnostic   string                `json:"diagnostic"`
}

var safeArg = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

func (t Target) Validate() error {
	if t.Host == "" || len(t.Host) > 253 || strings.ContainsAny(t.Host, "\x00\r\n \t/\\") || t.Port < 1 || t.Port > 65535 || t.Username == "" || !safeArg.MatchString(t.Username) || len(t.Username) > 128 {
		return errors.New("enter an SSH host, port and username")
	}
	if t.Fingerprint == "" && t.KnownHosts == "" {
		return errors.New("approve an independently verified SHA256 host-key fingerprint or choose a known_hosts file")
	}
	if t.Fingerprint != "" && (!strings.HasPrefix(t.Fingerprint, "SHA256:") || len(t.Fingerprint) != 50) {
		return errors.New("host-key fingerprint must use OpenSSH SHA256 format")
	}
	if t.Executable == "" || !safeArg.MatchString(t.Executable) || strings.HasPrefix(t.Executable, "-") || len(t.Executable) > 1024 || t.Interface != "" && (!safeArg.MatchString(t.Interface) || len(t.Interface) > 128) {
		return errors.New("executable and adapter must contain only safe path/name characters")
	}
	if t.Protocols != "sadp" && t.Protocols != "onvif" && t.Protocols != "sadp,onvif" {
		return errors.New("choose sadp, onvif or sadp,onvif")
	}
	if t.Timeout < 1 || t.Timeout > 300 {
		return errors.New("remote discovery timeout must be 1–300 seconds")
	}
	return nil
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func Commands(t Target) (string, string, error) {
	if e := t.Validate(); e != nil {
		return "", "", e
	}
	probe := quote(t.Executable) + " capabilities --json"
	scan := quote(t.Executable) + " discover --report --timeout " + strconv.Itoa(t.Timeout) + "s --protocols " + quote(t.Protocols)
	if t.Interface != "" {
		scan += " --interface " + quote(t.Interface)
	} else {
		scan += " --all-interfaces"
	}
	return probe, scan, nil
}

type bounded struct {
	bytes.Buffer
	max int
}

func (b *bounded) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errors.New("remote output exceeded limit")
	}
	return b.Buffer.Write(p)
}
func decode(data []byte, v any) error {
	if len(data) > MaxOutput {
		return errors.New("remote response exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if e := d.Decode(v); e != nil {
		return errors.New("remote CLI did not return valid JSON; check its installation/version")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("remote CLI returned trailing data")
	}
	return nil
}
func Parse(data []byte) (Report, error) {
	var r Report
	if e := decode(data, &r); e != nil {
		return r, e
	}
	if r.Schema != Schema || r.Version == "" || len(r.Devices) > 10000 || len(r.Warnings) > 1000 {
		return r, errors.New("remote CLI schema is incompatible or result is oversized; install SADPTools v0.5 or newer")
	}
	for i := range r.Devices {
		d := &r.Devices[i]
		if d.Key() == "" || net.ParseIP(d.IPv4) == nil || len(d.Interfaces) > 128 || len(d.XAddrs) > 64 {
			return Report{}, errors.New("remote device identity/address is invalid")
		}
		raw, _ := json.Marshal(d)
		if len(raw) > 64<<10 {
			return Report{}, errors.New("remote device metadata is oversized")
		}
		d.Name = sanitize.Label(d.Name)
		d.Model = sanitize.Label(d.Model)
		d.Firmware = sanitize.Label(d.Firmware)
	}
	for i := range r.Warnings {
		r.Warnings[i] = sanitize.Text(r.Warnings[i])
	}
	return r, nil
}
func Run(ctx context.Context, t Target, secret vault.Secret) (Observation, error) {
	return run(ctx, t, secret, "")
}
func Troubleshoot(ctx context.Context, t Target, secret vault.Secret, address string) (Observation, error) {
	ip := net.ParseIP(address)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || address == "255.255.255.255" {
		return Observation{}, errors.New("remote diagnostic requires one selected unicast IPv4 address")
	}
	return run(ctx, t, secret, ip.String())
}
func run(ctx context.Context, t Target, secret vault.Secret, diagnosticTarget string) (Observation, error) {
	out := Observation{Target: t, At: time.Now().UTC().Format(time.RFC3339Nano)}
	probe, scan, e := Commands(t)
	if e != nil {
		return out, e
	}
	if diagnosticTarget != "" {
		scan = quote(t.Executable) + " troubleshoot --ip " + quote(diagnosticTarget) + " --timeout 2s --ports 80,443,554,8000 --report"
		if t.Interface != "" {
			scan += " --interface " + quote(t.Interface)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(t.Timeout+30)*time.Second)
	defer cancel()
	var verify ssh.HostKeyCallback
	if t.KnownHosts != "" {
		verify, e = knownhosts.New(t.KnownHosts)
		if e != nil {
			return out, errors.New("cannot read SSH known_hosts file")
		}
	}
	if t.Fingerprint != "" {
		prior := verify
		verify = func(host string, addr net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != t.Fingerprint {
				return errors.New("SSH host key changed or is not approved; verify it independently before updating trust")
			}
			if prior != nil {
				return prior(host, addr, key)
			}
			return nil
		}
	}
	auth := []ssh.AuthMethod{}
	if t.KeyPath != "" {
		f, e := os.Open(t.KeyPath)
		if e != nil {
			return out, errors.New("cannot open selected SSH identity file")
		}
		b, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
		f.Close()
		if e != nil || len(b) > 1<<20 {
			return out, errors.New("SSH identity file is unreadable/oversized")
		}
		var signer ssh.Signer
		if secret.PrivacyPassword != "" {
			signer, e = ssh.ParsePrivateKeyWithPassphrase(b, []byte(secret.PrivacyPassword))
		} else {
			signer, e = ssh.ParsePrivateKey(b)
		}
		clear(b)
		if e != nil {
			return out, errors.New("SSH identity could not be unlocked; assign its passphrase through the vault")
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if t.Agent {
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return out, errors.New("SSH agent is unavailable; select an identity file or vault credential")
		}
		conn, e := net.DialTimeout("unix", socket, 3*time.Second)
		if e != nil {
			return out, errors.New("cannot connect to SSH agent")
		}
		defer conn.Close()
		auth = append(auth, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
	}
	if secret.Password != "" {
		auth = append(auth, ssh.Password(secret.Password))
	}
	if len(auth) == 0 {
		return out, errors.New("select an identity file, SSH agent or vault password")
	}
	address := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	started := time.Now()
	raw, e := (&net.Dialer{Timeout: 6 * time.Second}).DialContext(ctx, "tcp", address)
	if e != nil {
		return out, errors.New("SSH host is unreachable; check VPN, routing and port")
	}
	defer raw.Close()
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	defer stop()
	raw.SetDeadline(time.Now().Add(10 * time.Second))
	conn, chans, reqs, e := ssh.NewClientConn(raw, address, &ssh.ClientConfig{User: t.Username, Auth: auth, HostKeyCallback: verify, Timeout: 10 * time.Second})
	if e != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, errors.New("SSH handshake/authentication or host-key verification failed; check approved fingerprint and credentials")
	}
	raw.SetDeadline(time.Time{})
	out.LatencyMS = time.Since(started).Milliseconds()
	client := ssh.NewClient(conn, chans, reqs)
	defer client.Close()
	run := func(command string) ([]byte, string, error) {
		sess, e := client.NewSession()
		if e != nil {
			return nil, "", errors.New("SSH channel could not open")
		}
		defer sess.Close()
		b, diag := &bounded{max: MaxOutput}, &bounded{max: 32 << 10}
		sess.Stdout = b
		sess.Stderr = diag
		cancelSession := context.AfterFunc(ctx, func() { _ = sess.Signal(ssh.SIGTERM); sess.Close() })
		defer cancelSession()
		e = sess.Run(command)
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		diagnostic := sanitize.Text(diag.String())
		for _, value := range []string{secret.Password, secret.PrivacyPassword} {
			if value != "" {
				diagnostic = strings.ReplaceAll(diagnostic, value, "[redacted]")
			}
		}
		return b.Bytes(), diagnostic, e
	}
	data, _, e := run(probe)
	if e != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, errors.New("remote SADPTools capabilities command failed; check executable path and install v0.5 or newer")
	}
	var caps Capabilities
	if e = decode(data, &caps); e != nil {
		return out, e
	}
	if caps.Schema != Schema || diagnosticTarget == "" && !caps.Discovery || diagnosticTarget != "" && !caps.Troubleshoot {
		return out, errors.New("remote CLI does not support this discovery schema")
	}
	data, out.Diagnostic, e = run(scan)
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if e != nil {
		var exit *ssh.ExitError
		if !errors.As(e, &exit) || exit.ExitStatus() != 3 {
			return out, fmt.Errorf("remote discovery failed; %s", sanitize.Text(out.Diagnostic))
		}
	}
	if diagnosticTarget != "" {
		var report diagnostics.Envelope
		if e = decode(data, &report); e != nil {
			return out, e
		}
		if report.Schema != 1 || report.Version == "" || report.Run.Target != diagnosticTarget || len(report.Run.Steps) > 100 || containsCredential(report, secret) {
			return out, errors.New("invalid or unsafe remote diagnostics envelope")
		}
		safe := diagnostics.SafeJSON(report)
		if e = json.Unmarshal(safe, &report); e != nil {
			return out, e
		}
		out.Connectivity = &report
		return out, nil
	}
	out.Report, e = Parse(data)
	if e == nil && containsCredential(out.Report, secret) {
		out.Report = Report{}
		return out, errors.New("remote result contained credential material and was discarded")
	}
	return out, e
}

func containsCredential(value any, secret vault.Secret) bool {
	raw, _ := json.Marshal(value)
	var decoded any
	_ = json.Unmarshal(raw, &decoded)
	var visit func(any) bool
	visit = func(v any) bool {
		switch x := v.(type) {
		case string:
			for _, s := range []string{secret.Password, secret.PrivacyPassword} {
				if s != "" && strings.Contains(x, s) {
					return true
				}
			}
		case map[string]any:
			for _, v := range x {
				if visit(v) {
					return true
				}
			}
		case []any:
			for _, v := range x {
				if visit(v) {
					return true
				}
			}
		}
		return false
	}
	return visit(decoded)
}
