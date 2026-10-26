// Package isapi implements only documented read endpoints on an explicitly
// selected numeric camera/recorder address. It never follows redirects.
package isapi

import (
	"context"
	"errors"
	"github.com/TerminalAddict/SADPTools/internal/streaming"
	"github.com/TerminalAddict/SADPTools/internal/vault"
	"github.com/icholy/digest"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

var ErrAuthentication = errors.New("ISAPI authentication denied; check the selected account and permissions")
var ErrUnsupported = errors.New("ISAPI endpoint unsupported or unavailable on this firmware")
var ErrConnection = errors.New("ISAPI connection failed; check routing, port and certificate trust")

type Client struct {
	base *url.URL
	http *http.Client
}

var allowed = map[string]bool{"/ISAPI/System/deviceInfo": true, "/ISAPI/ContentMgmt/InputProxy/channels": true, "/ISAPI/ContentMgmt/InputProxy/channels/status": true, "/ISAPI/Streaming/channels": true, "/ISAPI/ContentMgmt/Storage": true, "/ISAPI/ContentMgmt/Storage/hdd": true, "/ISAPI/System/configurationData": true, "/ISAPI/System/time": true, "/ISAPI/System/Network/ntpServers": true}

func New(address, base, pin string) (*Client, error) {
	u, e := streaming.ValidateURL(address, base)
	if e != nil || u.Scheme != "http" && u.Scheme != "https" {
		return nil, streaming.ErrEndpoint
	}
	u.Path = ""
	u.RawQuery = ""
	tls, e := streaming.TLSConfig(pin)
	if e != nil {
		return nil, e
	}
	dial := &net.Dialer{Timeout: 3 * time.Second}
	tr := &http.Transport{Proxy: nil, DialContext: dial.DialContext, TLSClientConfig: tls, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableKeepAlives: true}
	return &Client{u, &http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Read(ctx context.Context, path string, s vault.Secret) ([]byte, error) {
	if !allowed[path] {
		return nil, errors.New("ISAPI operation is not an implemented read")
	}
	if e := streaming.ValidateCredentials(s); e != nil {
		return nil, e
	}
	u := *c.base
	u.Path = path
	do := func(auth string) (*http.Response, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		req.Header.Set("Cache-Control", "no-store")
		return c.http.Do(req)
	}
	r, e := do("")
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrConnection
	}
	if r.StatusCode == 401 && s.Username != "" {
		challenge, err := digest.FindChallenge(r.Header)
		r.Body.Close()
		if err != nil {
			return nil, ErrAuthentication
		}
		credential, err := digest.Digest(challenge, digest.Options{Method: "GET", URI: u.RequestURI(), Count: 1, Username: s.Username, Password: s.Password})
		if err != nil {
			return nil, ErrAuthentication
		}
		r, e = do(credential.String())
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrConnection
		}
	}
	defer r.Body.Close()
	if r.StatusCode == 401 || r.StatusCode == 403 {
		return nil, ErrAuthentication
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, ErrUnsupported
	}
	limit := 4 << 20
	if path == "/ISAPI/System/configurationData" {
		limit = 16 << 20
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, int64(limit+1)))
	if e != nil || len(b) > limit {
		clear(b)
		return nil, errors.New("ISAPI response incomplete or oversized")
	}
	return b, nil
}
