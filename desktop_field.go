//go:build desktop

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"os"
	"path/filepath"
	"time"
)

func (s *ToolkitService) FieldCatalogue() []diagnostics.Capability {
	return append([]diagnostics.Capability{}, diagnostics.Catalogue...)
}
func (s *ToolkitService) StartField(o toolkit.FieldOptions) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartField(o)
}
func (s *ToolkitService) Field() (toolkit.FieldEvidence, error) {
	if e := s.ready(); e != nil {
		return toolkit.FieldEvidence{}, e
	}
	return s.manager.Field(), nil
}
func (s *ToolkitService) ApplyImage(o toolkit.ImageChange) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.ApplyImage(o)
}
func (s *ToolkitService) PreviewLabel(o toolkit.LabelOptions) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	data, e := s.manager.Label(o)
	if e != nil {
		return "", e
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data), nil
}
func (s *ToolkitService) ExportLabel(o toolkit.LabelOptions) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	b, e := s.manager.Label(o)
	if e != nil {
		return "", e
	}
	path, e := s.saveFile("SADPTools-labels.svg", b)
	if e == nil && path != "" {
		_, e = s.manager.SaveLabel(o)
	}
	return path, e
}
func (s *ToolkitService) LocateAsset(code string) (toolkit.LabelAsset, error) {
	if e := s.ready(); e != nil {
		return toolkit.LabelAsset{}, e
	}
	return s.manager.LocateAsset(code)
}
func (s *ToolkitService) PreviewSupport(o toolkit.BundleOptions) (diagnostics.Bundle, error) {
	if e := s.ready(); e != nil {
		return diagnostics.Bundle{}, e
	}
	return s.manager.PrepareSupport(o)
}
func (s *ToolkitService) ExportSupport(digest, passphrase string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	b, e := s.manager.ExportSupport(digest, passphrase)
	if e != nil {
		return "", e
	}
	defer clear(b)
	name := "SADPTools-support.zip"
	if passphrase != "" {
		name += ".sadpencrypted"
	}
	return s.saveFile(name, b)
}
func (s *ToolkitService) ImportFirmware(site string, f diagnostics.Firmware) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	if e := f.Validate(); e != nil {
		return sites.Document{}, e
	}
	path, e := s.app.Dialog.OpenFile().SetTitle("Choose firmware for read-only hashing and compatibility planning").PromptForSingleSelection()
	if e != nil || path == "" {
		return sites.Document{}, e
	}
	file, e := os.Open(path)
	if e != nil {
		return sites.Document{}, errors.New("firmware cannot be opened")
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return sites.Document{}, errors.New("choose a regular firmware file")
	}
	f.Filename = filepath.Base(path)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f, e = diagnostics.HashFirmware(ctx, file, f)
	if e != nil {
		return sites.Document{}, e
	}
	return s.manager.SaveFirmware(site, f)
}
func (s *ToolkitService) VerifyFirmware(site, id string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	doc, e := s.manager.Store.Document(site, id)
	if e != nil || doc.Kind != "firmware" {
		return "", errors.New("firmware library item not found")
	}
	return s.verifyFirmwareFile(doc)
}
func (s *ToolkitService) MatchFirmware(site, id, device string) (diagnostics.FirmwareMatch, error) {
	if e := s.ready(); e != nil {
		return diagnostics.FirmwareMatch{}, e
	}
	return s.manager.MatchFirmware(site, id, device)
}
func (s *ToolkitService) CreateReplacement(site, oldID, newID, notes string) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	return s.manager.Store.CreateReplacement(site, oldID, newID, notes)
}
func (s *ToolkitService) VerifyReplacement(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.VerifyReplacement(site, id)
}
func (s *ToolkitService) CompleteReplacement(site, id, updated, confirmation, notes string) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	return s.manager.Store.CompleteReplacement(site, id, updated, confirmation, notes)
}
func (s *ToolkitService) StartANPR(o toolkit.DeviceRead, seconds int, retain bool) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartANPR(o, seconds, retain)
}
func (s *ToolkitService) ANPR() (toolkit.ANPRSession, error) {
	if e := s.ready(); e != nil {
		return toolkit.ANPRSession{}, e
	}
	return s.manager.ANPR(), nil
}
func (s *ToolkitService) ClearANPR() {
	if s.manager != nil {
		s.manager.ClearANPR()
	}
}
func (s *ToolkitService) SaveANPR(id string, g toolkit.ANPRGrade) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	return s.manager.SaveANPR(id, g)
}

func (s *ToolkitService) TraceSupport() diagnostics.TraceCapability {
	return diagnostics.TraceSupport()
}
func (s *ToolkitService) CaptureTrace(o diagnostics.TraceOptions) error {
	if e := s.ready(); e != nil {
		return e
	}
	if !o.Approved {
		return errors.New("approve the raw capture warning first")
	}
	path, e := s.app.Dialog.SaveFile().SetFilename("SADPTools-sensitive-trace.pcapng").AddFilter("Sensitive raw capture", "*.pcapng").PromptForSingleSelection()
	if e != nil || path == "" {
		return e
	}
	return s.manager.StartTrace(o, path)
}
func (s *ToolkitService) DecryptSupport(passphrase string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	_, data, e := s.importFile("Encrypted support bundle", "*.sadpencrypted", 32<<20)
	if e != nil || data == nil {
		return "", e
	}
	defer clear(data)
	plain, e := vault.DecryptBackup(data, passphrase)
	if e != nil {
		return "", e
	}
	defer clear(plain)
	if len(plain) < 4 || string(plain[:4]) != "PK\x03\x04" {
		return "", errors.New("encrypted file is not a ZIP support bundle")
	}
	return s.saveFile("SADPTools-support-decrypted.zip", plain)
}

func (s *ToolkitService) RemoteTroubleshoot(o toolkit.DeviceRead, targetID string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.RemoteTroubleshoot(o, targetID)
}

func (s *ToolkitService) LabelAssets(site string) ([]toolkit.LabelAsset, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	return s.manager.LabelAssets(site)
}
