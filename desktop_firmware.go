//go:build desktop

package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/diagnostics"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"os"
	"time"
)

func (s *ToolkitService) verifyFirmwareFile(doc sites.Document) (string, error) {
	var f diagnostics.Firmware
	if json.Unmarshal(doc.Data, &f) != nil {
		return "", errors.New("invalid firmware library entry")
	}
	path, e := s.app.Dialog.OpenFile().SetTitle("Select firmware again to compare SHA-256 with the imported file").PromptForSingleSelection()
	if e != nil || path == "" {
		return "", e
	}
	file, e := os.Open(path)
	if e != nil {
		return "", errors.New("file unavailable")
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return "", errors.New("choose a regular firmware file")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f.ExpectedSHA256 = f.SHA256
	f.Bytes = 0
	_, e = diagnostics.HashFirmware(ctx, file, f)
	if e != nil {
		return "", e
	}
	return "SHA-256 matches the imported file. This verifies unchanged bytes, not vendor authenticity or device compatibility.", nil
}
