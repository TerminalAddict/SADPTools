package onvif

import (
	"context"
	"errors"
	"fmt"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEventMappingAndSubscriptionCleanup(t *testing.T) {
	c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device_service"})
	closed := false
	c.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		reply := ""
		switch {
		case strings.Contains(body, "GetServices"):
			reply = soap("GetServices", `<Service><Namespace>`+eventsNS+`</Namespace><XAddr>http://192.0.2.3/onvif/events</XAddr></Service>`)
		case strings.Contains(body, "CreatePullPointSubscription"):
			if !strings.Contains(r.Header.Get("Content-Type"), "EventPortType/CreatePullPointSubscriptionRequest") {
				t.Fatal("incorrect action")
			}
			reply = soap("CreatePullPointSubscription", `<SubscriptionReference><Address>http://192.0.2.3/onvif/events/sub1</Address><ReferenceParameters><SubscriptionId xmlns="urn:synthetic">synthetic&lt;id</SubscriptionId></ReferenceParameters></SubscriptionReference>`)
		case strings.Contains(body, "PullMessages"):
			if !strings.Contains(body, "synthetic&lt;id") || !strings.Contains(body, "IsReferenceParameter") {
				t.Fatal("lost subscription reference")
			}
			reply = soap("PullMessages", `<NotificationMessage><Topic>tns:RuleEngine/CellMotionDetector/Motion</Topic><Message><Message UtcTime="2026-01-01T00:00:00Z"><Source><SimpleItem Name="Channel" Value="1"/></Source><Data><SimpleItem Name="IsMotion" Value="true"/></Data></Message></Message></NotificationMessage>`)
		case strings.Contains(body, "Unsubscribe"):
			closed = true
			reply = soap("Unsubscribe", "")
		default:
			t.Fatal("unexpected operation")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply))}, nil
	})
	sub, e := c.Subscribe(context.Background(), vault.Secret{Username: "fixture", Password: "synthetic-password"})
	if e != nil {
		t.Fatal(e)
	}
	items, e := sub.Pull(context.Background())
	if e != nil || len(items) != 1 || items[0].Type != "motion" || items[0].State != "active" || items[0].Channel != "1" || !items[0].ClockWarning {
		t.Fatal(items, e)
	}
	if e = sub.Close(context.Background()); e != nil || !closed {
		t.Fatal("unsubscribe missing", e)
	}
	for _, tc := range []struct{ topic, state, typ string }{{"Motion", "false", "motion"}, {"LineCrossing", "true", "line-crossing"}, {"VendorThing", "x", "unknown"}} {
		items, e = ParseEvents([]byte(soap("PullMessages", `<NotificationMessage><Topic>`+tc.topic+`</Topic><Message><SimpleItem Name="State" Value="`+tc.state+`"/></Message></NotificationMessage>`)), time.Now())
		if e != nil || items[0].Type != tc.typ {
			t.Fatal(e, items)
		}
	}
	if _, e = ParseEvents([]byte("<broken"), time.Now()); e == nil {
		t.Fatal("malformed event accepted")
	}
}

func TestEventPullCancelAndDisconnectedCleanup(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprint(disconnect), func(t *testing.T) {
			c, _ := NewClient(Target{Address: "192.0.2.3", URL: "http://192.0.2.3/onvif/device"})
			closed := false
			c.http.Transport = mediaTransport(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				if strings.Contains(string(b), "Unsubscribe") {
					closed = true
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(soap("Unsubscribe", "")))}, nil
				}
				if disconnect {
					return nil, errors.New("synthetic disconnected subscription")
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			})
			sub := &Subscription{client: c, endpoint: "http://192.0.2.3/onvif/events", secret: vault.Secret{Username: "fixture", Password: "synthetic"}, expires: time.Now().Add(time.Minute)}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if _, e := sub.Pull(ctx); e == nil {
				t.Fatal("missing failure")
			}
			closeCtx, finish := context.WithTimeout(context.Background(), time.Second)
			defer finish()
			if e := sub.Close(closeCtx); e != nil || !closed || sub.secret.Password != "" {
				t.Fatal("cleanup lost after cancellation/disconnect", e)
			}
		})
	}
}
