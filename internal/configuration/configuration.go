// Package configuration manages single-device SADP transactions independently of Wails.
package configuration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

type NetworkSettings = sadp.NetworkSettings

type Request struct {
	Action         sadp.Action     `json:"action"`
	MAC            string          `json:"mac"`
	Interface      string          `json:"interface"`
	TimeoutSeconds int             `json:"timeoutSeconds"`
	Password       string          `json:"password"`
	Code           string          `json:"code"`
	Network        NetworkSettings `json:"network"`
}

// Prevent accidental structured/debug logging of credential-bearing requests.
func (r Request) String() string       { return "SADP request (credentials omitted)" }
func (r Request) GoString() string     { return r.String() }
func (r Request) LogValue() slog.Value { return slog.StringValue(r.String()) }

type Result struct {
	Action    sadp.Action `json:"action"`
	MAC       string      `json:"mac"`
	Interface string      `json:"interface"`
	Outcome   string      `json:"outcome"`
	Message   string      `json:"message"`
	ErrorCode string      `json:"errorCode,omitempty"`
	Challenge string      `json:"challenge,omitempty"`
	Sent      bool        `json:"sent"`
}

func (r Result) LogValue() slog.Value {
	return slog.GroupValue(slog.String("action", string(r.Action)), slog.String("outcome", r.Outcome))
}

type Engine interface {
	Execute(context.Context, Request) (Result, error)
}

var ErrBusy = errors.New("a discovery or device operation is already running")
var ErrRejected = errors.New("device rejected the request")
var ErrUnconfirmed = errors.New("device operation outcome is unconfirmed")

func validText(s string, max int) bool {
	if s == "" || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == 0xfffe || r == 0xffff {
			return false
		}
	}
	return true
}
func ValidatePassword(s string) error {
	if !validText(s, 64) || len(s) < 8 {
		return errors.New("new password must contain 8–64 bytes and no control characters; firmware may impose a shorter limit")
	}
	classes := [4]bool{}
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			classes[0] = true
		case unicode.IsLower(r):
			classes[1] = true
		case unicode.IsDigit(r):
			classes[2] = true
		default:
			classes[3] = true
		}
	}
	n := 0
	for _, v := range classes {
		if v {
			n++
		}
	}
	if n < 2 {
		return errors.New("new password needs at least two of uppercase, lowercase, numbers and symbols")
	}
	return nil
}
func (r Request) Validate() error {
	if !r.Action.Valid() {
		return errors.New("unsupported SADP action")
	}
	if devices.NormalizeMAC(r.MAC) == "" {
		return errors.New("a valid device MAC address is required")
	}
	if strings.TrimSpace(r.Interface) == "" {
		return errors.New("select one network interface for this device operation")
	}
	if r.TimeoutSeconds < 1 || r.TimeoutSeconds > 300 {
		return errors.New("operation timeout must be 1–300 seconds")
	}
	if !r.Action.ReadOnly() {
		if !validText(r.Password, 128) {
			return errors.New("password is required and must not contain control characters")
		}
		if r.Action != sadp.UpdateNetwork {
			if err := ValidatePassword(r.Password); err != nil {
				return err
			}
		}
	}
	if r.Action == sadp.ResetPassword || r.Action == sadp.SecurityCode {
		if !validText(r.Code, 4096) {
			return errors.New("a recovery code supplied by your authorised support provider is required (up to 4096 bytes)")
		}
	}
	if r.Action.ReadOnly() && (r.Password != "" || r.Code != "") {
		return errors.New("recovery-data requests must not include credentials")
	}
	if r.Action != sadp.ResetPassword && r.Action != sadp.SecurityCode && r.Code != "" {
		return errors.New("recovery code is only valid for a password reset")
	}
	if r.Action != sadp.UpdateNetwork && r.Network != (NetworkSettings{}) {
		return errors.New("network fields are only valid for an update request")
	}
	if r.Action == sadp.UpdateNetwork {
		return ValidateNetwork(r.Network)
	}
	return nil
}

// Explicit values even for DHCP preserve the static configuration, without guesses.
func ValidateNetwork(n NetworkSettings) error {
	ip, err := netip.ParseAddr(n.IPv4)
	if err != nil || !ip.Is4() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || n.IPv4 == "255.255.255.255" {
		return errors.New("enter a usable IPv4 device address")
	}
	maskAddress, err := netip.ParseAddr(n.SubnetMask)
	if err != nil || !maskAddress.Is4() {
		return errors.New("enter a valid IPv4 subnet mask")
	}
	mask := maskAddress.As4()
	ones, bits := net.IPMask(mask[:]).Size()
	if bits != 32 || ones < 1 || ones > 30 {
		return errors.New("subnet mask must be contiguous, /1 through /30")
	}
	prefix := netip.PrefixFrom(ip, ones).Masked()
	broadcast := func(a netip.Addr) bool {
		host := a.As4()
		for i := range host {
			if host[i]|mask[i] != 255 {
				return false
			}
		}
		return true
	}
	if ip == prefix.Addr() || broadcast(ip) {
		return errors.New("device address must not be the subnet or broadcast address")
	}
	gw, err := netip.ParseAddr(n.Gateway)
	if err != nil || !gw.Is4() {
		return errors.New("enter a gateway IPv4 address, or 0.0.0.0 for none")
	}
	if !gw.IsUnspecified() && (gw.IsLoopback() || gw.IsMulticast() || !prefix.Contains(gw) || gw == ip || gw == prefix.Addr() || broadcast(gw)) {
		return errors.New("gateway must be a different usable host in the same subnet, or 0.0.0.0")
	}
	if n.CommandPort < 1 || n.CommandPort > 65535 {
		return errors.New("SDK command port must be 1–65535; preserve the advertised port unless changing it intentionally")
	}
	return nil
}

// Cancellation after sending cannot roll back a request. Never retry automatically.
func uncertain(r Result, cause error) (Result, error) {
	r.Outcome = "unconfirmed"
	r.Message = "No conclusive reply. The request may have been applied; rediscover and verify before trying again."
	if r.Action.ReadOnly() {
		r.Message = "No conclusive recovery response; this firmware may not support the selected method."
	}
	return r, fmt.Errorf("%w: %w", ErrUnconfirmed, cause)
}
