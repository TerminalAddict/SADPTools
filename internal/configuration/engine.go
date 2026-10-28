package configuration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/protocol/sadp"
)

type SADPEngine struct {
	log        *slog.Logger
	interfaces func() ([]platform.Adapter, error)
	open       func() (platform.Transport, error)
	active     atomic.Bool
}

func New(log *slog.Logger) *SADPEngine {
	if log == nil {
		log = slog.Default()
	}
	return &SADPEngine{log: log, interfaces: platform.Interfaces, open: platform.Open}
}

func (e *SADPEngine) Execute(parent context.Context, r Request) (result Result, err error) {
	result = Result{Action: r.Action, MAC: devices.NormalizeMAC(r.MAC), Interface: r.Interface, Outcome: "not_sent"}
	if err = r.Validate(); err != nil {
		return result, err
	}
	if !e.active.CompareAndSwap(false, true) {
		return result, ErrBusy
	}
	defer e.active.Store(false)
	if err = parent.Err(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(r.TimeoutSeconds)*time.Second)
	defer cancel()
	adapters, err := e.interfaces()
	if err != nil {
		return result, err
	}
	adapters, err = platform.Select(adapters, r.Interface)
	if err != nil {
		return result, err
	}
	if len(adapters) != 1 {
		return result, errors.New("select exactly one network interface")
	}
	adapter := adapters[0]
	conn, err := e.open()
	if err != nil {
		return result, err
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = conn.Close(); close(closed) })
	defer func() {
		if stop() {
			_ = conn.Close()
		} else {
			<-closed
		}
	}()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return result, fmt.Errorf("set command deadline: %w", err)
	}
	if err = conn.Join(adapter); err != nil {
		return result, fmt.Errorf("join selected interface: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if r.Action == sadp.Activate {
		return e.activate(ctx, conn, adapter, r, result)
	}
	id, err := sadp.NewUUID()
	if err != nil {
		return result, err
	}
	payload, err := sadp.Command(id, result.MAC, r.Action, r.Password, r.Code, r.Network)
	if err != nil {
		return result, err
	}
	defer clear(payload)
	e.log.Info("Sending device operation", "action", r.Action, "mac", result.MAC, "interface", r.Interface)
	// Mark the attempt before send. Even an I/O error cannot promise no delivery.
	result.Sent = true
	if err = conn.Send(adapter, payload); err != nil {
		if ctx.Err() != nil {
			return uncertain(result, ctx.Err())
		}
		return uncertain(result, errors.New("send failed"))
	}
	clear(payload)
	// One transmission only. Writes are never retried across adapters or on timeout.
	buffer := make([]byte, sadp.MaxPacketSize)
	defer clear(buffer)
	for {
		if ctx.Err() != nil {
			return uncertain(result, ctx.Err())
		}
		n, info, readErr := conn.Read(buffer)
		if readErr != nil {
			if ctx.Err() != nil {
				return uncertain(result, ctx.Err())
			}
			var timed net.Error
			if errors.As(readErr, &timed) && timed.Timeout() {
				return uncertain(result, context.DeadlineExceeded)
			}
			return uncertain(result, errors.New("could not receive device reply"))
		}
		if n < 0 || n > len(buffer) {
			return uncertain(result, errors.New("invalid datagram size"))
		}
		reply, parseErr := sadp.ParseCommandReply(buffer[:n])
		clear(buffer[:n])
		if parseErr != nil || info.InterfaceIndex != adapter.Index || devices.NormalizeMAC(reply.MAC) != result.MAC || !strings.EqualFold(reply.UUID, id) || !strings.EqualFold(reply.Types, string(r.Action)) {
			e.log.Debug("Ignored malformed or unrelated command reply")
			continue
		}
		result, err = interpret(r, result, reply)
		e.log.Info("Device operation completed", "action", r.Action, "mac", result.MAC, "outcome", result.Outcome)
		return result, err
	}
}

func interpret(request Request, result Result, reply sadp.CommandReply) (Result, error) {
	status := strings.ToLower(reply.Result)
	if request.Action == sadp.Activate {
		switch status {
		case "riskpassword":
			result.Outcome = "rejected"
			result.Message = "The camera rejected the password as too weak. Choose a stronger password of 8–16 bytes."
			return result, ErrRejected
		case "hasactivated":
			result.Outcome = "rejected"
			result.Message = "The camera reports it is already active. Rediscover and verify its existing password before another operation."
			return result, ErrRejected
		}
	}
	code, codeErr := strconv.ParseUint(reply.ErrorCode, 10, 16)
	if codeErr == nil && reply.ErrorCode != request.Password && reply.ErrorCode != request.Code {
		result.ErrorCode = strconv.FormatUint(code, 10)
	}
	if status == "failure" || status == "failed" || status == "denied" || (codeErr == nil && code != 0) {
		result.Outcome = "rejected"
		result.Message = "The device rejected the request. Check credentials, recovery code and firmware support before retrying."
		return result, ErrRejected
	}
	if request.Action.ReadOnly() {
		challenge := reply.Code
		if challenge == "" {
			challenge = reply.EncryptString
		}
		if validText(challenge, 4096) && (status == "" || status == "success") && (reply.ErrorCode == "" || (codeErr == nil && code == 0)) {
			result.Outcome = "challenge"
			result.Challenge = challenge
			result.Message = "Recovery data received. Use the method supported by your authorised support provider."
			return result, nil
		}
	} else if status == "success" && (reply.ErrorCode == "" || (codeErr == nil && code == 0)) {
		result.Outcome = "acknowledged"
		result.Message = "The device acknowledged the request. Rediscover to verify its current settings."
		return result, nil
	}
	return uncertain(result, errors.New("unrecognised response status or recovery format"))
}
