package toolkit

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/onvif"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

func (m *Manager) PreviewControls(ctx context.Context, o PreviewOptions, secret, other vault.Secret) (*onvif.Control, error) {
	d, e := m.previewDevice(o.SiteID, o.DeviceID)
	if e != nil {
		return nil, e
	}
	if o.Method != "onvif" && o.Method != "onvif-snapshot" {
		return nil, errors.New("select ONVIF video profiles before loading camera controls")
	}
	if o.CredentialSite != "" {
		saved, e := m.previewDevice(o.CredentialSite, o.CredentialRecord)
		if e != nil || !(o.SiteID == o.CredentialSite && o.DeviceID == o.CredentialRecord) && !samePreviewDevice(d, saved) {
			return nil, errors.New("credential identity mismatch")
		}
		var source string
		secret, source, e = m.Store.DeviceSecret(o.CredentialSite, o.CredentialRecord)
		if e != nil {
			return nil, e
		}
		if source == "none" {
			return nil, errors.New("credential assignment no longer exists")
		}
	}
	if o.SeparateONVIF {
		secret = other
	}
	defer secret.Clear()
	c, e := m.onvifClient(d, o.ONVIFPin)
	if e != nil {
		return nil, e
	}
	if e = c.SetClockOffset(o.ClockOffset); e != nil {
		return nil, e
	}
	catalog, e := c.MediaProfiles(ctx, secret, o.ClockOffset)
	if e != nil {
		return nil, e
	}
	id := o.Profile
	if id == "main" || id == "sub" || id == "auto" || id == "" {
		id = choosePreviewProfile(catalog.Profiles, id, false)
	}
	return c.Controls(ctx, catalog, id, d.Serial, secret)
}
