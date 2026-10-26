//go:build desktop

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/configbackup"
	"github.com/TerminalAddict/SADPTools/internal/nvr"
	"github.com/TerminalAddict/SADPTools/internal/reports"
	"github.com/TerminalAddict/SADPTools/internal/retention"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/toolkit"
	"io"
	"os"
	"path/filepath"
)

func (s *ToolkitService) Advanced(site string) (toolkit.AdvancedView, error) {
	if e := s.ready(); e != nil {
		return toolkit.AdvancedView{}, e
	}
	return s.manager.Advanced(site)
}
func (s *ToolkitService) SaveAdvanced(d sites.Document) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return d, e
	}
	return s.manager.SaveAdvanced(d)
}
func (s *ToolkitService) DeleteAdvanced(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.DeleteDocument(site, id)
}
func (s *ToolkitService) StartNVR(o toolkit.DeviceRead) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartNVR(o)
}
func (s *ToolkitService) StartWalk(o toolkit.DeviceRead, seconds int) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartWalk(o, seconds)
}
func (s *ToolkitService) Walk() (toolkit.WalkTest, error) {
	if e := s.ready(); e != nil {
		return toolkit.WalkTest{}, e
	}
	return s.manager.Walk(), nil
}
func (s *ToolkitService) SaveWalk(id, result, notes string, evidence []string, assessments []toolkit.EventAssessment) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	return s.manager.SaveWalk(id, result, notes, evidence, assessments)
}
func (s *ToolkitService) StartConfiguration(o toolkit.DeviceRead) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartConfiguration(o)
}
func (s *ToolkitService) CompareConfiguration(site, left, right string) ([]configbackup.Change, error) {
	if e := s.ready(); e != nil {
		return nil, e
	}
	a, e := s.manager.ConfigBackup(site, left)
	if e != nil {
		return nil, e
	}
	b, e := s.manager.ConfigBackup(site, right)
	if e != nil {
		return nil, e
	}
	return configbackup.Diff(a, b), nil
}
func (s *ToolkitService) ConfigurationPlan(site, backup, device string) (configbackup.Preview, error) {
	if e := s.ready(); e != nil {
		return configbackup.Preview{}, e
	}
	return s.manager.ConfigurationPlan(site, backup, device)
}
func (s *ToolkitService) ExportConfiguration(site, id, passphrase string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	b, e := s.manager.ConfigBackup(site, id)
	if e != nil {
		return "", e
	}
	data, e := configbackup.Export(b, passphrase)
	if e != nil {
		return "", e
	}
	return s.saveFile("SADPTools-configuration.sadpconfig", data)
}
func (s *ToolkitService) importFile(title, pattern string, max int64) (string, []byte, error) {
	path, e := s.app.Dialog.OpenFile().SetTitle(title).AddFilter(title, pattern).PromptForSingleSelection()
	if e != nil || path == "" {
		return "", nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return "", nil, errors.New("selected file could not be opened")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) > max {
		return "", nil, errors.New("file is unreadable or exceeds the size limit")
	}
	return filepath.Base(path), b, nil
}
func (s *ToolkitService) ImportConfiguration(site, device, passphrase string) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	_, b, e := s.importFile("Encrypted configuration", "*.sadpconfig", 64<<20)
	if e != nil || b == nil {
		return sites.Document{}, e
	}
	defer clear(b)
	return s.manager.ImportConfiguration(site, device, b, passphrase)
}
func (s *ToolkitService) CalculateRetention(scenario retention.Scenario) (retention.Result, error) {
	return retention.Calculate(scenario)
}
func (s *ToolkitService) ImportImage(site, kind string) (sites.Asset, error) {
	if e := s.ready(); e != nil {
		return sites.Asset{}, e
	}
	if kind != "plan" && kind != "branding" {
		return sites.Asset{}, errors.New("choose a plan or branding image")
	}
	name, data, e := s.importFile("Plan or logo image", "*.png;*.jpg;*.jpeg;*.webp", 16<<20)
	if e != nil || data == nil {
		return sites.Asset{}, e
	}
	return s.manager.Store.SaveAsset(sites.Asset{SiteID: site, Kind: kind, Title: name}, data)
}
func (s *ToolkitService) Image(site, id string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	a, b, e := s.manager.Store.AssetData(site, id)
	if e != nil {
		return "", e
	}
	if a.Encrypted || a.MIME != "image/png" {
		clear(b)
		return "", errors.New("only site images may be displayed")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b), nil
}
func (s *ToolkitService) DeleteImage(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.Store.DeleteAsset(site, id)
}
func (s *ToolkitService) CreateReport(o toolkit.ReportOptions) (sites.Document, error) {
	if e := s.ready(); e != nil {
		return sites.Document{}, e
	}
	return s.manager.CreateReport(o)
}
func (s *ToolkitService) ExportReport(site, id, format string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	b, e := s.manager.ExportReport(site, id, format)
	if e != nil {
		return "", e
	}
	return s.saveFile("SADPTools-handover."+format, b)
}
func (s *ToolkitService) ExportEvidence(site, id string) (string, error) {
	if e := s.ready(); e != nil {
		return "", e
	}
	d, e := s.manager.Store.Document(site, id)
	if e != nil {
		return "", e
	}
	switch d.Kind {
	case "nvr-channels", "stream-diagnostic", "walk-test", "retention", "plan":
	default:
		return "", errors.New("this document needs its dedicated export")
	}
	b, e := json.MarshalIndent(d, "", "  ")
	if e != nil {
		return "", e
	}
	return s.saveFile("SADPTools-"+d.Kind+".json", b)
}
func (s *ToolkitService) StartRemote(site, id string) error {
	if e := s.ready(); e != nil {
		return e
	}
	return s.manager.StartRemote(site, id)
}
func (s *ToolkitService) ImportRemote(site, id string, devices []string) (sites.SaveResult, error) {
	if e := s.ready(); e != nil {
		return sites.SaveResult{}, e
	}
	return s.manager.ImportRemote(site, id, devices)
}

