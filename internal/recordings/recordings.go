// Package recordings distinguishes metadata coverage from verified replay.
package recordings

import (
	"errors"
	"sort"
	"time"
)

type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type Interval struct {
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Channel   string    `json:"channel"`
	Precision string    `json:"precision"`
}
type Result struct {
	Provider  string     `json:"provider"`
	Window    Window     `json:"window"`
	Channel   string     `json:"channel"`
	Intervals []Interval `json:"intervals"`
	Gaps      []Window   `json:"gaps"`
	Complete  bool       `json:"complete"`
	State     string     `json:"state"`
	Replay    string     `json:"replay"`
	Notes     []string   `json:"notes"`
	Clock     string     `json:"clock"`
}

func (w Window) Validate() error {
	if w.Start.IsZero() || !w.End.After(w.Start) || w.End.Sub(w.Start) > 7*24*time.Hour {
		return errors.New("choose an explicit time-zone-aware recording window of at most seven days")
	}
	return nil
}
func Analyse(r Result) (Result, error) {
	if e := r.Window.Validate(); e != nil {
		return r, e
	}
	r.Gaps = []Window{}
	r.Replay = "not-verified"
	r.State = "metadata-only"
	ranges := []Interval{}
	precise := r.Complete
	for _, v := range r.Intervals {
		if v.Start.IsZero() || !v.End.After(v.Start) {
			precise = false
			continue
		}
		if v.Precision != "segment" {
			precise = false
		}
		if v.Channel != r.Channel {
			continue
		}
		if v.End.Before(r.Window.Start) || v.Start.After(r.Window.End) {
			continue
		}
		v.Start = v.Start.UTC()
		v.End = v.End.UTC()
		if v.Start.Before(r.Window.Start) {
			v.Start = r.Window.Start
		}
		if v.End.After(r.Window.End) {
			v.End = r.Window.End
		}
		ranges = append(ranges, v)
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start.Before(ranges[j].Start) })
	r.Intervals = ranges
	if !precise {
		r.Notes = append(r.Notes, "Partial or coarse metadata: precise gaps cannot be established. Verify playback manually on the recorder.")
		return r, nil
	}
	cursor := r.Window.Start
	for _, v := range ranges {
		if v.Start.After(cursor) {
			r.Gaps = append(r.Gaps, Window{cursor, v.Start})
		}
		if v.End.After(cursor) {
			cursor = v.End
		}
	}
	if cursor.Before(r.Window.End) {
		r.Gaps = append(r.Gaps, Window{cursor, r.Window.End})
	}
	r.Notes = append(r.Notes, "Gaps are absences in returned metadata, not proof of lost footage. Live video never verifies recording.")
	return r, nil
}
