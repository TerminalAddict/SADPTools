package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/cameraip"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

type CameraIPRequest struct {
	SiteID       string           `json:"siteID"`
	CredentialID string           `json:"credentialID"`
	Target       cameraip.Options `json:"target"`
}
type CameraIPCheck struct {
	Token        string          `json:"token"`
	SiteID       string          `json:"siteID"`
	CredentialID string          `json:"credentialID"`
	Result       cameraip.Result `json:"result"`
}
type CameraIPSave struct {
	Token           string `json:"token"`
	SiteID          string `json:"siteID"`
	Name            string `json:"name"`
	Location        string `json:"location"`
	AllowUnverified bool   `json:"allowUnverified"`
}
type pendingCameraIP struct {
	Check   CameraIPCheck
	Expires time.Time
}

func (m *Manager) CheckCameraIP(parent context.Context, o CameraIPRequest, session vault.Secret) (CameraIPCheck, error) {
	if e := o.Target.Validate(); e != nil {
		return CameraIPCheck{}, e
	}
	view, e := m.Store.View(o.SiteID)
	if e != nil {
		return CameraIPCheck{}, e
	}
	if view.Site.Archived {
		return CameraIPCheck{}, errors.New("choose an active site")
	}
	secret := session
	if o.CredentialID != "" {
		if session.Username != "" || session.Password != "" {
			return CameraIPCheck{}, errors.New("choose saved credentials or a one-time login")
		}
		secret, e = m.Store.Secret(o.SiteID, o.CredentialID)
		if e != nil {
			return CameraIPCheck{}, e
		}
	}
	type answer struct {
		check CameraIPCheck
		err   error
	}
	done := make(chan answer, 1)
	e = m.start("Camera by IP", o.SiteID, func(task context.Context) error {
		defer secret.Clear()
		ctx, cancel := context.WithCancel(task)
		defer cancel()
		stop := context.AfterFunc(parent, cancel)
		defer stop()
		m.mu.Lock()
		m.cameraIP = nil
		m.mu.Unlock()
		probe := m.cameraIPProbe
		if probe == nil {
			probe = cameraip.New().Probe
		}
		result, err := probe(ctx, o.Target, secret, func(message string) { m.progress(0, 0, message) })
		check := CameraIPCheck{}
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil {
			check = CameraIPCheck{Token: sites.ID(), SiteID: o.SiteID, CredentialID: o.CredentialID, Result: result}
			m.mu.Lock()
			m.cameraIP = &pendingCameraIP{Check: check, Expires: time.Now().Add(10 * time.Minute)}
			m.mu.Unlock()
			m.progress(1, 1, "Camera check complete. Review the results before saving")
		}
		done <- answer{check, err}
		return err
	})
	if e != nil {
		secret.Clear()
		return CameraIPCheck{}, e
	}
	a := <-done
	return a.check, a.err
}
func (m *Manager) SaveCameraIP(o CameraIPSave) (sites.AddedCamera, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.cameraIP
	if m.closed || p == nil || o.Token == "" || o.Token != p.Check.Token || o.SiteID != p.Check.SiteID || time.Now().After(p.Expires) {
		return sites.AddedCamera{}, errors.New("camera check expired or changed; check the address again")
	}
	if !p.Check.Result.Identified && !o.AllowUnverified {
		return sites.AddedCamera{}, errors.New("confirm saving an unverified camera address")
	}
	if len(o.Name) > 200 || len(o.Location) > 1000 {
		return sites.AddedCamera{}, errors.New("camera name or location is too long")
	}
	d := p.Check.Result.Device
	r := sites.Record{SiteID: o.SiteID, Device: d, Location: sanitize.Text(o.Location), CredentialMode: "none", Tags: []string{}, ManualFields: []string{"ipv4"}}
	if o.Name != "" {
		r.Device.Name = sanitize.Label(o.Name)
		r.ManualFields = append(r.ManualFields, "name")
	}
	if p.Check.CredentialID != "" {
		r.CredentialMode = "profile"
		r.CredentialID = p.Check.CredentialID
	}
	evidence, _ := json.Marshal(struct {
		At         string              `json:"at"`
		Identified bool                `json:"identified"`
		Evidence   []cameraip.Evidence `json:"evidence"`
	}{p.Check.Result.At, p.Check.Result.Identified, p.Check.Result.Evidence})
	saved, e := m.Store.AddCameraIP(r, string(evidence))
	if e == nil {
		m.cameraIP = nil
	}
	return saved, e
}
