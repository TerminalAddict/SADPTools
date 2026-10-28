package configuration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

// One socket, one adapter, fresh UUID per stage. Inquiry and key exchange cannot
// activate a device; Result.Sent becomes true only at the activation write.
func (e *SADPEngine) activate(ctx context.Context, conn platform.Transport, adapter platform.Adapter, r Request, result Result) (Result, error) {
	notSent := func(stage string, cause error) (Result, error) {
		result.Message = "Activation was not sent: " + stage + "."
		e.log.Info("Activation stopped before write", "stage", stage, "mac", result.MAC, "interface", adapter.Name)
		return result, fmt.Errorf("%s: %w", result.Message, cause)
	}
	exchange, err := sadp.NewActivationExchange()
	if err != nil {
		return notSent("could not prepare encryption", err)
	}
	defer exchange.Close()
	inquiryID, err := sadp.NewUUID()
	if err != nil {
		return notSent("could not prepare device check", err)
	}
	probes, err := sadp.Probes(inquiryID)
	if err != nil {
		return notSent("could not prepare device check", err)
	}
	if err = ctx.Err(); err != nil {
		return notSent("device check cancelled or timed out", err)
	}
	e.log.Info("Checking device before activation", "mac", result.MAC, "interface", adapter.Name)
	if err = conn.Send(adapter, probes[0]); err != nil {
		return notSent("could not send the read-only device check", safeActivationIO(ctx, err))
	}
	_, device, peer, err := e.activationReply(ctx, conn, adapter, inquiryID, result.MAC, "inquiry", "")
	if err != nil {
		return notSent("no matching discovery reply on the selected adapter", err)
	}
	if device.Activated == nil {
		return notSent("camera did not report its activation state", errors.New("rediscover and verify the camera"))
	}
	if *device.Activated {
		return notSent("camera already reports active", errors.New("verify the existing password through the camera login"))
	}
	exchangeID, err := sadp.NewUUID()
	if err != nil {
		return notSent("could not prepare key exchange", err)
	}
	probe, err := exchange.Probe(exchangeID, result.MAC)
	if err != nil {
		return notSent("could not prepare key exchange", err)
	}
	if err = ctx.Err(); err != nil {
		return notSent("key exchange cancelled or timed out", err)
	}
	e.log.Info("Negotiating encrypted activation", "mac", result.MAC, "interface", adapter.Name)
	if err = conn.Send(adapter, probe); err != nil {
		return notSent("could not send the read-only key exchange", safeActivationIO(ctx, err))
	}
	reply, _, _, err := e.activationReply(ctx, conn, adapter, exchangeID, result.MAC, string(sadp.ExchangeCode), peer)
	if err != nil {
		return notSent("no matching encryption reply; this firmware may not support the activation method", err)
	}
	// Reuse conservative status handling, but never expose this challenge in a
	// configuration result, GUI snapshot, log or recovery export.
	check, err := interpret(Request{Action: sadp.ExchangeCode}, Result{}, reply)
	if err != nil || check.Outcome != "challenge" || !strings.EqualFold(reply.Result, "success") {
		if !errors.Is(err, ErrRejected) {
			err = errors.New("unsupported key-exchange response")
		}
		return notSent("camera did not accept the encryption exchange", err)
	}
	encrypted, err := exchange.EncryptPassword(reply.Code, r.Password)
	reply.Code, check.Challenge = "", ""
	if err != nil {
		return notSent("unsupported activation encryption response", err)
	}
	activationID, err := sadp.NewUUID()
	if err != nil {
		return notSent("could not prepare activation request", err)
	}
	payload, err := sadp.ActivationCommand(activationID, result.MAC, encrypted)
	if err != nil {
		return notSent("could not prepare activation request", err)
	}
	defer clear(payload)
	if err = ctx.Err(); err != nil {
		return notSent("activation cancelled or timed out before sending", err)
	}
	e.log.Info("Sending encrypted activation", "mac", result.MAC, "interface", adapter.Name)
	result.Sent = true
	if err = conn.Send(adapter, payload); err != nil {
		return uncertain(result, safeActivationIO(ctx, err))
	}
	clear(payload)
	reply, _, _, err = e.activationReply(ctx, conn, adapter, activationID, result.MAC, string(sadp.Activate), peer)
	if err != nil {
		return uncertain(result, err)
	}
	result, err = interpret(r, result, reply)
	e.log.Info("Device operation completed", "action", r.Action, "mac", result.MAC, "outcome", result.Outcome)
	return result, err
}

func safeActivationIO(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var timed net.Error
	if errors.As(err, &timed) && timed.Timeout() {
		return context.DeadlineExceeded
	}
	return errors.New("activation transport failed")
}

// Some genuine activation/exchangecode replies omit MAC. Accept that only after
// a fresh inquiry bound the selected MAC to this source IP and receiving adapter.
// UUID and type must still match exactly; an explicitly conflicting MAC fails.
// The unrelated command/recovery reply matcher retains its stricter requirements.
func (e *SADPEngine) activationReply(ctx context.Context, conn platform.Transport, adapter platform.Adapter, id, mac, kind, peer string) (sadp.CommandReply, devices.Device, string, error) {
	buffer := make([]byte, sadp.MaxPacketSize)
	defer clear(buffer)
	for {
		if err := ctx.Err(); err != nil {
			return sadp.CommandReply{}, devices.Device{}, "", err
		}
		n, info, err := conn.Read(buffer)
		if err != nil {
			return sadp.CommandReply{}, devices.Device{}, "", safeActivationIO(ctx, err)
		}
		if n < 0 || n > len(buffer) {
			return sadp.CommandReply{}, devices.Device{}, "", errors.New("invalid activation datagram size")
		}
		reply, parseErr := sadp.ParseCommandReply(buffer[:n])
		source, sourceErr := netip.ParseAddrPort(info.Source)
		reason := ""
		switch {
		case parseErr != nil:
			reason = "malformed XML"
		case info.InterfaceIndex != adapter.Index:
			reason = "different adapter"
		case sourceErr != nil || !source.Addr().Is4() || source.Addr().IsUnspecified() || source.Addr().IsMulticast() || source.Addr().IsLoopback():
			reason = "invalid source address"
		case peer != "" && source.Addr().String() != peer:
			reason = "different source address"
		case !strings.EqualFold(reply.UUID, id):
			reason = "different request UUID"
		case !strings.EqualFold(reply.Types, kind):
			reason = "different response type"
		case reply.MAC != "" && devices.NormalizeMAC(reply.MAC) != mac:
			reason = "different device MAC"
		}
		var device devices.Device
		if reason == "" && kind == "inquiry" {
			discovery, parseErr := sadp.Parse(buffer[:n])
			device = discovery.Device
			if parseErr != nil || device.MAC != mac || (device.IPv4 != "" && device.IPv4 != source.Addr().String()) {
				reason = "device identity not confirmed"
			}
		}
		clear(buffer[:n])
		if reason != "" {
			e.log.Debug("Ignored activation reply", "stage", kind, "reason", reason)
			continue
		}
		return reply, device, source.Addr().String(), nil
	}
}
