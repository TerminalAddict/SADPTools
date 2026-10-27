package isapi

import (
	"context"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/recordings"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"github.com/google/uuid"
	"regexp"
	"strings"
	"time"
)

var trackID = regexp.MustCompile(`^[0-9]{1,8}$`)

// SearchRecordings is a fixed read-only search POST, never a configurable request.
func (c *Client) SearchRecordings(ctx context.Context, track string, w recordings.Window, s vault.Secret) (recordings.Result, error) {
	r := recordings.Result{Provider: "ISAPI recording search", Window: w, Channel: track, Intervals: []recordings.Interval{}, Notes: []string{}, Replay: "not-verified"}
	if !trackID.MatchString(track) {
		return r, errors.New("select a numeric recording track advertised by the recorder")
	}
	if e := w.Validate(); e != nil {
		return r, e
	}
	id := uuid.NewString()
	for position := 0; position < 2000; {
		body := []byte(fmt.Sprintf(`<CMSearchDescription version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema"><searchID>%s</searchID><trackIDList><trackID>%s</trackID></trackIDList><timeSpanList><timeSpan><startTime>%s</startTime><endTime>%s</endTime></timeSpan></timeSpanList><maxResults>100</maxResults><searchResultPostion>%d</searchResultPostion><metadataList><metadataDescriptor>//recordType.meta.std-cgi.com</metadataDescriptor></metadataList></CMSearchDescription>`, id, track, w.Start.UTC().Format(time.RFC3339), w.End.UTC().Format(time.RFC3339), position))
		data, e := c.request(ctx, "/ISAPI/ContentMgmt/search", "POST", body, s, nil)
		clear(body)
		if e != nil {
			return r, e
		}
		root, e := xmlsafe.Parse(data)
		clear(data)
		if e != nil {
			return r, e
		}
		if root.Name != "CMSearchResult" {
			return r, errors.New("invalid recording search response")
		}
		status := strings.ToUpper(root.Value("responseStatusStrg"))
		if strings.EqualFold(root.Value("responseStatus"), "false") {
			return r, ErrUnsupported
		}
		if status != "OK" && status != "MORE" && status != "NO MATCHES" {
			return r, ErrUnsupported
		}
		items := root.All("searchMatchItem")
		if len(items) > 100 {
			r.Notes = append(r.Notes, "Response exceeded requested page size")
			break
		}
		for _, item := range items {
			if item.Value("trackID") != track {
				return r, errors.New("recording search returned a different track")
			}
			start, e1 := time.Parse(time.RFC3339Nano, item.Value("timeSpan", "startTime"))
			end, e2 := time.Parse(time.RFC3339Nano, item.Value("timeSpan", "endTime"))
			if e1 != nil || e2 != nil || !end.After(start) {
				r.Notes = append(r.Notes, "Ambiguous or invalid timestamp; gaps withheld")
				return recordings.Analyse(r)
			}
			r.Intervals = append(r.Intervals, recordings.Interval{Start: start.UTC(), End: end.UTC(), Channel: track, Precision: "segment"})
		}
		position += len(items)
		if status == "OK" || status == "NO MATCHES" {
			r.Complete = true
			break
		}
		if len(items) == 0 {
			break
		}
	}
	return recordings.Analyse(r)
}
