// Package recovery implements local ISAPI security-question recovery without Wails.
package recovery

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type Target struct {
	Address           string
	Port              int
	Interface         string
	Source            string
	Scheme            string
	CertificateSHA256 string
}
type Question struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}
type Status struct {
	State     string                `json:"state"`
	Message   string                `json:"message"`
	Questions []Question            `json:"questions"`
	CheckID   string                `json:"checkID"`
	Endpoint  webinterface.Endpoint `json:"endpoint"`
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
	if t.Scheme != "" && t.Scheme != "http" && t.Scheme != "https" {
		return errors.New("unsupported recovery web protocol")
	}
	if t.CertificateSHA256 != "" {
		pin, err := hex.DecodeString(t.CertificateSHA256)
		if err != nil || len(pin) != 32 || t.Scheme == "http" {
			return errors.New("HTTPS trust requires the camera's SHA-256 certificate fingerprint")
		}
	}
	return nil
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
