//go:build desktop

package main

import (
	"fmt"
	"os"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/configuration"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/update"
	"github.com/TerminalAddict/SADPTools/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DesktopService is the only application API exposed to the webview.
type DesktopService struct {
	controller *app.Controller
	app        *application.App
	updates    *update.Manager
}

func (s *DesktopService) Snapshot() app.Snapshot                  { return s.controller.Snapshot() }
func (s *DesktopService) Interfaces() ([]platform.Adapter, error) { return s.controller.Interfaces() }
func (s *DesktopService) GetSettings() app.Settings               { return s.controller.Settings() }
func (s *DesktopService) SaveSettings(settings app.Settings) error {
	return s.controller.SaveSettings(settings)
}
func (s *DesktopService) StartScan() error       { return s.controller.StartScan() }
func (s *DesktopService) CancelScan()            { s.controller.CancelScan() }
func (s *DesktopService) Version() string        { return version.Value }
func (s *DesktopService) ServiceShutdown() error { s.updates.Close(); s.controller.Close(); return nil }

func (s *DesktopService) UpdateStatus() update.State { return s.updates.Snapshot() }
func (s *DesktopService) CheckForUpdates() error {
	return s.updates.Check(s.controller.Settings().UpdateChannel)
}
func (s *DesktopService) DownloadUpdate() error {
	return s.updates.Download(s.controller.Settings().UpdateChannel)
}
func (s *DesktopService) CancelUpdate() { s.updates.Cancel() }
func (s *DesktopService) InstallUpdate() error {
	if err := s.controller.LaunchUpdate(func(settings app.Settings) error { return s.updates.Install(settings.UpdateChannel) }); err != nil {
		return err
	}
	s.app.Quit()
	return nil
}
func (s *DesktopService) OpenDevice(id string) error {
	url, err := s.controller.DeviceURL(id)
	if err != nil {
		return err
	}
	return s.app.Browser.OpenURL(url)
}
func (s *DesktopService) ExportDevices(ids []string, format string) (string, error) {
	content, err := s.controller.Export(ids, format)
	if err != nil {
		return "", err
	}
	path, err := s.app.Dialog.SaveFile().SetFilename("sadptools-devices."+format).AddFilter("Device inventory", "*."+format).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err = os.WriteFile(path, content, 0600); err != nil {
		return "", fmt.Errorf("export inventory: %w", err)
	}
	return path, nil
}

func (s *DesktopService) ConfigureDevice(id string, request configuration.Request) (configuration.Result, error) {
	return s.controller.Configure(id, request)
}
func (s *DesktopService) CancelOperation() { s.controller.CancelOperation() }

func (s *DesktopService) SaveRecoveryData(id string, result configuration.Result) (string, error) {
	data, err := s.controller.RecoveryExport(id, result)
	if err != nil {
		return "", err
	}
	path, err := s.app.Dialog.SaveFile().SetFilename("sadptools-recovery.json").AddFilter("Private recovery data", "*.json").PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("save recovery data: %w", err)
	}
	return path, nil
}
