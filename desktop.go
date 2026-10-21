//go:build desktop

package main

import (
	"fmt"
	"os"

	"github.com/TerminalAddict/SADPTools/internal/app"
	"github.com/TerminalAddict/SADPTools/internal/platform"
	"github.com/TerminalAddict/SADPTools/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// DesktopService is the only application API exposed to the webview.
type DesktopService struct {
	controller *app.Controller
	app        *application.App
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
func (s *DesktopService) ServiceShutdown() error { s.controller.Close(); return nil }
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
