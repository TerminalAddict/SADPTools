package isapi

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
)

// DeviceInformation uses the documented GET /ISAPI/System/deviceInfo endpoint.
// API compatibility alone is not proof of manufacturer or activation status.
func (c *Client) DeviceInformation(ctx context.Context, s vault.Secret) (devices.Device, error) {
	b, e := c.Read(ctx, "/ISAPI/System/deviceInfo", s)
	if e != nil {
		return devices.Device{}, e
	}
	defer clear(b)
	return parseDeviceInformation(b)
}
func parseDeviceInformation(b []byte) (devices.Device, error) {
	n, e := xmlsafe.Parse(b)
	if e != nil {
		return devices.Device{}, e
	}
	if n.Name != "DeviceInfo" {
		return devices.Device{}, errors.New("ISAPI device information missing")
	}
	d := devices.Device{Vendor: sanitize.Label(n.Value("manufacturer")), Model: sanitize.Label(n.Value("model")), Name: sanitize.Label(n.Value("deviceName")), Serial: sanitize.Label(n.Value("serialNumber")), Firmware: sanitize.Label(n.Value("firmwareVersion")), MAC: devices.NormalizeMAC(n.Value("macAddress")), DeviceType: sanitize.Label(n.Value("deviceType"))}
	if d.Model == "" && d.Serial == "" && d.MAC == "" {
		return d, errors.New("ISAPI device information contains no identity")
	}
	return d, nil
}
