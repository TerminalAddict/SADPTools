package onvif

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"strconv"
	"strings"
	"time"
)

type PlateEvent struct {
	ID          string   `json:"id"`
	DeviceTime  string   `json:"deviceTime"`
	Received    string   `json:"received"`
	Source      string   `json:"source"`
	Type        string   `json:"type"`
	Plate       string   `json:"plate"`
	Confidence  *float64 `json:"confidence"`
	Direction   string   `json:"direction"`
	Lane        string   `json:"lane"`
	Integration string   `json:"integration"`
}

func (s *Subscription) PullPlates(ctx context.Context) ([]PlateEvent, error) {
	data, e := s.pullRaw(ctx)
	if e != nil {
		return nil, e
	}
	defer clear(data)
	return ParsePlates(data, time.Now().UTC())
}
func ParsePlates(data []byte, now time.Time) ([]PlateEvent, error) {
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return nil, e
	}
	out := []PlateEvent{}
	for _, n := range root.All("NotificationMessage") {
		topic := n.Value("Topic")
		lower := strings.ToLower(topic)
		known := strings.Contains(lower, "plate") || strings.Contains(lower, "anpr") || strings.Contains(lower, "lpr")
		if !known {
			continue
		}
		p := PlateEvent{Received: now.Format(time.RFC3339Nano), Source: sanitize.Label(topic), Type: "ANPR event (vendor topic)", Integration: "not-verified"}
		for _, msg := range n.All("Message") {
			if at, e := time.Parse(time.RFC3339Nano, msg.Attribute("UtcTime")); e == nil {
				p.DeviceTime = at.UTC().Format(time.RFC3339Nano)
			}
		}
		for _, item := range n.All("SimpleItem") {
			value := sanitize.Label(item.Attribute("Value"))
			switch strings.ToLower(item.Attribute("Name")) {
			case "licenseplate", "licenceplate", "platenumber", "platetext", "plate":
				p.Plate = value
			case "confidence", "plateconfidence":
				if v, e := strconv.ParseFloat(value, 64); e == nil && v >= 0 && v <= 100 {
					p.Confidence = &v
				}
			case "direction":
				p.Direction = value
			case "lane", "laneid":
				p.Lane = value
			}
		}
		hash := sha256.Sum256([]byte(p.Source + "\x00" + p.DeviceTime + "\x00" + p.Plate + "\x00" + p.Lane))
		p.ID = fmt.Sprintf("%x", hash[:16])
		if p.DeviceTime == "" {
			p.ID += "-" + p.Received
		}
		out = append(out, p)
		if len(out) >= 256 {
			break
		}
	}
	return out, nil
}
