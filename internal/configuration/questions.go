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

func (e *SADPEngine) securityQuestions(ctx context.Context, conn platform.Transport, adapter platform.Adapter, r Request, result Result) (Result, error) {
	notSent := func(err error) (Result, error) {
		result.Message = "Security-question setup was not sent: " + err.Error() + "."
		return result, err
	}
	id, err := sadp.NewUUID()
	if err != nil {
		return notSent(errors.New("could not prepare camera check"))
	}
	probes, err := sadp.Probes(id)
	if err != nil {
		return notSent(err)
	}
	if err = ctx.Err(); err != nil {
		return notSent(err)
	}
	if err = conn.Send(adapter, probes[0]); err != nil {
		return notSent(safeNetworkIO(ctx, err))
	}
	reply, device, peer, err := e.activationReply(ctx, conn, adapter, id, result.MAC, "inquiry", "")
	if err != nil {
		return notSent(err)
	}
	device.Interfaces = []devices.Interface{{Index: adapter.Index, Name: adapter.Name, Address: adapter.Address()}}
	result.ObservedDevice = &device // A real fresh inquiry, never an optimistic change.
	if device.Activated == nil || !*device.Activated {
		result.Outcome = "inactive"
		result.Message = "The camera has not confirmed active status. If you just activated it, wait and check again; otherwise rediscover before activating it."
		return result, nil
	}
	if reply.QuestionSupport == nil || !*reply.QuestionSupport {
		result.Outcome = "unsupported"
		result.Message = "The camera did not advertise SADP security-question setup support. Use its web interface if available."
		return result, nil
	}
	questions, err := e.readSecurityQuestions(ctx, conn, adapter, result.MAC, peer)
	if err != nil {
		return notSent(err)
	}
	identity := result.MAC + "\x00" + device.Serial + "\x00" + device.Model + "\x00" + device.Firmware + "\x00" + adapter.Name
	result.Questions = questions
	result.QuestionRevision = sadp.QuestionRevision(identity, questions)
	configured := false
	for _, q := range questions {
		configured = configured || q.Configured
	}
	if r.Action == sadp.GetSecurityQuestions {
		result.Outcome = "questions_ready"
		result.Message = "No security questions are configured. Select three questions and enter their answers."
		if configured {
			result.Outcome = "questions_configured"
			result.Message = "The camera already has three security questions. Replacing them requires explicit confirmation and its current admin password."
		}
		return result, nil
	}
	if result.QuestionRevision != r.QuestionRevision {
		return notSent(errors.New("camera identity or question settings changed; check again before saving"))
	}
	if configured && !r.ReplaceQuestions {
		return notSent(errors.New("confirm replacement of the camera's existing security questions"))
	}
	exchange, err := sadp.NewActivationExchange()
	if err != nil {
		return notSent(err)
	}
	defer exchange.Close()
	id, err = sadp.NewUUID()
	if err != nil {
		return notSent(errors.New("could not prepare key exchange"))
	}
	probe, err := exchange.Probe(id, result.MAC)
	if err != nil {
		return notSent(err)
	}
	if err = ctx.Err(); err != nil {
		return notSent(err)
	}
	if err = conn.Send(adapter, probe); err != nil {
		return notSent(safeNetworkIO(ctx, err))
	}
	reply, _, _, err = e.activationReply(ctx, conn, adapter, id, result.MAC, string(sadp.ExchangeCode), peer)
	if err != nil {
		return notSent(err)
	}
	if !strings.EqualFold(reply.Result, "success") || reply.Code == "" || (reply.ErrorCode != "" && reply.ErrorCode != "0") {
		return notSent(errors.New("camera refused security-question encryption negotiation"))
	}
	id, err = sadp.NewUUID()
	if err != nil {
		return notSent(errors.New("could not prepare question setup"))
	}
	payload, err := exchange.SecurityQuestionsCommand(id, result.MAC, reply.Code, r.Password, r.Answers)
	reply.Code = ""
	if err != nil {
		return notSent(err)
	}
	defer clear(payload)
	if err = ctx.Err(); err != nil {
		return notSent(err)
	}
	e.log.Info("Setting encrypted security questions", "mac", result.MAC, "interface", adapter.Name)
	result.Sent = true
	if err = conn.Send(adapter, payload); err != nil {
		return uncertainQuestions(result, safeNetworkIO(ctx, err))
	}
	clear(payload)
	reply, _, _, err = e.activationReply(ctx, conn, adapter, id, result.MAC, string(sadp.SetSecurityQuestions), peer)
	if err != nil {
		return uncertainQuestions(result, err)
	}
	if message, ok := map[string]string{
		"passworderror":  "The camera rejected the admin password. Verify it before retrying; repeated failures can lock the camera.",
		"locked":         "The camera reports a lockout. Wait for it to expire before trying again.",
		"longanswer":     "The camera rejected an answer as too long. Its firmware has a stricter answer limit.",
		"questionnumerr": "The camera rejected the selected question IDs or question count.",
		"notactivated":   "The camera reports it is inactive; rediscover before continuing.",
		"failed":         "The camera rejected security-question setup. Check the password, answers and firmware support before retrying.",
	}[strings.ToLower(reply.Result)]; ok {
		result.Outcome = "rejected"
		result.Message = message
		return result, ErrRejected
	}
	result, err = interpret(r, result, reply)
	if err != nil {
		if errors.Is(err, ErrUnconfirmed) {
			return uncertainQuestions(result, err)
		}
		return result, err
	}
	// Read back only IDs and configured flags. Never test the answers by resetting
	// a password, and never retry the setup write after a missing acknowledgement.
	after, err := e.readSecurityQuestions(ctx, conn, adapter, result.MAC, peer)
	if err != nil {
		return uncertainQuestions(result, errors.New("setup acknowledged but question readback failed"))
	}
	selected := map[int]bool{}
	for _, a := range r.Answers {
		selected[a.ID] = true
	}
	for _, q := range after {
		if q.Configured != selected[q.ID] {
			return uncertainQuestions(result, errors.New("setup acknowledged but selected question IDs were not verified"))
		}
	}
	result.Questions = after
	result.QuestionRevision = sadp.QuestionRevision(identity, after)
	result.Outcome = "verified"
	result.Message = "Security questions saved. The camera acknowledged setup and reports the three selected questions configured. The answers were not tested by resetting its password."
	return result, nil
}

