// Package configuration declares future device management contracts.
// No writer is registered or implemented in milestone one.
package configuration

import (
	"context"
	"errors"
)

var ErrUnsupported = errors.New("device configuration is not implemented")

type Capabilities struct{ Activate, Network, Password bool }
type NetworkSettings struct {
	IPv4, SubnetMask, Gateway string
	DHCP                      bool
}
type Credentials struct{ Username, Password string }

type Engine interface {
	Capabilities(deviceID string) Capabilities
	Activate(context.Context, string, string) error
	ConfigureNetwork(context.Context, string, Credentials, NetworkSettings) error
	ChangePassword(context.Context, string, Credentials, string) error
}
