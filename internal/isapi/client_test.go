package isapi

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"io"
	"net/http"
	"strings"
	"testing"
)

type syntheticTransport func(*http.Request) (*http.Response, error)

func (f syntheticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestISAPIReadBoundaryAndFailures(t *testing.T) {
	for _, status := range []int{200, 401, 403, 404, 302} {
		c, e := New("192.0.2.4", "http://192.0.2.4/", "")
		if e != nil {
			t.Fatal(e)
		}
		calls := 0
		c.http.Transport = syntheticTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "GET" || r.URL.Path != "/ISAPI/ContentMgmt/InputProxy/channels" {
				t.Fatal("unsafe operation")
			}
			h := make(http.Header)
			if status == 302 {
				h.Set("Location", "http://198.51.100.8/")
			}
			return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader("<InputProxyChannelList/>"))}, nil
		})
		_, e = c.Read(context.Background(), "/ISAPI/ContentMgmt/InputProxy/channels", vault.Secret{})
		if status == 200 && e != nil || status != 200 && e == nil {
			t.Fatal(status, e)
		}
		if status == 401 || status == 403 {
			if !errors.Is(e, ErrAuthentication) {
				t.Fatal(e)
			}
		}
		if calls != 1 {
			t.Fatal("redirect/auth retry without credential")
		}
		if _, e = c.Read(context.Background(), "/ISAPI/System/reboot", vault.Secret{}); e == nil {
			t.Fatal("unapproved path")
		}
	}
	c, _ := New("192.0.2.4", "http://192.0.2.4/", "")
	c.http.Transport = syntheticTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("synthetic network loss") })
	if _, e := c.Read(context.Background(), "/ISAPI/System/deviceInfo", vault.Secret{}); !errors.Is(e, ErrConnection) {
		t.Fatal(e)
	}
}