func uncertainQuestions(result Result, cause error) (Result, error) {
	result.Outcome = "unconfirmed"
	result.Message = "Security-question setup is unconfirmed. The questions may have been saved; check their status before trying again. This command does not request activation or a password change."
	return result, fmt.Errorf("%w: %w", ErrUnconfirmed, cause)
}

func (e *SADPEngine) readSecurityQuestions(ctx context.Context, conn platform.Transport, adapter platform.Adapter, mac, peer string) ([]sadp.SecurityQuestion, error) {
	id, err := sadp.NewUUID()
	if err != nil {
		return nil, errors.New("could not prepare security-question check")
	}
	payload, err := sadp.Command(id, strings.ToLower(strings.ReplaceAll(mac, ":", "-")), sadp.GetSecurityQuestions, "", "", sadp.NetworkSettings{})
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = conn.Send(adapter, payload); err != nil {
		return nil, safeNetworkIO(ctx, err)
	}
	b := make([]byte, sadp.MaxPacketSize)
	defer clear(b)
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		n, info, err := conn.Read(b)
		if err != nil {
			return nil, safeNetworkIO(ctx, err)
		}
		if n < 0 || n > len(b) {
			return nil, errors.New("invalid security-question datagram size")
		}
		reply, questions, parseErr := sadp.ParseSecurityQuestions(b[:n])
		clear(b[:n])
		source, sourceErr := netip.ParseAddrPort(info.Source)
		if sourceErr != nil || source.Addr().String() != peer || info.InterfaceIndex != adapter.Index || !strings.EqualFold(reply.UUID, id) || !strings.EqualFold(reply.Types, string(sadp.GetSecurityQuestions)) || (reply.MAC != "" && devices.NormalizeMAC(reply.MAC) != mac) {
			continue
		}
		if parseErr != nil {
			return nil, parseErr
		}
		if !strings.EqualFold(reply.Result, "success") || (reply.ErrorCode != "" && reply.ErrorCode != "0") {
			return nil, errors.New("camera refused the security-question check; use its web interface")
		}
		return questions, nil
	}
}
