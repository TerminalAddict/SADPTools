// Package retention calculates estimates from explicit, labelled assumptions.
package retention

import (
	"errors"
	"math"
)

type Channel struct {
	Label       string    `json:"label"`
	Count       int       `json:"count"`
	Mbps        *float64  `json:"mbps"`
	LowMbps     *float64  `json:"lowMbps"`
	HighMbps    *float64  `json:"highMbps"`
	AudioMbps   float64   `json:"audioMbps"`
	HoursPerDay float64   `json:"hoursPerDay"`
	Calendar    []float64 `json:"calendar"`
	DutyCycle   float64   `json:"dutyCycle"`
	Source      string    `json:"source"`
}
type Scenario struct {
	Name         string    `json:"name"`
	Channels     []Channel `json:"channels"`
	Capacity     float64   `json:"capacity"`
	Unit         string    `json:"unit"`
	Reserve      float64   `json:"reserve"`
	Overhead     float64   `json:"overhead"`
	SafetyMargin float64   `json:"safetyMargin"`
	TargetDays   float64   `json:"targetDays"`
}
type Result struct {
	Complete          bool     `json:"complete"`
	UsableBytes       float64  `json:"usableBytes"`
	BytesPerDay       *float64 `json:"bytesPerDay"`
	LowBytesPerDay    *float64 `json:"lowBytesPerDay"`
	HighBytesPerDay   *float64 `json:"highBytesPerDay"`
	RetentionDays     *float64 `json:"retentionDays"`
	LowRetentionDays  *float64 `json:"lowRetentionDays"`
	HighRetentionDays *float64 `json:"highRetentionDays"`
	RequiredBytes     *float64 `json:"requiredBytes"`
	Warnings          []string `json:"warnings"`
}

func Number(v float64) *float64 { return &v }
func valid(v, min, max float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= min && v <= max
}
func Bytes(value float64, unit string) (float64, error) {
	units := map[string]float64{"TB": 1e12, "TiB": 1 << 40, "GB": 1e9, "GiB": 1 << 30}
	m, ok := units[unit]
	if !ok || !valid(value, 0, 1e8) {
		return 0, errors.New("choose TB, TiB, GB or GiB and a nonnegative capacity")
	}
	return value * m, nil
}
func Calculate(s Scenario) (Result, error) {
	out := Result{Complete: true, Warnings: []string{"Estimate only: use capacity after RAID/redundancy. Filesystem reserve, NVR retention rules, VBR activity, pre/post-event buffers and stream choices affect actual retention.", "Mb/s means one million bits per second; TB is decimal and TiB is binary. Recording fraction and calendar are explicit assumptions."}}
	capacity, e := Bytes(s.Capacity, s.Unit)
	if e != nil {
		return out, e
	}
	if len(s.Channels) == 0 || len(s.Channels) > 1024 || !valid(s.Reserve, 0, .95) || !valid(s.Overhead, 0, 2) || !valid(s.SafetyMargin, 0, 2) || !valid(s.TargetDays, 0, 36600) {
		return out, errors.New("check channel count, reserve, overhead, safety margin and target days")
	}
	out.UsableBytes = capacity * (1 - s.Reserve)
	low, typical, high := 0.0, 0.0, 0.0
	for _, c := range s.Channels {
		if c.Count < 1 || c.Count > 100000 || !valid(c.HoursPerDay, 0, 24) || !valid(c.DutyCycle, 0, 1) || !valid(c.AudioMbps, 0, 100) || len(c.Calendar) != 0 && len(c.Calendar) != 7 {
			return out, errors.New("invalid channel count, hours, calendar, audio or recording fraction")
		}
		if c.Source != "measured" && c.Source != "configured" && c.Source != "estimate" {
			return out, errors.New("label every bitrate as measured, configured or estimate")
		}
		hours := c.HoursPerDay
		if len(c.Calendar) == 7 {
			hours = 0
			for _, h := range c.Calendar {
				if !valid(h, 0, 24) {
					return out, errors.New("calendar needs seven daily hours from 0 to 24")
				}
				hours += h / 7
			}
		}
		for _, v := range []*float64{c.Mbps, c.LowMbps, c.HighMbps} {
			if v != nil && !valid(*v, 0, 10000) {
				return out, errors.New("bitrate must be finite and between 0 and 10000 Mb/s")
			}
		}
		if c.Mbps == nil || *c.Mbps == 0 {
			out.Complete = false
			out.Warnings = append(out.Warnings, "A channel bitrate is unknown; no complete site retention estimate is possible")
			continue
		}
		a, b := *c.Mbps, *c.Mbps
		if c.LowMbps != nil {
			a = *c.LowMbps
		}
		if c.HighMbps != nil {
			b = *c.HighMbps
		}
		if a <= 0 || a > *c.Mbps || b < *c.Mbps {
			return out, errors.New("VBR range must be positive low <= typical <= high")
		}
		factor := float64(c.Count) * 1e6 / 8 * hours * 3600 * c.DutyCycle * (1 + s.Overhead) * (1 + s.SafetyMargin)
		low += (a + c.AudioMbps) * factor
		typical += (*c.Mbps + c.AudioMbps) * factor
		high += (b + c.AudioMbps) * factor
	}
	if !out.Complete {
		return out, nil
	}
	out.BytesPerDay = Number(typical)
	out.LowBytesPerDay = Number(low)
	out.HighBytesPerDay = Number(high)
	out.RequiredBytes = Number(typical * s.TargetDays)
	if typical > 0 {
		out.RetentionDays = Number(out.UsableBytes / typical)
		out.LowRetentionDays = Number(out.UsableBytes / high)
		out.HighRetentionDays = Number(out.UsableBytes / low)
	} else {
		out.Warnings = append(out.Warnings, "No recording hours or recording fraction: retention is not available, not infinite")
	}
	return out, nil
}
