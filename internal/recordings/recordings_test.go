package recordings

import (
	"testing"
	"time"
)

func tm(v string) time.Time { t, _ := time.Parse(time.RFC3339, v); return t }
func TestUnsortedOverlappingTimezoneRanges(t *testing.T) {
	w := Window{tm("2026-01-01T00:00:00Z"), tm("2026-01-01T04:00:00Z")}
	r, e := Analyse(Result{Window: w, Channel: "1", Complete: true, Intervals: []Interval{{tm("2026-01-01T14:00:00+13:00"), tm("2026-01-01T15:00:00+13:00"), "1", "segment"}, {w.Start, tm("2026-01-01T01:30:00Z"), "1", "segment"}, {tm("2026-01-01T03:00:00Z"), w.End, "1", "segment"}}})
	if e != nil || len(r.Gaps) != 1 || r.Gaps[0].Start != tm("2026-01-01T02:00:00Z") || r.Replay != "not-verified" {
		t.Fatal(r, e)
	}
	r.Complete = false
	r, e = Analyse(r)
	if e != nil || len(r.Gaps) != 0 {
		t.Fatal("partial search fabricated gap")
	}
	r.Complete = true
	r.Intervals[0].Precision = "coarse"
	r, _ = Analyse(r)
	if len(r.Gaps) != 0 {
		t.Fatal("coarse bounds fabricated gap")
	}
}
func TestEmptyCompleteWindowAndInvalid(t *testing.T) {
	w := Window{tm("2026-01-01T00:00:00Z"), tm("2026-01-01T01:00:00Z")}
	r, e := Analyse(Result{Window: w, Channel: "1", Complete: true})
	if e != nil || len(r.Gaps) != 1 {
		t.Fatal(r, e)
	}
	if _, e = Analyse(Result{}); e == nil {
		t.Fatal("invalid window")
	}
}
