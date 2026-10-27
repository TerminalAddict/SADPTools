package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/installation"
	"github.com/TerminalAddict/SADPTools/internal/nvr"
	"github.com/TerminalAddict/SADPTools/internal/reports"
	"github.com/TerminalAddict/SADPTools/internal/retention"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/sites"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"strings"
)

func measured(v *float64, unit string) string {
	if v == nil {
		return "Unknown"
	}
	return fmt.Sprintf("%.2f %s", *v, unit)
}
func counter(v *uint64) string {
	if v == nil {
		return "Unknown"
	}
	return fmt.Sprint(*v)
}
func evidenceSection(d sites.Document, redact bool, names map[string]string) (reports.Section, error) {
	out := reports.Section{Title: sanitize.Label(d.Title), Note: "Saved evidence · " + d.Updated, Rows: [][]string{}}
	switch d.Kind {
	case "field-evidence":
		var f FieldEvidence
		if json.Unmarshal(d.Data, &f) != nil {
			return out, errors.New("invalid field evidence")
		}
		out.Headers = []string{"Test", "State", "Evidence", "Next check"}
		for _, step := range f.Run.Steps {
			out.Rows = append(out.Rows, []string{step.Test, step.Status, step.Evidence, step.Next})
		}
		for _, finding := range f.Findings {
			out.Rows = append(out.Rows, []string{finding.Name, finding.State, finding.Evidence, finding.Recommendation + "; what could break: " + finding.CouldBreak})
		}
		if f.Imaging != nil {
			out.Note += "\nPrivacy masks: " + f.Imaging.PrivacyMasks
			for _, setting := range f.Imaging.Settings {
				out.Rows = append(out.Rows, []string{setting.Name, setting.State, setting.Value, setting.Reason})
			}
		}
		if f.Recording != nil {
			r := f.Recording
			out.Note += "\nRecording: " + r.State + "; replay: " + r.Replay + "; " + strings.Join(r.Notes, "; ")
			for _, v := range r.Intervals {
				out.Rows = append(out.Rows, []string{"Recording metadata " + v.Channel, "metadata-only", v.Start.String() + " to " + v.End.String(), v.Precision})
			}
			for _, g := range r.Gaps {
				out.Rows = append(out.Rows, []string{"Metadata gap", "observed", g.Start.String() + " to " + g.End.String(), "Verify recorded playback"})
			}
		}
	case "anpr-session", "image-change":
		out.Headers = []string{"Evidence", "Value"}
		var obj map[string]any
		if json.Unmarshal(d.Data, &obj) != nil {
			return out, errors.New("invalid commissioning evidence")
		}
		for _, key := range []string{"grade", "matchRate", "source", "received", "started", "completed", "transport", "field", "previous", "proposed", "state", "at"} {
			if value, ok := obj[key]; ok {
				b, _ := json.Marshal(value)
				out.Rows = append(out.Rows, []string{key, string(b)})
			}
		}

	case "nvr-channels":
		var inv nvr.Inventory
		if json.Unmarshal(d.Data, &inv) != nil {
			return out, errors.New("invalid channel inventory")
		}
		out.Note += " · " + inv.Provider + "\n" + strings.Join(inv.Warnings, "\n")
		out.Headers = []string{"Channel / name", "Camera identity", "Connection / recording", "Linked camera / confidence", "Advertised streams"}
		for _, c := range inv.Channels {
			identity := c.Model + "\n" + c.Address + "\n" + c.MAC + "\n" + c.Serial
			if redact {
				identity = c.Model + "\n[identity redacted]"
			}
			streams := []string{}
			for _, s := range c.Streams {
				streams = append(streams, fmt.Sprintf("%s · %s %d×%d", s.ID, s.Codec, s.Width, s.Height))
			}
			out.Rows = append(out.Rows, []string{c.ID + " · " + c.Name, identity, c.Status + " / " + c.Recording, names[c.LinkedDeviceID] + "\n" + c.Confidence, strings.Join(streams, "\n")})
		}
	case "stream-diagnostic":
		var v struct {
			Status   streaming.Status `json:"status"`
			Profile  string           `json:"profile"`
			StreamID string           `json:"streamID"`
			Player   struct {
				DisplayedFPS  *float64 `json:"displayedFPS"`
				DroppedFrames *uint64  `json:"droppedFrames"`
				TestSeconds   float64  `json:"testSeconds"`
			} `json:"player"`
		}
		if json.Unmarshal(d.Data, &v) != nil {
			return out, errors.New("invalid stream diagnostic")
		}
		s := v.Status
		m := s.Metrics
		out.Headers = []string{"Metric", "Value", "Source"}
		out.Rows = [][]string{{"Profile / NVR stream", v.Profile + " / " + v.StreamID, "Selected stream"}, {"Codec / resolution", fmt.Sprintf("%s · %d×%d", s.Info.Codec, s.Info.Width, s.Info.Height), "Media metadata"}, {"Transport", s.Info.Transport, "RTSP client"}, {"RTP bitrate", measured(m.RTPMbps, "Mb/s"), "Observed RTP bytes including RTP headers"}, {"Encoded rate", measured(m.EncodedFPS, "access units/s"), "RTP parser"}, {"Displayed rate", measured(v.Player.DisplayedFPS, "frames/s"), "Player frame callbacks"}, {"Dropped video frames", counter(v.Player.DroppedFrames), "WebView playback quality (when available)"}, {"First media", measured(m.FirstMediaMS, "ms"), "Session monotonic timing"}, {"First playing", measured(m.FirstPlayingMS, "ms"), "Player acknowledgement"}, {"Lost RTP packets", counter(m.LostPackets), "RTP receiver"}, {"Decode errors", counter(m.DecodeErrors), "RTP/access-unit parser"}, {"Outcome", s.State + " · " + s.ErrorCode + "\n" + s.Message, "Saved session observation"}}
		out.Note += fmt.Sprintf(" · %.1f s test window\n", v.Player.TestSeconds)
		for _, t := range s.Timeline {
			out.Note += fmt.Sprintf("%.0f ms: %s; ", t.ElapsedMS, t.State)
		}
	case "walk-test":
		var w WalkTest
		if json.Unmarshal(d.Data, &w) != nil {
			return out, errors.New("invalid walk-test")
		}
		out.Note += fmt.Sprintf("\nOverall: %s · received %d · %s\n%s\n%s", w.Result, w.Total, w.Cleanup, w.Notes, w.Error)
		out.Headers = []string{"Event / source channel", "Device / receipt time", "State", "Grade / notes"}
		grades := map[string]EventAssessment{}
		for _, a := range w.Assessments {
			grades[a.EventID] = a
		}
		for _, e := range w.Events {
			a, ok := grades[e.EventID]
			grade := "not-tested"
			if ok {
				grade = a.Result + " · " + a.Notes
			}
			clock := ""
			if e.ClockWarning {
				clock = "\nClock offset suspicious"
			}
			out.Rows = append(out.Rows, []string{e.Type + " / " + e.Channel + "\n" + e.Topic, e.DeviceTime + "\n" + e.Received + clock, e.State, grade})
		}
		if len(out.Rows) == 0 {
			out.Note += "\nNo event observed; this alone does not establish detection failure."
		}
	case "retention":
		var s retention.Scenario
		if json.Unmarshal(d.Data, &s) != nil {
			return out, errors.New("invalid storage scenario")
		}
		r, e := retention.Calculate(s)
		if e != nil {
			return out, e
		}
		out.Note += fmt.Sprintf("\nCapacity %.2f %s · reserve %.1f%% · overhead %.1f%% · margin %.1f%%\nTypical retention %s; range %s–%s. Target %.1f days.", s.Capacity, s.Unit, s.Reserve*100, s.Overhead*100, s.SafetyMargin*100, measured(r.RetentionDays, "days"), measured(r.LowRetentionDays, "days"), measured(r.HighRetentionDays, "days"), s.TargetDays)
		out.Note += "\n" + strings.Join(r.Warnings, "\n")
		out.Headers = []string{"Group / count", "Bitrate provenance", "Low / typical / high", "Schedule / duty", "Audio"}
		for _, ch := range s.Channels {
			schedule := fmt.Sprintf("%.1f h/day", ch.HoursPerDay)
			if len(ch.Calendar) == 7 {
				schedule = fmt.Sprint(ch.Calendar) + " h Mon–Sun"
			}
			out.Rows = append(out.Rows, []string{fmt.Sprintf("%s · %d", ch.Label, ch.Count), ch.Source, measured(ch.LowMbps, "Mb/s") + " / " + measured(ch.Mbps, "Mb/s") + " / " + measured(ch.HighMbps, "Mb/s"), fmt.Sprintf("%s · %.1f%%", schedule, ch.DutyCycle*100), fmt.Sprintf("%.3f Mb/s", ch.AudioMbps)})
		}
	case "plan":
		var p installation.Plan
		if json.Unmarshal(d.Data, &p) != nil {
			return out, errors.New("invalid plan")
		}
		out.Note += "\n" + p.Notes + "\nManual coordinates and coverage are approximate."
		out.Headers = []string{"Marker / camera", "Layer", "Position", "Direction / cone", "Notes"}
		for _, m := range p.Markers {
			out.Rows = append(out.Rows, []string{m.Label + " / " + names[m.DeviceID], m.Layer, fmt.Sprintf("%.1f%%, %.1f%%", m.X*100, m.Y*100), fmt.Sprintf("%.1f° / %.1f°", m.Bearing, m.Cone), m.Notes})
		}
	default:
		return out, errors.New("unsupported report evidence")
	}
	out.Note = sanitize.Text(out.Note)
	for i := range out.Rows {
		for j := range out.Rows[i] {
			out.Rows[i][j] = sanitize.Text(out.Rows[i][j])
		}
	}
	return out, nil
}
