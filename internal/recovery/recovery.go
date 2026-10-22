// Package recovery implements local ISAPI security-question recovery without Wails.
package recovery

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
)

type Target struct {
	Address   string
	Port      int
	Interface string
	Source    string
}
type Question struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}
type Status struct {
	State     string     `json:"state"`
	Message   string     `json:"message"`
	Questions []Question `json:"questions"`
	CheckID   string     `json:"checkID"`
}
type Answer struct {
	ID     int    `json:"id"`
	Answer string `json:"answer"`
}
type Request struct {
	CheckID  string   `json:"checkID"`
	Answers  []Answer `json:"answers"`
	Password string   `json:"password"`
}

func (r Request) String() string       { return "security-question reset (credentials omitted)" }
func (r Request) GoString() string     { return r.String() }
func (r Request) LogValue() slog.Value { return slog.StringValue(r.String()) }
func (a Answer) String() string        { return "security answer (omitted)" }
func (a Answer) GoString() string      { return a.String() }
func (a Answer) LogValue() slog.Value  { return slog.StringValue(a.String()) }

type Result struct {
	Outcome     string `json:"outcome"`
	Message     string `json:"message"`
	Sent        bool   `json:"sent"`
	RetryTimes  *int   `json:"retryTimes,omitempty"`
	LockSeconds *int   `json:"lockSeconds,omitempty"`
}
type Engine interface {
	Check(context.Context, Target) (Status, error)
	Reset(context.Context, Target, Request) (Result, error)
}

func (r Request) Validate() error {
	if len(r.Answers) != 3 {
		return errors.New("answer all three configured security questions")
	}
	seen := map[int]bool{}
	for _, a := range r.Answers {
		if a.ID < 1 || a.ID > 4096 || seen[a.ID] || !validText(a.Answer, 512) || utf8.RuneCountInString(a.Answer) > 128 {
			return errors.New("provide three distinct question IDs and nonempty answers of at most 128 characters (512 UTF-8 bytes), without control characters")
		}
		seen[a.ID] = true
	}
	return configuration.ValidatePassword(r.Password)
}
func validText(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, c := range value {
		if unicode.IsControl(c) || c == 0xfffe || c == 0xffff {
			return false
		}
	}
	return true
}
func validIP(address string) bool {
	ip, err := netip.ParseAddr(address)
	return err == nil && ip.Is4() && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLoopback() && address != "255.255.255.255"
}
func (t Target) validate() error {
	if !validIP(t.Address) || !validIP(t.Source) || t.Port < 1 || t.Port > 65535 || t.Interface == "" {
		return errors.New("security-question recovery requires a usable device IPv4 address, HTTP port and local adapter address")
	}
	return nil
}

// ForAddress chooses a current on-link source address, including on adapters
// with multiple IPv4 addresses. Publicly numbered local networks are supported.
func ForAddress(address string, port int, adapter string, all []platform.Adapter) (Target, error) {
	if !validIP(address) || port < 1 || port > 65535 {
		return Target{}, errors.New("device has no usable advertised IPv4 address and HTTP port")
	}
	ip := netip.MustParseAddr(address)
	for _, a := range all {
		if a.Name != adapter || !a.Suitable {
			continue
		}
		for _, value := range a.Addresses {
			p, err := netip.ParsePrefix(value)
			if err != nil || !p.Addr().Is4() || !p.Contains(ip) || p.Addr() == ip || p.Bits() < 1 || p.Bits() > 30 {
				continue
			}
			v := ip.As4()
			n := uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
			mask := ^uint32(0) >> p.Bits()
			if ip == p.Masked().Addr() || n&mask == mask {
				continue
			}
			t := Target{address, port, a.Name, p.Addr().String()}
			if err := t.validate(); err == nil {
				return t, nil
			}
		}
	}
	return Target{}, errors.New("no usable IPv4 address on the selected adapter shares the camera subnet; SADP discovery alone does not provide web connectivity")
}
func ForDevice(d devices.Device, requested, preferred string, all []platform.Adapter) (Target, error) {
	if d.HTTPPort == nil {
		return Target{}, errors.New("camera did not advertise an HTTP port; rediscover or use its web interface")
	}
	names := []string{}
	for _, observed := range d.Interfaces {
		if observed.Name != "" {
			names = append(names, observed.Name)
		}
	}
	if requested != "" {
		for _, name := range names {
			if name == requested {
				return ForAddress(d.IPv4, *d.HTTPPort, name, all)
			}
		}
		return Target{}, errors.New("select an adapter on which this camera was discovered")
	}
	for _, name := range names {
		if name == preferred {
			if t, err := ForAddress(d.IPv4, *d.HTTPPort, name, all); err == nil {
				return t, nil
			}
		}
	}
	for _, name := range names {
		if t, err := ForAddress(d.IPv4, *d.HTTPPort, name, all); err == nil {
			return t, nil
		}
	}
	return Target{}, errors.New("camera is not on a usable local IPv4 subnet through its discovered adapters; check its address and network connection")
}

func SameQuestions(questions []Question, answers []Answer) bool {
	if len(questions) != 3 || len(answers) != 3 {
		return false
	}
	ids := map[int]bool{}
	for _, q := range questions {
		ids[q.ID] = true
	}
	for _, a := range answers {
		if !ids[a.ID] {
			return false
		}
		delete(ids, a.ID)
	}
	return len(ids) == 0
}

func cleanLabel(s string) string { return strings.TrimSpace(s) }
