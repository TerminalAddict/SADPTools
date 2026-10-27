package onvif

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/recordings"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"time"
)

const recordingNS = "http://www.onvif.org/ver10/recording/wsdl"
const searchNS = "http://www.onvif.org/ver10/search/wsdl"

func (c *Client) RecordingMetadata(ctx context.Context, w recordings.Window, s vault.Secret) (recordings.Result, error) {
	r := recordings.Result{Provider: "ONVIF recording/search", Window: w, Channel: "all", Intervals: []recordings.Interval{}, Notes: []string{"Earliest/latest recording bounds are coarse; no continuity or replay claim is made"}}
	if e := w.Validate(); e != nil {
		return r, e
	}
	endpoint, e := c.service(ctx, recordingNS, "Recording", s)
	if e != nil {
		return r, e
	}
	data, e := c.soap(ctx, endpoint, recordingNS, "GetRecordings", "", s, 0)
	if e != nil {
		return r, e
	}
	root, e := xmlsafe.Parse(data)
	clear(data)
	if e != nil {
		return r, e
	}
	if len(root.All("GetRecordingsResponse")) != 1 {
		return r, errors.New("recording response is incomplete")
	}
	items := root.All("RecordingItem")
	if len(items) > 64 {
		return r, errors.New("recorder has more than 64 recordings; select vendor-specific channel search")
	}
	search, e := c.service(ctx, searchNS, "Search", s)
	if e != nil {
		return r, e
	}
	for _, item := range items {
		token := item.Value("RecordingToken")
		if token == "" || len(token) > 1024 {
			return r, errors.New("invalid recording reference")
		}
		data, e := c.soap(ctx, search, searchNS, "GetRecordingInformation", "<tds:RecordingToken>"+xmlText(token)+"</tds:RecordingToken>", s, 0)
		if e != nil {
			return r, e
		}
		info, e := xmlsafe.Parse(data)
		clear(data)
		if e != nil {
			return r, e
		}
		if len(info.All("RecordingInformation")) != 1 {
			return r, errors.New("recording information is incomplete")
		}
		for _, n := range info.All("RecordingInformation") {
			start, e1 := time.Parse(time.RFC3339Nano, n.Value("EarliestRecording"))
			end, e2 := time.Parse(time.RFC3339Nano, n.Value("LatestRecording"))
			if e1 == nil && e2 == nil {
				r.Intervals = append(r.Intervals, recordings.Interval{Start: start, End: end, Channel: "all", Precision: "coarse"})
			} else {
				r.Notes = append(r.Notes, "Recording bounds missing or ambiguous")
			}
		}
	}
	return recordings.Analyse(r)
}
