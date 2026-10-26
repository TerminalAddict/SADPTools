package retention

import (
	"math"
	"testing"
)

func base() Scenario {
	return Scenario{Channels: []Channel{{Count: 1, Mbps: Number(8), HoursPerDay: 24, DutyCycle: 1, Source: "estimate"}}, Capacity: 1, Unit: "TB", TargetDays: 30}
}
func TestRetentionUnitsRangesAndUnknowns(t *testing.T) {
	s := base()
	r, e := Calculate(s)
	if e != nil || *r.BytesPerDay != 86400e6 || math.Abs(*r.RetentionDays-1e12/86400e6) > 1e-9 {
		t.Fatal(r, e)
	}
	s.Unit = "TiB"
	r, _ = Calculate(s)
	if r.UsableBytes != 1<<40 {
		t.Fatal("binary units")
	}
	s.Channels[0].Count = 8
	s.Channels[0].HoursPerDay = 12
	s.Channels[0].DutyCycle = .5
	s.Channels[0].LowMbps = Number(4)
	s.Channels[0].HighMbps = Number(12)
	r, e = Calculate(s)
	if e != nil || *r.BytesPerDay != 2*86400e6 || *r.LowRetentionDays >= *r.HighRetentionDays {
		t.Fatal(r, e)
	}
	s.Channels[0].Mbps = nil
	r, e = Calculate(s)
	if e != nil || r.Complete || r.RetentionDays != nil {
		t.Fatal("unknown treated as infinite", r, e)
	}
}
func TestMixedCalendarAndInvalid(t *testing.T) {
	s := base()
	s.Channels = append(s.Channels, Channel{Count: 2, Mbps: Number(4), HoursPerDay: 24, Calendar: []float64{8, 8, 8, 8, 8, 0, 0}, DutyCycle: 1, AudioMbps: .064, Source: "measured"})
	if r, e := Calculate(s); e != nil || *r.BytesPerDay <= 86400e6 {
		t.Fatal(r, e)
	}
	s.Channels[0].Mbps = Number(math.NaN())
	if _, e := Calculate(s); e == nil {
		t.Fatal("NaN accepted")
	}
	s = base()
	s.Channels[0].Mbps = Number(0)
	r, e := Calculate(s)
	if e != nil || r.RetentionDays != nil {
		t.Fatal("zero bitrate fabricated")
	}
}
