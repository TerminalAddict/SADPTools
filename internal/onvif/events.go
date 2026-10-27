package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/sanitize"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/TerminalAddict/SADPTools/internal/xmlsafe"
	"strings"
	"time"
)

type EventCapabilities struct {
	Supported bool     `json:"supported"`
	Topics    []string `json:"topics"`
	Reason    string   `json:"reason"`
}
type CameraEvent struct {
	EventID            string   `json:"eventID"`
	Type               string   `json:"type"`
	Topic              string   `json:"topic"`
	State              string   `json:"state"`
	Channel            string   `json:"channel"`
	DeviceTime         string   `json:"deviceTime"`
	Received           string   `json:"received"`
	ClockOffsetSeconds *float64 `json:"clockOffsetSeconds"`
	ClockWarning       bool     `json:"clockWarning"`
	Mapping            string   `json:"mapping"`
}
type Subscription struct {
	client            *Client
	endpoint, headers string
	secret            vault.Secret
	expires           time.Time
}

func (c *Client) EventCapabilities(ctx context.Context, s vault.Secret) (EventCapabilities, error) {
	out := EventCapabilities{Topics: []string{}}
	endpoint, e := c.service(ctx, eventsNS, "Events", s)
	if e != nil {
		return out, e
	}
	data, e := c.action(ctx, endpoint, eventsNS, "GetEventProperties", "", eventsNS+"/EventPortType/GetEventPropertiesRequest", "", s, false)
	if e != nil {
		return out, e
	}
	defer clear(data)
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return out, e
	}
	out.Supported = true
	var walk func(*xmlsafe.Node, string)
	walk = func(n *xmlsafe.Node, path string) {
		if n == nil {
			return
		}
		if path != "" {
			path += "/"
		}
		path += n.Name
		if n.Attribute("topic") == "true" && len(out.Topics) < 128 {
			out.Topics = append(out.Topics, sanitize.Label(path))
		}
		for _, child := range n.Children {
			walk(child, path)
		}
	}
	for _, topics := range root.All("TopicSet") {
		for _, n := range topics.Children {
			walk(n, "")
		}
	}
	out.Reason = "Advertised ONVIF event topics; subscription support is verified on Start"
	return out, nil
}
func (c *Client) Subscribe(ctx context.Context, s vault.Secret) (*Subscription, error) {
	endpoint, e := c.service(ctx, eventsNS, "Events", s)
	if e != nil {
		return nil, e
	}
	data, e := c.action(ctx, endpoint, eventsNS, "CreatePullPointSubscription", "<tds:InitialTerminationTime>PT2M</tds:InitialTerminationTime>", eventsNS+"/EventPortType/CreatePullPointSubscriptionRequest", "", s, false)
	if e != nil {
		return nil, e
	}
	defer clear(data)
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return nil, e
	}
	refs := root.All("SubscriptionReference")
	if len(refs) != 1 {
		return nil, errors.New("ONVIF subscription reference missing")
	}
	raw := refs[0].Value("Address")
	if !c.sameAuthority(raw) {
		return nil, errors.New("event subscription changed host, port or scheme")
	}
	var headers bytes.Buffer
	enc := xml.NewEncoder(&headers)
	if params := refs[0].At("ReferenceParameters"); params != nil {
		if len(params.Children) > 16 {
			return nil, errors.New("too many subscription reference parameters")
		}
		for _, n := range params.Children {
			if e = encodeReference(enc, n, true); e != nil {
				return nil, e
			}
		}
	}
	enc.Flush()
	if headers.Len() > 16<<10 {
		return nil, errors.New("subscription reference parameters too large")
	}
	return &Subscription{client: c, endpoint: raw, headers: headers.String(), secret: s, expires: time.Now().Add(90 * time.Second)}, nil
}
func encodeReference(enc *xml.Encoder, n *xmlsafe.Node, top bool) error {
	start := xml.StartElement{Name: xml.Name{Space: n.Space, Local: n.Name}}
	if top {
		start.Attr = []xml.Attr{{Name: xml.Name{Space: "http://www.w3.org/2005/08/addressing", Local: "IsReferenceParameter"}, Value: "true"}}
	}
	if e := enc.EncodeToken(start); e != nil {
		return e
	}
	if e := enc.EncodeToken(xml.CharData(n.Text)); e != nil {
		return e
	}
	for _, c := range n.Children {
		if e := encodeReference(enc, c, false); e != nil {
			return e
		}
	}
	return enc.EncodeToken(start.End())
}
func (s *Subscription) Pull(ctx context.Context) ([]CameraEvent, error) {
	data, e := s.pullRaw(ctx)
	if e != nil {
		return nil, e
	}
	defer clear(data)
	return ParseEvents(data, time.Now().UTC())
}
func (s *Subscription) pullRaw(ctx context.Context) ([]byte, error) {
	if time.Now().After(s.expires) {
		b, e := s.client.action(ctx, s.endpoint, "http://docs.oasis-open.org/wsn/b-2", "Renew", "<tds:TerminationTime>PT2M</tds:TerminationTime>", "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/RenewRequest", s.headers, s.secret, false)
		clear(b)
		if e != nil {
			return nil, e
		}
		s.expires = time.Now().Add(90 * time.Second)
	}
	data, e := s.client.action(ctx, s.endpoint, eventsNS, "PullMessages", "<tds:Timeout>PT3S</tds:Timeout><tds:MessageLimit>64</tds:MessageLimit>", eventsNS+"/PullPointSubscription/PullMessagesRequest", s.headers, s.secret, false)
	if e != nil {
		return nil, e
	}
	return data, nil
}
func (s *Subscription) Close(ctx context.Context) error {
	data, e := s.client.action(ctx, s.endpoint, "http://docs.oasis-open.org/wsn/b-2", "Unsubscribe", "", "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest", s.headers, s.secret, false)
	clear(data)
	s.secret.Clear()
	return e
}
func ParseEvents(data []byte, receipt time.Time) ([]CameraEvent, error) {
	root, e := xmlsafe.Parse(data)
	if e != nil {
		return nil, e
	}
	messages := root.All("NotificationMessage")
	if len(messages) > 256 {
		return nil, errors.New("event batch exceeds limit")
	}
	out := []CameraEvent{}
	for _, n := range messages {
		event := CameraEvent{Topic: sanitize.Label(n.Value("Topic")), Type: "unknown", State: "unknown", Received: receipt.Format(time.RFC3339Nano), Mapping: "unrecognised vendor topic retained"}
		topic := strings.ToLower(event.Topic)
		switch {
		case strings.Contains(topic, "motion"):
			event.Type = "motion"
		case strings.Contains(topic, "line") && strings.Contains(topic, "cross"):
			event.Type = "line-crossing"
		case strings.Contains(topic, "intrusion") || strings.Contains(topic, "fielddetector"):
			event.Type = "intrusion"
		case strings.Contains(topic, "tamper"):
			event.Type = "tamper"
		case strings.Contains(topic, "digitalinput"):
			event.Type = "digital-input"
		}
		if event.Type != "unknown" {
			event.Mapping = "topic-name normalisation; confirm vendor meaning"
		}
		for _, msg := range n.All("Message") {
			if stamp := msg.Attribute("UtcTime"); stamp != "" {
				if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
					event.DeviceTime = at.UTC().Format(time.RFC3339Nano)
					offset := receipt.Sub(at).Seconds()
					event.ClockOffsetSeconds = &offset
					event.ClockWarning = offset > 300 || offset < -300
				}
			}
		}
		for _, item := range n.All("SimpleItem") {
			name := strings.ToLower(item.Attribute("Name"))
			value := item.Attribute("Value")
			if name == "videosourceconfigurationtoken" || name == "videosourcetoken" || name == "channel" || name == "input" {
				event.Channel = sanitize.Label(value)
			}
			if name == "ismotion" || name == "state" || name == "logicalstate" || name == "alarm" {
				switch strings.ToLower(value) {
				case "true", "1", "active", "start":
					event.State = "active"
				case "false", "0", "inactive", "stop":
					event.State = "inactive"
				}
			}
		}
		out = append(out, event)
	}
	return out, nil
}
