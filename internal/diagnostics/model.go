// Package diagnostics contains bounded, read-only field diagnostics and safe evidence.
package diagnostics

import (
	"encoding/json"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"strings"
	"time"
)

type Step struct {
	Category   string `json:"category"`
	Test       string `json:"test"`
	Protocol   string `json:"protocol"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
	Next       string `json:"next"`
	Code       int    `json:"code,omitempty"`
	DurationMS int64  `json:"durationMS"`
	At         string `json:"at"`
}
type Run struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Interface string `json:"interface"`
	Source    string `json:"source"`
	Started   string `json:"started"`
	Completed string `json:"completed"`
	State     string `json:"state"`
	Steps     []Step `json:"steps"`
}

func NewRun(kind, target string) Run {
	return Run{Kind: kind, Target: target, Started: time.Now().UTC().Format(time.RFC3339Nano), State: "running", Steps: []Step{}}
}
func (r Run) Summary() string {
	var b strings.Builder
	b.WriteString(r.Kind + " · " + r.Target + " · " + r.Started + "\n")
	for _, s := range r.Steps {
		b.WriteString(s.Test + ": " + s.Status + " — " + s.Evidence + "\n")
		if s.Next != "" {
			b.WriteString("Next: " + s.Next + "\n")
		}
	}
	return b.String()
}

// SafeJSON recursively omits sensitive fields, including plates and authentication
// material. Unknown objects remain structural data, never executable markup.
func SafeJSON(v any, secrets ...string) []byte {
	raw, _ := json.Marshal(v)
	var x any
	_ = json.Unmarshal(raw, &x)
	var scrub func(any) any
	scrub = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			for k, value := range t {
				if protectedField(k) {
					t[k] = "[redacted]"
				} else {
					t[k] = scrub(value)
				}
			}
			return t
		case []any:
			for i := range t {
				t[i] = scrub(t[i])
			}
			return t
		case string:
			for _, secret := range secrets {
				if secret != "" {
					t = strings.ReplaceAll(t, secret, "[redacted]")
				}
			}
			if strings.HasPrefix(strings.TrimSpace(t), "<") {
				if safe, e := ScrubXML([]byte(t), secrets...); e == nil {
					return safe
				}
				return "[unparsed payload omitted]"
			}
			if strings.Contains(t, "-----BEGIN") {
				return "[key/certificate material omitted]"
			}
			return sanitize.Text(t)
		default:
			return v
		}
	}
	result, _ := json.MarshalIndent(scrub(x), "", "  ")
	clear(raw)
	return result
}

type Envelope struct {
	Schema  int    `json:"schema"`
	Version string `json:"version"`
	Run     Run    `json:"run"`
}
