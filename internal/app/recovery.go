package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/devices"
	"github.com/TerminalAddict/SADPTools/internal/recovery"
	"github.com/TerminalAddict/SADPTools/internal/webinterface"
)

type questionCheck struct {
	deviceID, token string
	target          recovery.Target
	original        recovery.Target
	questions       []recovery.Question
	expires         time.Time
}

func (c *Controller) beginQuestionOperation(id, adapter string) (context.Context, recovery.Target, func(), error) {
	c.mu.Lock()
	fail := func(err error) (context.Context, recovery.Target, func(), error) {
		c.mu.Unlock()
		return nil, recovery.Target{}, nil, err
	}
	if c.closed {
		return fail(errors.New("application is closing"))
	}
	if c.scanning || c.configuring {
		return fail(configuration.ErrBusy)
	}
	var device *devices.Device
	for _, d := range c.inventory.Snapshot() {
		if d.ID == id {
			device = &d
			break
		}
	}
	if device == nil || device.MAC == "" || !device.SADPCompatible() {
		return fail(errors.New("select a discovered camera with a valid MAC address"))
	}
	if device.Activated != nil && !*device.Activated {
		return fail(errors.New("activate this camera before using password recovery"))
	}
	adapters, err := c.recoveryInterfaces()
	if err != nil {
		return fail(errors.New("could not enumerate local adapters"))
	}
	target, err := recovery.ForDevice(*device, adapter, c.settings.Interface, adapters)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(c.ctx, time.Duration(c.settings.TimeoutSeconds)*time.Second)
	c.configuring = true
	if c.webCancel != nil {
		c.webCancel()
	}
	c.operationCancel = cancel
	c.wg.Add(1)
	c.mu.Unlock()
	c.changed()
	finish := func() {
		cancel()
		c.mu.Lock()
		c.configuring = false
		c.operationCancel = nil
		c.nextScan = time.Now().Add(time.Duration(c.settings.IntervalSeconds) * time.Second)
		c.mu.Unlock()
		c.changed()
		c.wg.Done()
	}
	return ctx, target, finish, nil
}

func (c *Controller) CheckSecurityQuestions(id, adapter, certificateSHA256 string) (recovery.Status, error) {
	ctx, target, finish, err := c.beginQuestionOperation(id, adapter)
	if err != nil {
		return recovery.Status{}, err
	}
	defer finish()
	c.mu.Lock()
	c.recoveryCheck = nil
	c.mu.Unlock()
	original := target
	target.CertificateSHA256 = certificateSHA256
	c.log.Info("Checking security-question recovery", "interface", target.Interface, "source", target.Source, "address", target.Address, "port", target.Port)
	status, err := c.questionRecovery.Check(ctx, target)
	if err == nil && ctx.Err() == nil && status.Endpoint.ValidFor(target.Address) {
		c.mu.Lock()
		if webTarget, e := c.webTarget(id); e == nil && webTarget.Address == target.Address {
			c.rememberWeb(id, webTarget, webinterface.Result{Endpoint: status.Endpoint, Message: "Detected " + status.Endpoint.Scheme + " web interface."})
		}
		c.mu.Unlock()
	}
	if err == nil && status.State == "ready" && ctx.Err() == nil {
		if !status.Endpoint.ValidFor(target.Address) {
			return recovery.Status{}, errors.New("recovery endpoint could not be verified")
		}
		target.Scheme, target.Port = status.Endpoint.Scheme, status.Endpoint.Port
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return recovery.Status{}, errors.New("could not create recovery check identity")
		}
		status.CheckID = hex.EncodeToString(nonce[:])
		c.mu.Lock()
		c.recoveryCheck = &questionCheck{deviceID: id, token: status.CheckID, target: target, original: original, questions: status.Questions, expires: time.Now().Add(5 * time.Minute)}
		c.mu.Unlock()
	}
	c.log.Info("Security-question availability checked", "state", status.State)
	return status, err
}

func (c *Controller) ResetWithSecurityQuestions(id, adapter string, request recovery.Request) (recovery.Result, error) {
	if err := request.Validate(); err != nil {
		return recovery.Result{}, err
	}
	ctx, target, finish, err := c.beginQuestionOperation(id, adapter)
	if err != nil {
		return recovery.Result{}, err
	}
	defer finish()
	c.mu.Lock()
	checked := c.recoveryCheck
	// A check authorises at most one explicit submission, including failed or
	// uncertain outcomes. It contains no answers, password or crypto challenge.
	c.recoveryCheck = nil
	c.mu.Unlock()
	if checked == nil || checked.deviceID != id || request.CheckID == "" || checked.token != request.CheckID || checked.original != target || time.Now().After(checked.expires) || !recovery.SameQuestions(checked.questions, request.Answers) {
		return recovery.Result{}, errors.New("camera, connection or questions changed, or the check expired; check security questions again")
	}
	result, err := c.questionRecovery.Reset(ctx, checked.target, request)
	c.log.Info("Security-question recovery finished", "outcome", result.Outcome)
	// Preserve uncertain/rejected outcomes across Wails' error/result boundary.
	if result.Sent {
		return result, nil
	}
	return result, err
}
