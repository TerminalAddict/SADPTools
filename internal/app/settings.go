package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TerminalAddict/SADPTools/internal/multidiscovery"
	"github.com/TerminalAddict/SADPTools/internal/update"
	"github.com/TerminalAddict/SADPTools/internal/version"
)

type Settings struct {
	Protocols       []string `json:"protocols"`
	Theme           string   `json:"theme"`
	Interface       string   `json:"interface"`
	TimeoutSeconds  int      `json:"timeoutSeconds"`
	IntervalSeconds int      `json:"intervalSeconds"`
	Automatic       bool     `json:"automatic"`
	CheckUpdates    bool     `json:"checkUpdates"`
	UpdateChannel   string   `json:"updateChannel"`
}

func DefaultSettings() Settings {
	return Settings{Protocols: []string{"sadp"}, Theme: "dark", TimeoutSeconds: 5, IntervalSeconds: 30,
		CheckUpdates: true, UpdateChannel: update.DefaultChannel(version.Value)}
}
func (s Settings) Validate() error {
	if e := multidiscovery.Validate(s.Protocols); e != nil {
		return e
	}
	if s.UpdateChannel != "stable" && s.UpdateChannel != "beta" {
		return fmt.Errorf("update channel must be stable or beta")
	}
	if s.Theme != "dark" && s.Theme != "light" {
		return fmt.Errorf("theme must be dark or light")
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 300 {
		return fmt.Errorf("scan timeout must be 1–300 seconds")
	}
	if s.IntervalSeconds < 5 || s.IntervalSeconds > 86400 {
		return fmt.Errorf("automatic interval must be 5–86400 seconds")
	}
	return nil
}
func SettingsPath() (string, error) {
	p, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "SADPTools", "settings.json"), nil
}
func loadSettings(path string) (Settings, error) {
	s := DefaultSettings()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return DefaultSettings(), err
	}
	if err = s.Validate(); err != nil {
		return DefaultSettings(), err
	}
	return s, nil
}
func saveSettings(path string, s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
