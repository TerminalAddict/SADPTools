// Package installation defines site evidence and interactive planning data.
// It performs no device/network operations and has no desktop dependency.
package installation

import (
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"math"
	"strings"
	"time"
)

type Marker struct {
	ID       string  `json:"id"`
	DeviceID string  `json:"deviceID"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Bearing  float64 `json:"bearing"`
	Cone     float64 `json:"cone"`
	Label    string  `json:"label"`
	Layer    string  `json:"layer"`
	Notes    string  `json:"notes"`
}
type Plan struct {
	ImageID string   `json:"imageID"`
	Notes   string   `json:"notes"`
	Markers []Marker `json:"markers"`
}

func (p Plan) Validate(devices map[string]bool) error {
	if p.ImageID == "" || len(p.Markers) > 2000 || len(p.Notes) > 8192 {
		return errors.New("choose a plan image and keep markers/notes within limits")
	}
	ids := map[string]bool{}
	for _, m := range p.Markers {
		if m.ID == "" || ids[m.ID] || !devices[m.DeviceID] || !finite(m.X, 0, 1) || !finite(m.Y, 0, 1) || !finite(m.Bearing, 0, 360) || !finite(m.Cone, 0, 180) || len(m.Label) > 256 || len(m.Layer) > 64 || len(m.Notes) > 4096 {
			return errors.New("invalid marker, coordinates or device from another site")
		}
		ids[m.ID] = true
	}
	return nil
}
func finite(v, min, max float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= min && v <= max
}

var Checks = []string{"reachable", "focus-aim", "timestamp-ntp", "live-video", "recording", "night-ir", "motion-alarm", "ptz-presets", "credentials-handover"}

type Check struct {
	DeviceID    string   `json:"deviceID"`
	Test        string   `json:"test"`
	Result      string   `json:"result"`
	Source      string   `json:"source"`
	At          string   `json:"at"`
	Notes       string   `json:"notes"`
	EvidenceIDs []string `json:"evidenceIDs"`
}
type Commissioning struct {
	Title      string  `json:"title"`
	Technician string  `json:"technician"`
	Date       string  `json:"date"`
	Project    string  `json:"project"`
	Summary    string  `json:"summary"`
	Signoff    string  `json:"signoff"`
	BrandingID string  `json:"brandingID"`
	Checks     []Check `json:"checks"`
}
type Branding struct {
	Company string `json:"company"`
	Details string `json:"details"`
	LogoID  string `json:"logoID"`
}

func (c *Commissioning) Validate(devices map[string]bool) error {
	if len(c.Checks) > 10000 || len(c.Title) > 200 || len(c.Technician) > 200 || len(c.Project) > 200 || len(c.Summary) > 8192 || len(c.Signoff) > 8192 {
		return errors.New("commissioning form exceeds limits")
	}
	if c.Date != "" {
		if _, e := time.Parse("2006-01-02", c.Date); e != nil {
			return errors.New("enter the commissioning date as YYYY-MM-DD")
		}
	}
	validTests := map[string]bool{}
	for _, v := range Checks {
		validTests[v] = true
	}
	seen := map[string]bool{}
	for i := range c.Checks {
		x := &c.Checks[i]
		key := x.DeviceID + "/" + x.Test
		if !devices[x.DeviceID] || !validTests[x.Test] || seen[key] || len(x.EvidenceIDs) > 20 || len(x.Notes) > 4096 {
			return errors.New("invalid/duplicate checklist item or another site's device")
		}
		seen[key] = true
		switch x.Result {
		case "pass", "fail", "not-tested", "not-applicable":
		default:
			return errors.New("choose Pass, Fail, Not tested or Not applicable")
		}
		if x.Source == "" {
			x.Source = "manual"
		}
		if x.Source != "manual" && len(x.EvidenceIDs) == 0 {
			return errors.New("automatic checklist results need evidence")
		}
		if x.Source == "" {
			x.Source = "manual"
		}
		x.Notes = sanitize.Text(x.Notes)
		if x.At == "" {
			x.At = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	c.Title = sanitize.Label(c.Title)
	c.Technician = sanitize.Label(c.Technician)
	c.Project = sanitize.Label(c.Project)
	c.Summary = sanitize.Text(c.Summary)
	c.Signoff = sanitize.Text(c.Signoff)
	if strings.TrimSpace(c.Title) == "" {
		c.Title = "Commissioning and handover"
	}
	return nil
}
