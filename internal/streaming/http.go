package streaming

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/icholy/digest"
)

type idleConn struct{ net.Conn }

func (c idleConn) Read(b []byte) (int, error) {
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	return c.Conn.Read(b)
}

func playHTTP(parent context.Context, s Source, sink Sink) (result error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var stalled atomic.Bool
	watchdog := time.AfterFunc(12*time.Second, func() { stalled.Store(true); cancel() })
	defer func() {
		watchdog.Stop()
		if stalled.Load() {
			result = errors.New("camera image stream stalled; check the endpoint and use Retry")
		}
	}()
	u, e := ValidateURL(s.address, s.raw)
	if e != nil {
		return e
	}
	expected := net.JoinHostPort(u.Hostname(), endpointPort(u))
	tlsConfig, e := TLSConfig(s.pin)
	if e != nil {
		return e
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig, ResponseHeaderTimeout: 4 * time.Second, TLSHandshakeTimeout: 3 * time.Second, MaxResponseHeaderBytes: 32 << 10,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != expected {
				return nil, ErrEndpoint
			}
			c, e := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp4", expected)
			if e != nil {
				return nil, e
			}
			return idleConn{c}, nil
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var lastNonce string
	var nonceCount int
	get := func() (*http.Response, error) {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		request.Header.Set("Cache-Control", "no-store")
		request.Header.Set("Accept", "multipart/x-mixed-replace, image/jpeg")
		response, err := client.Do(request)
		if err != nil {
			return nil, connectionError(ctx, err)
		}
		if response.StatusCode == 401 && s.secret.Username != "" {
			response.Body.Close()
			if challenge, err := digest.FindChallenge(response.Header); err == nil {
				if challenge.Nonce != lastNonce {
					lastNonce = challenge.Nonce
					nonceCount = 0
				}
				nonceCount++
				credentials, err := digest.Digest(challenge, digest.Options{Method: "GET", URI: u.RequestURI(), Count: nonceCount, Username: s.secret.Username, Password: s.secret.Password})
				if err != nil {
					return nil, ErrAuthentication
				}
				request.Header.Set("Authorization", credentials.String())
			} else if strings.Contains(strings.ToLower(strings.Join(response.Header.Values("WWW-Authenticate"), ",")), "basic") {
				request.SetBasicAuth(s.secret.Username, s.secret.Password)
			} else {
				return nil, ErrAuthentication
			}
			response, err = client.Do(request)
			if err != nil {
				return nil, connectionError(ctx, err)
			}
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			if response.StatusCode == 401 || response.StatusCode == 403 {
				return nil, ErrAuthentication
			}
			if response.StatusCode >= 300 && response.StatusCode < 400 {
				return nil, ErrEndpoint
			}
			return nil, errors.New("camera image endpoint was not found or rejected the request")
		}
		return response, nil
	}
	sendImage := func(reader io.Reader, kind string) error {
		frame, err := io.ReadAll(io.LimitReader(reader, MaxFrame+1))
		if err != nil {
			return connectionError(ctx, err)
		}
		if len(frame) > MaxFrame {
			return ErrMedia
		}
		config, err := jpeg.DecodeConfig(bytes.NewReader(frame))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > MaxWidth || config.Height > MaxHeight {
			return ErrMedia
		}
		if err = sink.Information(Info{MIME: "image/jpeg", Codec: "MJPEG", Width: config.Width, Height: config.Height, Transport: kind}); err != nil {
			return err
		}
		if err := sink.Write(Packet{Kind: 2, Data: frame}); err != nil {
			return err
		}
		watchdog.Reset(6 * time.Second)
		return nil
	}
	response, e := get()
	if e != nil {
		return e
	}
	if s.mode == "mjpeg" {
		defer response.Body.Close()
		typ, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil || typ != "multipart/x-mixed-replace" || len(params["boundary"]) == 0 || len(params["boundary"]) > 200 {
			return errors.New("endpoint is not a multipart MJPEG stream; choose snapshot refresh for still JPEG images")
		}
		reader := multipart.NewReader(response.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				return connectionError(ctx, err)
			}
			err = sendImage(part, "HTTP multipart MJPEG")
			if err != nil {
				return err
			}
			part.Close()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	for {
		typ, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if typ != "image/jpeg" {
			response.Body.Close()
			return errors.New("snapshot endpoint did not return a JPEG image")
		}
		err := sendImage(response.Body, "Snapshot refresh · up to 1 frame/s")
		response.Body.Close()
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		response, e = get()
		if e != nil {
			return e
		}
	}
}
