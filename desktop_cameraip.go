//go:build desktop

package main

import (
	"context"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/TerminalAddict/SADPTools/internal/vault"
)

func (s *ToolkitService) CheckCameraIP(ctx context.Context, o toolkit.CameraIPRequest, username, password string) (toolkit.CameraIPCheck, error) {
	if e := s.ready(); e != nil {
		return toolkit.CameraIPCheck{}, e
	}
	secret := vault.Secret{Username: username, Password: password}
	defer secret.Clear()
	return s.manager.CheckCameraIP(ctx, o, secret)
}
func (s *ToolkitService) SaveCameraIP(o toolkit.CameraIPSave) (sites.AddedCamera, error) {
	if e := s.ready(); e != nil {
		return sites.AddedCamera{}, e
	}
	return s.manager.SaveCameraIP(o)
}
