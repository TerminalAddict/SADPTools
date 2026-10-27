package isapi

import (
	"context"
	"github.com/TerminalAddict/SADPTools/internal/recordings"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Synthetic recorder responses: never collected from a customer's network.
func TestRecordingSearchPaginationFailuresAndNoReplayClaims(t *testing.T) {
	for _, mode := range []string{"pages", "partial", "false", "wrong-track", "timezone", "unauthorised", "malformed", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := New("192.0.2.4", "http://192.0.2.4/", "")
			calls := 0
			c.http.Transport = syntheticTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				body := string(raw)
				if r.Method != "POST" || r.URL.Path != "/ISAPI/ContentMgmt/search" || !strings.Contains(body, "<trackID>101</trackID>") {
					t.Fatal("unsafe search")
				}
				if calls == 2 && !strings.Contains(body, "<searchResultPostion>1</searchResultPostion>") {
					t.Fatal("pagination did not advance")
				}
				if mode == "cancelled" {
					return nil, context.Canceled
				}
				status, flag, track, stamp := "OK", "true", "101", "2026-01-01T00:10:00Z"
				if calls == 1 && (mode == "pages" || mode == "partial") {
					status = "MORE"
				}
				if mode == "false" {
					flag = "false"
				}
				if mode == "wrong-track" {
					track = "102"
				}
				if mode == "timezone" {
					stamp = "2026-01-01T00:10:00"
				}
				items := `<searchMatchItem><trackID>` + track + `</trackID><timeSpan><startTime>` + stamp + `</startTime><endTime>2026-01-01T00:20:00Z</endTime></timeSpan><mediaSegmentDescriptor><playbackURI>rtsp://user:secret@192.0.2.4/clip</playbackURI></mediaSegmentDescriptor></searchMatchItem>`
				if mode == "partial" && calls == 2 {
					items = ""
					status = "MORE"
				}
				reply := `<CMSearchResult><responseStatus>` + flag + `</responseStatus><responseStatusStrg>` + status + `</responseStatusStrg><matchList>` + items + `</matchList></CMSearchResult>`
				if mode == "malformed" {
					reply = "<broken"
				}
				code := 200
				if mode == "unauthorised" {
					code = 403
				}
				return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply))}, nil
			})
			start, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
			r, e := c.SearchRecordings(context.Background(), "101", recordings.Window{Start: start, End: start.Add(time.Hour)}, vault.Secret{})
			switch mode {
			case "pages":
				if e != nil || !r.Complete || len(r.Gaps) != 2 || calls != 2 {
					t.Fatal(r, e, calls)
				}
			case "partial", "timezone":
				if e != nil || r.Complete || len(r.Gaps) != 0 {
					t.Fatal("fabricated precise gaps", r, e)
				}
			default:
				if e == nil || r.Complete {
					t.Fatal("failure misreported", r, e)
				}
			}
			if r.Replay == "verified" {
				t.Fatal("metadata treated as replay")
			}
			n := calls
			if _, e = c.SearchRecordings(context.Background(), "101</trackID>", recordings.Window{Start: start, End: start.Add(time.Hour)}, vault.Secret{}); e == nil || calls != n {
				t.Fatal("untrusted track escaped validation")
			}
		})
	}
}