func (s *ToolkitService) ExportEvidenceFormat(site, id, format string) (string, error) {
	if format == "json" {
		return s.ExportEvidence(site, id)
	}
	if e := s.ready(); e != nil {
		return "", e
	}
	d, e := s.manager.Store.Document(site, id)
	if e != nil {
		return "", e
	}
	headers := []string{"Field", "Value"}
	rows := [][]string{}
	if d.Kind == "nvr-channels" {
		var inv nvr.Inventory
		if json.Unmarshal(d.Data, &inv) != nil {
			return "", errors.New("invalid channel inventory")
		}
		headers = []string{"Recorder", "Observed", "Channel", "Name", "Address", "Model", "MAC", "Serial", "Connection", "Recording", "Linked device", "Match confidence", "Advertised streams"}
		for _, c := range inv.Channels {
			ids := []string{}
			for _, st := range c.Streams {
				ids = append(ids, st.ID)
			}
			rows = append(rows, []string{inv.NVRID, inv.At, c.ID, c.Name, c.Address, c.Model, c.MAC, c.Serial, c.Status, c.Recording, c.LinkedDeviceID, c.Confidence, fmt.Sprint(ids)})
		}
	} else if d.Kind == "stream-diagnostic" || d.Kind == "walk-test" || d.Kind == "retention" {
		clean, e := reports.CleanJSON(d.Data, false)
		if e != nil {
			return "", e
		}
		rows = append(rows, []string{d.Title, clean})
	} else {
		return "", errors.New("unsupported evidence export")
	}
	if format == "text" {
		b := []byte{}
		for _, r := range rows {
			for i, v := range r {
				b = append(b, []byte(headers[i]+": "+v+"\n")...)
			}
			b = append(b, '\n')
		}
		return s.saveFile("SADPTools-"+d.Kind+".txt", b)
	}
	if format != "csv" {
		return "", errors.New("choose CSV, JSON or text")
	}
	b, e := reports.CSV(headers, rows)
	if e != nil {
		return "", e
	}
	return s.saveFile("SADPTools-"+d.Kind+".csv", b)
}
