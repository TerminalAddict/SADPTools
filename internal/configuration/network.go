package configuration

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

// A fresh inquiry chooses the camera's network authentication profile and the
// unrelated settings to preserve. It is read-only; Sent refers to the write.
func (e *SADPEngine) updateNetwork(ctx context.Context, conn platform.Transport, adapter platform.Adapter, r Request, result Result) (Result, error) {
	notSent := func(cause error) (Result, error) {
		result.Message = "Network change was not sent: " + cause.Error() + "."
		return result, fmt.Errorf("%s: %w", result.Message, cause)
	}
	id, err := sadp.NewUUID()
	if err != nil {
		return notSent(errors.New("could not prepare the device check"))
	}
	probes, err := sadp.Probes(id)
	if err != nil {
		return notSent(errors.New("could not prepare the device check"))
	}
	if err = ctx.Err(); err != nil {
		return notSent(err)
	}
	e.log.Info("Checking network authentication and settings", "mac", result.MAC, "interface", adapter.Name)
	if err = conn.Send(adapter, probes[0]); err != nil {
		return notSent(safeNetworkIO(ctx, err))
	}
	_, profile, peer, err := e.networkReply(ctx, conn, adapter, id, result.MAC, "inquiry", "", "")
	if err != nil {
		return notSent(err)
	}
	id, err = sadp.NewUUID()
	if err != nil {
		return notSent(errors.New("could not prepare the network request"))
	}
	payload, err := sadp.NetworkCommand(id, result.MAC, r.Password, r.Network, profile)
	if err != nil {
		return notSent(err)
	}
	defer clear(payload)
	if err = ctx.Err(); err != nil {
		return notSent(err)
	}
	e.log.Info("Sending authenticated network change", "mac", result.MAC, "interface", adapter.Name)
	result.Sent = true
	if err = conn.Send(adapter, payload); err != nil {
		return uncertain(result, safeNetworkIO(ctx, err))
	}
	clear(payload)
	// An acknowledgement may originate before or after an address change.
	// A MAC-less reply from an unknown DHCP address cannot identify this camera.
	newPeer := ""
	if !r.Network.DHCP {
		newPeer = r.Network.IPv4
	}
	reply, _, _, err := e.networkReply(ctx, conn, adapter, id, result.MAC, "update", peer, newPeer)
	if err != nil {
		return uncertain(result, err)
	}
	result, err = interpret(r, result, reply)
	e.log.Info("Device operation completed", "action", r.Action, "mac", result.MAC, "outcome", result.Outcome)
	return result, err
}

func safeNetworkIO(ctx context.Context, err error) error {
	if cause := safeActivationIO(ctx, err); errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return errors.New("could not exchange network configuration messages on the selected adapter")
}

func (e *SADPEngine) networkReply(ctx context.Context, conn platform.Transport, adapter platform.Adapter, id, mac, kind, peer, newPeer string) (sadp.CommandReply, sadp.NetworkProfile, string, error) {
	buffer := make([]byte, sadp.MaxPacketSize)
	defer clear(buffer)
	for {
		if err := ctx.Err(); err != nil {
			return sadp.CommandReply{}, sadp.NetworkProfile{}, "", err
		}
		n, info, err := conn.Read(buffer)
		if err != nil {
			return sadp.CommandReply{}, sadp.NetworkProfile{}, "", safeNetworkIO(ctx, err)
		}
		if n < 0 || n > len(buffer) {
			return sadp.CommandReply{}, sadp.NetworkProfile{}, "", errors.New("invalid network datagram size")
		}
		reply, parseErr := sadp.ParseCommandReply(buffer[:n])
		source, sourceErr := netip.ParseAddrPort(info.Source)
		valid := parseErr == nil && info.InterfaceIndex == adapter.Index && sourceErr == nil &&
			source.Addr().Is4() && !source.Addr().IsUnspecified() && !source.Addr().IsLoopback() && !source.Addr().IsMulticast() && source.Addr().String() != "255.255.255.255" &&
			strings.EqualFold(reply.UUID, id) && strings.EqualFold(reply.Types, kind)
		if reply.MAC != "" {
			valid = valid && devices.NormalizeMAC(reply.MAC) == mac
		} else {
			valid = valid && kind != "inquiry" && peer != "" && (source.Addr().String() == peer || (newPeer != "" && source.Addr().String() == newPeer))
		}
		var profile sadp.NetworkProfile
		if valid && kind == "inquiry" {
			device, deviceErr := sadp.Parse(buffer[:n])
			valid = deviceErr == nil && device.Device.MAC == mac && device.Device.IPv4 == source.Addr().String()
			if valid {
				profile, err = sadp.ParseNetworkProfile(buffer[:n])
			}
		}
		clear(buffer[:n])
		if !valid {
			e.log.Debug("Ignored malformed or unrelated network reply", "stage", kind)
			continue
		}
		// An identified camera with an unsupported profile is a preflight failure,
		// not a timeout and never an invitation to try a plaintext fallback.
		return reply, profile, source.Addr().String(), err
	}
}
