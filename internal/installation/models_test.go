package installation

import (
	"math"
	"testing"
)

func TestPlanBoundsAndSiteIdentity(t *testing.T) {
	p := Plan{ImageID: "image", Markers: []Marker{{ID: "one", DeviceID: "camera", X: .5, Y: .5}}}
	if e := p.Validate(map[string]bool{"camera": true}); e != nil {
		t.Fatal(e)
	}
	if e := p.Validate(map[string]bool{"other": true}); e == nil {
		t.Fatal("cross-site marker")
	}
	p.Markers[0].X = math.NaN()
	if e := p.Validate(map[string]bool{"camera": true}); e == nil {
		t.Fatal("invalid coordinates")
	}
}
func TestChecklistRequiresEvidenceForAutomaticPass(t *testing.T) {
	c := Commissioning{Checks: []Check{{DeviceID: "camera", Test: "live-video", Result: "pass", Source: "automatic"}}}
	if e := c.Validate(map[string]bool{"camera": true}); e == nil {
		t.Fatal("unverified pass")
	}
	c.Checks[0].Source = "manual"
	if e := c.Validate(map[string]bool{"camera": true}); e != nil {
		t.Fatal(e)
	}
}
